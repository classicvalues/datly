package reader

import (
	"context"
	"datly/base"
	"datly/cache"
	"datly/config"
	"datly/data"
	"datly/generic"
	"datly/metric"
	"fmt"
	"github.com/pkg/errors"
	"github.com/viant/afs"
	fcache "github.com/viant/afs/cache"
	"github.com/viant/dsc"
	"github.com/viant/toolbox"
	"strings"
	"sync"
	"time"
)

//Service represents a reader service
type Service interface {
	Read(ctx context.Context, request *Request) *Response
}

type service struct {
	cache  cache.Service
	config *config.Config
	fs     afs.Service
	cfs    afs.Service
}

//Read reads data for matched request URI
func (s *service) Read(ctx context.Context, request *Request) *Response {
	response := NewResponse()
	err := s.read(ctx, request, response)
	if err != nil {
		response.AddError(base.ErrorTypeException, "service.Read", err)
	}
	return response
}

func (s *service) read(ctx context.Context, request *Request, response *Response) error {
	err := s.config.ReloadChanged(ctx, s.cfs)
	if err != nil {
		response.RuleError = err.Error()
	}
	rule, uriParams := s.config.Rules.Match(request.URI)
	if rule == nil {
		response.Status = base.StatusNoMatch
		return nil
	}
	response.Rule = rule
	request.URIParams = uriParams

	waitGroup := &sync.WaitGroup{}
	waitGroup.Add(len(rule.Output))

	for i := range rule.Output {
		go func(output *data.Output) {
			defer waitGroup.Done()
			err := s.readOutputData(rule, output, ctx, request, response)
			if err != nil {
				response.AddError(base.ErrorTypeException, "service.readOutputData", err)
			}
		}(rule.Output[i])
	}
	waitGroup.Wait()
	return nil
}

func (s *service) readOutputData(rule *config.Rule, output *data.Output, ctx context.Context, request *Request, response *Response) error {
	view, err := rule.View(output.DataView)
	if err != nil {
		return err
	}
	selector := view.Selector.Clone()
	genericProvider := generic.NewProvider()
	collection := genericProvider.NewSlice()
	err = s.readViewData(ctx, collection, selector, view, rule, request, response)
	if err == nil {
		response.Put(output.Key, collection)
	}
	return err
}

func (s *service) readViewData(ctx context.Context, collection generic.Collection, selector *data.Selector, view *data.View, rule *config.Rule, request *Request, response *Response) error {
	bindings, err := s.assembleBinding(ctx, view, rule, request, response.Metrics)
	if err != nil {
		return errors.Wrapf(err, "failed to assemble bindings with rule: %v", rule.Info.URL)
	}
	selector.Apply(bindings)
	waitGroup := &sync.WaitGroup{}
	waitGroup.Add(1 + len(view.Refs))
	refData := &base.Registry{}
	go s.readRefs(ctx, view, selector, bindings, rule, request, response, waitGroup, refData)
	SQL, parameters, err := view.BuildSQL(selector, bindings)
	if err != nil {
		return errors.Wrapf(err, "failed to build SQL with rule: %v", rule.Info.URL)
	}

	parametrizedSQL := &dsc.ParametrizedSQL{SQL: SQL, Values: parameters}
	query := metric.NewQuery(parametrizedSQL)
	startTime := time.Now()
	started := false
	err = s.readData(ctx, SQL, parameters, view.Connector, func(record data.Record) error {
		query.Count++
		if ! started {
			started = true
			query.ExecutionTimeMs = base.ElapsedInMs(startTime)
			startTime = time.Now()
		}
		collection.Add(record)
		return nil
	})
	query.FetchTimeMs = base.ElapsedInMs(startTime)
	response.Metrics.AddQuery(query)
	if err != nil {
		return errors.Wrapf(err, "failed to read data with rule: %v", rule.Info.URL)
	}
	waitGroup.Wait()
	if len(refData.Data) > 0 {
		s.assignRefs(view, collection, refData.Data)
	}
	return err
}

func (s *service) readData(ctx context.Context, SQL string, parameters []interface{}, connector string, onRecord func(record data.Record) error) error {
	manager, err := s.getManager(ctx, connector)
	if err != nil {
		return err
	}
	return manager.ReadAllWithHandler(SQL, parameters, func(scanner dsc.Scanner) (toContinue bool, err error) {
		record := map[string]interface{}{}
		err = scanner.Scan(&record)
		if err == nil {
			err = onRecord(record)
		}
		return err == nil, err
	})
}

func (s *service) assembleBinding(ctx context.Context, view *data.View, rule *config.Rule, request *Request, metrics *metric.Metrics) (map[string]interface{}, error) {
	var result = make(map[string]interface{})
	base.MergeValues(request.QueryParams, result)
	base.MergeMap(request.Data, result)
	base.MergeValues(request.URIParams, result)
	var err error
	if len(view.Bindings) > 0 {

		var value interface{}
		for _, binding := range view.Bindings {
			switch binding.Type {
			case base.BindingDataView:
				if value, err = s.loadBindingData(ctx, rule, binding, result, metrics); err != nil {
					return nil, err
				}
			case base.BindingHeader:
				value = request.Headers.Get(binding.Name)
			case base.BindingData:
				value = request.Data[binding.Name]
			case base.BindingQueryString:
				value = request.QueryParams.Get(binding.Name)
			case base.BindingURI:
				value = request.URIParams.Get(binding.Name)
			default:
				return nil, errors.Errorf("unsupported binding source: %v", binding.Type)
			}
			if value == nil {
				value = binding.Default
			} else if text, ok := value.(string); ok && text == "" {
				value = binding.Default
			}
			result[binding.Placeholder] = value

		}
	}
	return result, nil
}

func (s *service) getManager(ctx context.Context, connectorName string) (dsc.Manager, error) {
	connector, err := s.config.Connectors.Get(connectorName)
	if err != nil {
		return nil, err
	}
	manager, err := dsc.NewManagerFactory().Create(connector.Config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get manager for %v", connectorName)
	}
	return manager, nil
}

func (s *service) loadBindingData(ctx context.Context, rule *config.Rule, binding *data.Binding, bindings map[string]interface{}, metrics *metric.Metrics) (interface{}, error) {
	view, err := rule.View(binding.DataView)
	if err != nil {
		return nil, err
	}
	selector := view.Selector.Clone()
	SQL, parameters, sqlErr := view.BuildSQL(selector, bindings)
	if sqlErr != nil {
		return nil, sqlErr
	}
	manager, err := s.getManager(ctx, view.Connector)
	if err != nil {
		return nil, err
	}
	var result = make([]string, 0)

	startTime := time.Now()
	started := false
	parametrizedSQL := &dsc.ParametrizedSQL{SQL: SQL, Values: parameters}
	query := metric.NewQuery(parametrizedSQL)
	err = manager.ReadAllWithHandler(SQL, parameters, func(scanner dsc.Scanner) (toContinue bool, err error) {
		query.Count++
		if ! started {
			started = true
			query.ExecutionTimeMs = base.ElapsedInMs(startTime)
			startTime = time.Now()
		}
		var data = make([]interface{}, 1)
		err = scanner.Scan(&data[0])
		if err == nil {
			text, ok := data[0].(string)
			if ok {
				result = append(result, "'"+text+"'")
			} else {
				result = append(result, toolbox.AsString(data[0]))
			}
		}
		return err == nil, err
	})
	query.FetchTimeMs = base.ElapsedInMs(startTime)
	metrics.AddQuery(query)
	return strings.Join(result, ","), err
}

func (s *service) readRefs(ctx context.Context, owner *data.View, selector *data.Selector, bindings map[string]interface{}, rule *config.Rule, request *Request, response *Response, group *sync.WaitGroup, refData *base.Registry) {
	defer group.Done()
	refs := owner.Refs
	if len(refs) == 0 {
		return
	}
	for i := range refs {
		go s.readRefData(owner, refs[i], selector, bindings, response, ctx, rule, request, refData, group)
	}
}

func (s *service) readRefData(owner *data.View, ref *data.Reference, selector *data.Selector, bindings map[string]interface{}, response *Response, ctx context.Context, rule *config.Rule, request *Request, refData *base.Registry, group *sync.WaitGroup) {
	defer group.Done()
	view, err := s.buildRefView(owner.Clone(), ref, selector, bindings)
	if err != nil {
		response.AddError(base.ErrorTypeException, "service.readOutputData", err)
		return
	}
	provider := generic.NewProvider()
	var collection generic.Collection
	if ref.Cardinality == base.CardinalityOne {
		collection = provider.NewMap(ref.RefIndex())
	} else {
		collection = provider.NewMultimap(ref.RefIndex())
	}

	err = s.readViewData(ctx, collection, view.Selector.Clone(), view, rule, request, response)
	if err != nil {
		response.AddError(base.ErrorTypeException, "service.readViewData", err)
	}
	refData.Put(ref.Name, collection)
}

func (s *service) buildRefView(owner *data.View, ref *data.Reference, selector *data.Selector, bindings map[string]interface{}) (*data.View, error) {
	refView := ref.View()
	if refView == nil {
		return nil, errors.Errorf("ref view was empty for owner: %v", owner.Name)
	}
	refView = refView.Clone()
	//Only when owner and reference connector is the same you can apply JOIN, otherwise all reference table has to be read into memory.
	if refView.Connector == owner.Connector {
		selector = selector.Clone()
		selector.Columns = ref.Columns()
		SQL, parameters, err := owner.BuildSQL(selector, bindings)
		if err != nil {
			return nil, err
		}
		refView.Params = parameters
		join := &data.Join{
			Type:  base.JoinTypeInner,
			Alias: ref.Alias(),
			Table: fmt.Sprintf("(%s)", SQL),
			On:    ref.Criteria(refView.Alias),
		}
		refView.AddJoin(join)
	}
	return refView, nil
}


func (s *service) assignRefs(view *data.View, ownerCollection generic.Collection, refData map[string]generic.Collection) error {
	return ownerCollection.Objects(func(item *generic.Object) (b bool, err error) {
		for _, ref := range view.Refs {
			data, ok := refData[ref.Name]
			if !ok {
				continue
			}
			index := ref.Index()
			key := index(item)

			if ref.Cardinality == base.CardinalityOne {
				aMap, ok := data.(*generic.Map)
				if ! ok {
					return false, errors.Errorf("invalid collection: expected : %T, but had %T", aMap, data)
				}
				value := aMap.Object(key)
				item.SetValue(ref.Name, value)
			} else {
				aMultimap, ok := data.(*generic.Multimap)
				if ! ok {
					return false, errors.Errorf("invalid collection: expected : %T, but had %T", aMultimap, data)
				}
				value := aMultimap.Slice(key)
				item.SetValue(ref.Name, value)
			}
		}
		return true, nil
	})
}

//New creates a service
func New(ctx context.Context, config *config.Config) (Service, error) {
	fs := afs.New()
	cfs := fs
	if config.UseRuleCache && config.URL != "" {
		cfs = fcache.Singleton(config.URL)
	}
	err := config.Init(ctx, cfs)
	srv := &service{
		config: config,
		fs:     fs,
		cfs:    cfs,
		cache:  cache.New(config.DataCacheURL, fs),
	}
	return srv, err
}
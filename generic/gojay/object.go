package gojay

import (
	"encoding/base64"
	"fmt"
	"github.com/francoispqt/gojay"
	"github.com/viant/datly/generic"
	"github.com/viant/toolbox"
)

type Object struct {
	*generic.Object
}

func (o Object) MarshalJSONObject(enc *gojay.Encoder) {
	fields := o.Proto().Fields()
	for _, field := range fields {
		value := o.ValueAt(field.Index)
		omitEmpty := field.ShallOmitEmpty(o.Proto())
		if omitEmpty {
			empty := field.IsEmpty(o.Proto(), value)
			if empty {
				continue
			}
		}
		if value == nil {
			enc.AddNullKey(field.OutputName())
			continue
		}
		if field.DataType == "" {
			field.InitType(value)
		}
		o.encodeJSONValue(field, value, enc)
	}
}

func (o *Object) encodeJSONValue(field *generic.Field, value interface{}, enc *gojay.Encoder) {
	switch field.DataType {
	case generic.FieldTypeInt:
		enc.IntKey(field.Name, toolbox.AsInt(value))
		return
	case generic.FieldTypeFloat:
		enc.FloatKey(field.Name, toolbox.AsFloat(value))
		return
	case generic.FieldTypeBool:

		enc.BoolKey(field.Name, toolbox.AsBoolean(value))
		return
	case generic.FieldTypeBytes:
		bs, ok := value.([]byte)
		if ok {
			value = base64.StdEncoding.EncodeToString(bs)
		}
		return
	case generic.FieldTypeArray:

		var marshaler gojay.MarshalerJSONArray
		collection, ok := value.(generic.Collection)
		if ok {
			switch val := collection.(type) {
			case *generic.Array:
				if val == nil {
					return
				}
			case *generic.Map:
				if val == nil {
					return
				}
			case *generic.Multimap:
				if val == nil {
					return
				}
			}
			marshaler = &Collection{collection}
		} else {
			fmt.Printf("collection fallback %T (%s)  !%s!\n", value, field.Name, value)
			marshaler = NewSlice(value)
		}
		enc.ArrayKeyOmitEmpty(field.Name, marshaler)

		return
	case generic.FieldTypeObject:
		object, ok := value.(*generic.Object)
		if !ok {
			provider := generic.NewProvider()
			provider.SetOmitEmpty(o.Proto().OmitEmpty)
			var err error
			object, err = provider.Object(value)
			if err != nil {
				return
			}
		}
		marshaler := &Object{object}
		enc.ObjectKeyOmitEmpty(field.Name, marshaler)
		return
	case generic.FieldTypeTime:
		timeLayout := field.TimeLayout(o.Proto())
		if timeLayout != "" {
			if timeValue, err := toolbox.ToTime(value, timeLayout); err == nil {
				value = timeValue.Format(timeLayout)
			}
		}
	}
	if field.ShallOmitEmpty(o.Proto()) {
		enc.StringKeyOmitEmpty(field.Name, toolbox.AsString(value))
		return
	}
	enc.StringKey(field.Name, toolbox.AsString(value))
}
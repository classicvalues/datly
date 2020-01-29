package contract

import (
	"github.com/viant/datly/generic"
	"sync"
)

//Data represents a registry
type Data struct {
	Data map[string]generic.Collection `json:",omitempty"`
	mux  sync.Mutex
}

//Put add data key
func (r *Data) Put(key string, value generic.Collection) {
	r.mux.Lock()
	defer r.mux.Unlock()
	if len(r.Data) == 0 {
		r.Data = make(map[string]generic.Collection)
	}
	r.Data[key] = value
}

//Get returns a collection for provided key
func (r *Data) Get(key string) generic.Collection {
	r.mux.Lock()
	defer r.mux.Unlock()
	if len(r.Data) == 0 {
		return nil
	}
	return r.Data[key]
}

//NewData creates new data
func NewData() *Data {
	return &Data{
		Data: make(map[string]generic.Collection),
		mux:  sync.Mutex{},
	}
}
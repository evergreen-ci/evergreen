//go:generate mapgen -name "" -zero "nil" -go-type "interface{}" -pkg "" -a "0" -b "1" -c "2" -bb "-1" -destination "syncx"
// Code generated by github.com/gobuffalo/mapgen. DO NOT EDIT.

package syncx

import (
	"sort"
	"sync"
)

// Map wraps sync.Map and uses the following types:
// key:   string
// value: interface{}
type Map struct {
	data sync.Map
}

// Delete the key from the map
func (m *Map) Delete(key string) {
	m.data.Delete(key)
}

// Load the key from the map.
// Returns interface{} or bool.
// A false return indicates either the key was not found
// or the value is not of type interface{}
func (m *Map) Load(key string) (interface{}, bool) {
	i, ok := m.data.Load(key)
	if !ok {
		return nil, false
	}
	s, ok := i.(interface{})
	return s, ok
}

// LoadOrStore will return an existing key or
// store the value if not already in the map
func (m *Map) LoadOrStore(key string, value interface{}) (interface{}, bool) {
	i, _ := m.data.LoadOrStore(key, value)
	s, ok := i.(interface{})
	return s, ok
}

// Range over the interface{} values in the map
func (m *Map) Range(f func(key string, value interface{}) bool) {
	m.data.Range(func(k, v interface{}) bool {
		key, ok := k.(string)
		if !ok {
			return false
		}
		value, ok := v.(interface{})
		if !ok {
			return false
		}
		return f(key, value)
	})
}

// Store a interface{} in the map
func (m *Map) Store(key string, value interface{}) {
	m.data.Store(key, value)
}

// Keys returns a list of keys in the map
func (m *Map) Keys() []string {
	var keys []string
	m.Range(func(key string, value interface{}) bool {
		keys = append(keys, key)
		return true
	})
	sort.Strings(keys)
	return keys
}

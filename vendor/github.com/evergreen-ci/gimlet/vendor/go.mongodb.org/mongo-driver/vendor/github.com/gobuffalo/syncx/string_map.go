//go:generate mapgen -name "String" -zero "``" -go-type "string" -pkg "" -a "`A`" -b "`B`" -c "`C`" -bb "`BB`" -destination "syncx"
// Code generated by github.com/gobuffalo/mapgen. DO NOT EDIT.

package syncx

import (
	"sort"
	"sync"
)

// StringMap wraps sync.Map and uses the following types:
// key:   string
// value: string
type StringMap struct {
	data sync.Map
}

// Delete the key from the map
func (m *StringMap) Delete(key string) {
	m.data.Delete(key)
}

// Load the key from the map.
// Returns string or bool.
// A false return indicates either the key was not found
// or the value is not of type string
func (m *StringMap) Load(key string) (string, bool) {
	i, ok := m.data.Load(key)
	if !ok {
		return ``, false
	}
	s, ok := i.(string)
	return s, ok
}

// LoadOrStore will return an existing key or
// store the value if not already in the map
func (m *StringMap) LoadOrStore(key string, value string) (string, bool) {
	i, _ := m.data.LoadOrStore(key, value)
	s, ok := i.(string)
	return s, ok
}

// Range over the string values in the map
func (m *StringMap) Range(f func(key string, value string) bool) {
	m.data.Range(func(k, v interface{}) bool {
		key, ok := k.(string)
		if !ok {
			return false
		}
		value, ok := v.(string)
		if !ok {
			return false
		}
		return f(key, value)
	})
}

// Store a string in the map
func (m *StringMap) Store(key string, value string) {
	m.data.Store(key, value)
}

// Keys returns a list of keys in the map
func (m *StringMap) Keys() []string {
	var keys []string
	m.Range(func(key string, value string) bool {
		keys = append(keys, key)
		return true
	})
	sort.Strings(keys)
	return keys
}

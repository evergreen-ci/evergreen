package util

import (
	"math"
	"reflect"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// IsFieldUndefined is an adaptation of IsZero https://golang.org/src/reflect/value.go?s=34297:34325#L1090
func IsFieldUndefined(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return math.Float64bits(v.Float()) == 0
	case reflect.Complex64, reflect.Complex128:
		c := v.Complex()
		return math.Float64bits(real(c)) == 0 && math.Float64bits(imag(c)) == 0
	case reflect.Array:
		return v.Len() == 0
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return v.IsNil()
	case reflect.String:
		return v.Len() == 0
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !IsFieldUndefined(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// this should never happen
		grip.Error(message.Fields{
			"message":    "field has no valid type",
			"value_type": v.Type(),
			"value_kind": v.Kind(),
			"value":      v.String(),
		})
		return false
	}
}

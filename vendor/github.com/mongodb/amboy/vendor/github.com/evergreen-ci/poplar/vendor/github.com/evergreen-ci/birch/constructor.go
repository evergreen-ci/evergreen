// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import (
	"math"
	"time"

	"github.com/evergreen-ci/birch/decimal"
	"github.com/evergreen-ci/birch/elements"
	"github.com/evergreen-ci/birch/types"
	"github.com/pkg/errors"
)

// EC is a convenience variable provided for access to the ElementConstructor methods.
var EC ElementConstructor

// VC is a convenience variable provided for access to the ValueConstructor methods.
var VC ValueConstructor

// ElementConstructor is used as a namespace for document element constructor functions.
type ElementConstructor struct{}

// ValueConstructor is used as a namespace for document element constructor functions.
type ValueConstructor struct{}

// Interface will attempt to turn the provided key and value into an Element.
// For common types, type casting is used, for all slices and all
// other complex types, this relies on the Marshaler interface.
//
// If the value cannot be converted to bson, a null Element is constructed with the
// key. This method will never return a nil *Element. If an error turning the
// value into an Element is desired, use the InterfaceErr method.
func (ElementConstructor) Interface(key string, value interface{}) *Element {
	var (
		elem *Element
		err  error
	)

	switch t := value.(type) {
	case bool:
		elem = EC.Boolean(key, t)
	case int8:
		elem = EC.Int32(key, int32(t))
	case int16:
		elem = EC.Int32(key, int32(t))
	case int32:
		elem = EC.Int32(key, t)
	case int64:
		elem = EC.Int64(key, t)
	case int:
		elem = EC.Int(key, t)
	case uint8:
		elem = EC.Int32(key, int32(t))
	case uint16:
		elem = EC.Int32(key, int32(t))
	case uint:
		switch {
		case t < math.MaxInt32:
			elem = EC.Int32(key, int32(t))
		case uint64(t) > math.MaxInt64:
			elem = EC.Null(key)
		default:
			elem = EC.Int64(key, int64(t))
		}
	case uint32:
		if t < math.MaxInt32 {
			elem = EC.Int32(key, int32(t))
		} else {
			elem = EC.Int64(key, int64(t))
		}

	case uint64:
		switch {
		case t < math.MaxInt32:
			elem = EC.Int32(key, int32(t))
		case t > math.MaxInt64:
			elem = EC.Null(key)
		default:
			elem = EC.Int64(key, int64(t))
		}
	case float32:
		elem = EC.Double(key, float64(t))
	case float64:
		elem = EC.Double(key, t)
	case string:
		elem = EC.String(key, t)
	case time.Time:
		elem = EC.Time(key, t)
	case types.Timestamp:
		elem = EC.Timestamp(key, t.T, t.I)
	case map[string]string:
		elem = EC.SubDocument(key, DC.MapString(t))
	case map[string]interface{}:
		elem = EC.SubDocument(key, DC.MapInterface(t))
	case map[string]int64:
		elem = EC.SubDocument(key, DC.MapInt64(t))
	case map[string]int32:
		elem = EC.SubDocument(key, DC.MapInt32(t))
	case map[string]int:
		elem = EC.SubDocument(key, DC.MapInt(t))
	case map[string]float64:
		elem = EC.SubDocument(key, DC.MapFloat64(t))
	case map[string]float32:
		elem = EC.SubDocument(key, DC.MapFloat32(t))
	case map[string]time.Time:
		elem = EC.SubDocument(key, DC.MapTime(t))
	case map[string]time.Duration:
		elem = EC.SubDocument(key, DC.MapDuration(t))
	case map[string]DocumentMarshaler:
		elem = EC.SubDocument(key, DC.MapDocumentMarshaler(t))
	case map[string]Marshaler:
		elem = EC.SubDocument(key, DC.MapMarshaler(t))
	case map[string][]string:
		elem = EC.SubDocument(key, DC.MapSliceString(t))
	case map[string][]interface{}:
		elem = EC.SubDocument(key, DC.MapSliceInterface(t))
	case map[string][]int64:
		elem = EC.SubDocument(key, DC.MapSliceInt64(t))
	case map[string][]int32:
		elem = EC.SubDocument(key, DC.MapSliceInt32(t))
	case map[string][]float64:
		elem = EC.SubDocument(key, DC.MapSliceFloat64(t))
	case map[string][]float32:
		elem = EC.SubDocument(key, DC.MapSliceFloat32(t))
	case map[string][]int:
		elem = EC.SubDocument(key, DC.MapSliceInt(t))
	case map[string][]time.Time:
		elem = EC.SubDocument(key, DC.MapSliceTime(t))
	case map[string][]time.Duration:
		elem = EC.SubDocument(key, DC.MapSliceDuration(t))
	case map[string][]DocumentMarshaler:
		elem = EC.SubDocument(key, DC.MapSliceDocumentMarshaler(t))
	case map[string][]Marshaler:
		elem = EC.SubDocument(key, DC.MapSliceMarshaler(t))
	case map[interface{}]interface{}:
		elem = EC.SubDocument(key, DC.Interface(t))
	case []interface{}:
		elem = EC.SliceInterface(key, t)
	case []string:
		elem = EC.SliceString(key, t)
	case []int64:
		elem = EC.SliceInt64(key, t)
	case []int32:
		elem = EC.SliceInt32(key, t)
	case []float64:
		elem = EC.SliceFloat64(key, t)
	case []float32:
		elem = EC.SliceFloat32(key, t)
	case []int:
		elem = EC.SliceInt(key, t)
	case []time.Time:
		elem = EC.SliceTime(key, t)
	case []time.Duration:
		elem = EC.SliceDuration(key, t)
	case []DocumentMarshaler:
		elem, err = EC.SliceDocumentMarshalerErr(key, t)
	case []Marshaler:
		elem, err = EC.SliceMarshalerErr(key, t)
	case []*Element:
		elem = EC.SubDocumentFromElements(key, t...)
	case *Element:
		elem = t
	case *Value:
		elem, err = EC.ValueErr(key, t)
	case *Document:
		if t != nil {
			elem = EC.SubDocument(key, t)
		}
	case Reader:
		var doc *Document
		doc, err = DC.ReaderErr(t)
		if err == nil {
			elem = EC.SubDocument(key, doc)
		}
	case DocumentMarshaler:
		elem, err = EC.DocumentMarshalerErr(key, t)
	case Marshaler:
		elem, err = EC.MarshalerErr(key, t)
	default:
		elem = EC.Null(key)
	}

	if err != nil || elem == nil {
		elem = EC.Null(key)
	}

	return elem
}

// InterfaceErr does what Interface does, but returns an error when it cannot
// properly convert a value into an *Element. See Interface for details.
func (ElementConstructor) InterfaceErr(key string, value interface{}) (*Element, error) {
	switch t := value.(type) {
	case uint:
		switch {
		case t < math.MaxInt32:
			return EC.Int32(key, int32(t)), nil
		case uint64(t) > math.MaxInt64:
			return nil, errors.Errorf("BSON only has signed integer types and %d overflows an int64", t)
		default:
			return EC.Int64(key, int64(t)), nil
		}
	case uint64:
		switch {
		case t < math.MaxInt32:
			return EC.Int32(key, int32(t)), nil
		case t > math.MaxInt64:
			return nil, errors.Errorf("BSON only has signed integer types and %d overflows an int64", t)
		default:
			return EC.Int64(key, int64(t)), nil
		}
	case bool, int8, int16, int32, int, int64, uint8, uint16, uint32, string, float32, float64,
		*Element, *Document, Reader, types.Timestamp,
		time.Time:

		return EC.Interface(key, t), nil

	case map[string]string, map[string]float32, map[string]float64,
		map[string]int32, map[string]int64, map[string]int,
		map[string]time.Time, map[string]time.Duration:

		return EC.Interface(key, t), nil
	case map[string][]string, map[string][]int32, map[string][]int64, map[string][]int,
		map[string][]time.Time, map[string][]time.Duration, map[string][]float32, map[string][]float64:

		return EC.Interface(key, value), nil
	case []string, []int32, []int64, []int, []time.Time, []time.Duration, []float64, []float32:
		return EC.Interface(key, value), nil
	case map[string]interface{}, map[interface{}]interface{}, map[string]Marshaler, map[string]DocumentMarshaler:
		return EC.InterfaceErr(key, t)
	case map[string][]interface{}, map[string][]Marshaler, map[string][]DocumentMarshaler:
		return EC.InterfaceErr(key, value)
	case []interface{}, []Marshaler, []DocumentMarshaler:
		return EC.InterfaceErr(key, value)
	case *Value:
		return EC.ValueErr(key, t)
	case DocumentMarshaler:
		return EC.DocumentMarshalerErr(key, t)
	case Marshaler:
		return EC.MarshalerErr(key, t)
	default:
		return nil, errors.Errorf("Cannot create element for type %T, try using bsoncodec.ConstructElementErr", value)
	}
}

// Double creates a double element with the given key and value.
func (ElementConstructor) Double(key string, f float64) *Element {
	b := make([]byte, 1+len(key)+1+8)
	elem := newElement(0, 1+uint32(len(key))+1)
	_, err := elements.Double.Element(0, b, key, f)
	if err != nil {
		panic(err)
	}

	elem.value.data = b
	return elem
}

// String creates a string element with the given key and value.
func (ElementConstructor) String(key string, val string) *Element {
	size := uint32(1 + len(key) + 1 + 4 + len(val) + 1)
	b := make([]byte, size)
	elem := newElement(0, 1+uint32(len(key))+1)
	_, err := elements.String.Element(0, b, key, val)
	if err != nil {
		panic(err)
	}
	elem.value.data = b
	return elem
}

// SubDocument creates a subdocument element with the given key and value.
func (ElementConstructor) SubDocument(key string, d *Document) *Element {
	size := uint32(1 + len(key) + 1)
	b := make([]byte, size)
	elem := newElement(0, size)
	_, err := elements.Byte.Encode(0, b, '\x03')
	if err != nil {
		panic(err)
	}
	_, err = elements.CString.Encode(1, b, key)
	if err != nil {
		panic(err)
	}
	elem.value.data = b
	elem.value.d = d
	return elem
}

// SubDocumentFromReader creates a subdocument element with the given key and value.
func (ElementConstructor) SubDocumentFromReader(key string, r Reader) *Element {
	size := uint32(1 + len(key) + 1 + len(r))
	b := make([]byte, size)
	elem := newElement(0, uint32(1+len(key)+1))
	_, err := elements.Byte.Encode(0, b, '\x03')
	if err != nil {
		panic(err)
	}
	_, err = elements.CString.Encode(1, b, key)
	if err != nil {
		panic(err)
	}
	// NOTE: We don't validate the Reader here since we don't validate the
	// Document when provided to SubDocument.
	copy(b[1+len(key)+1:], r)
	elem.value.data = b
	return elem
}

// SubDocumentFromElements creates a subdocument element with the given key. The elements passed as
// arguments will be used to create a new document as the value.
func (ElementConstructor) SubDocumentFromElements(key string, elems ...*Element) *Element {
	return EC.SubDocument(key, NewDocument(elems...))
}

// Array creates an array element with the given key and value.
func (ElementConstructor) Array(key string, a *Array) *Element {
	size := uint32(1 + len(key) + 1)
	b := make([]byte, size)
	elem := newElement(0, size)
	_, err := elements.Byte.Encode(0, b, '\x04')
	if err != nil {
		panic(err)
	}
	_, err = elements.CString.Encode(1, b, key)
	if err != nil {
		panic(err)
	}
	elem.value.data = b
	elem.value.d = a.doc
	return elem
}

// ArrayFromElements creates an element with the given key. The elements passed as
// arguments will be used to create a new array as the value.
func (ElementConstructor) ArrayFromElements(key string, values ...*Value) *Element {
	return EC.Array(key, NewArray(values...))
}

// Binary creates a binary element with the given key and value.
func (ElementConstructor) Binary(key string, b []byte) *Element {
	return EC.BinaryWithSubtype(key, b, 0)
}

// BinaryWithSubtype creates a binary element with the given key. It will create a new BSON binary value
// with the given data and subtype.
func (ElementConstructor) BinaryWithSubtype(key string, b []byte, btype byte) *Element {
	size := uint32(1 + len(key) + 1 + 4 + 1 + len(b))
	if btype == 2 {
		size += 4
	}

	buf := make([]byte, size)
	elem := newElement(0, 1+uint32(len(key))+1)
	_, err := elements.Binary.Element(0, buf, key, b, btype)
	if err != nil {
		panic(err)
	}

	elem.value.data = buf
	return elem
}

// Undefined creates a undefined element with the given key.
func (ElementConstructor) Undefined(key string) *Element {
	size := 1 + uint32(len(key)) + 1
	b := make([]byte, size)
	elem := newElement(0, size)
	_, err := elements.Byte.Encode(0, b, '\x06')
	if err != nil {
		panic(err)
	}
	_, err = elements.CString.Encode(1, b, key)
	if err != nil {
		panic(err)
	}
	elem.value.data = b
	return elem
}

// ObjectID creates a objectid element with the given key and value.
func (ElementConstructor) ObjectID(key string, oid types.ObjectID) *Element {
	size := uint32(1 + len(key) + 1 + 12)
	elem := newElement(0, 1+uint32(len(key))+1)
	elem.value.data = make([]byte, size)

	_, err := elements.ObjectID.Element(0, elem.value.data, key, oid)
	if err != nil {
		panic(err)
	}

	return elem
}

// Boolean creates a boolean element with the given key and value.
func (ElementConstructor) Boolean(key string, b bool) *Element {
	size := uint32(1 + len(key) + 1 + 1)
	elem := newElement(0, 1+uint32(len(key))+1)
	elem.value.data = make([]byte, size)

	_, err := elements.Boolean.Element(0, elem.value.data, key, b)
	if err != nil {
		panic(err)
	}

	return elem
}

// DateTime creates a datetime element with the given key and value.
// dt represents milliseconds since the Unix epoch
func (ElementConstructor) DateTime(key string, dt int64) *Element {
	size := uint32(1 + len(key) + 1 + 8)
	elem := newElement(0, 1+uint32(len(key))+1)
	elem.value.data = make([]byte, size)

	_, err := elements.DateTime.Element(0, elem.value.data, key, dt)
	if err != nil {
		panic(err)
	}

	return elem
}

// Time creates a datetime element with the given key and value.
func (ElementConstructor) Time(key string, t time.Time) *Element {
	// Apply nanoseconds to milliseconds conversion
	return EC.DateTime(key, t.Unix()*1000+int64(t.Nanosecond()/1e6))
}

// Null creates a null element with the given key.
func (ElementConstructor) Null(key string) *Element {
	size := uint32(1 + len(key) + 1)
	b := make([]byte, size)
	elem := newElement(0, uint32(1+len(key)+1))
	_, err := elements.Byte.Encode(0, b, '\x0A')
	if err != nil {
		panic(err)
	}
	_, err = elements.CString.Encode(1, b, key)
	if err != nil {
		panic(err)
	}
	elem.value.data = b
	return elem
}

// Regex creates a regex element with the given key and value.
func (ElementConstructor) Regex(key string, pattern, options string) *Element {
	size := uint32(1 + len(key) + 1 + len(pattern) + 1 + len(options) + 1)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Regex.Element(0, elem.value.data, key, pattern, options)
	if err != nil {
		panic(err)
	}

	return elem
}

// DBPointer creates a dbpointer element with the given key and value.
func (ElementConstructor) DBPointer(key string, ns string, oid types.ObjectID) *Element {
	size := uint32(1 + len(key) + 1 + 4 + len(ns) + 1 + 12)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.DBPointer.Element(0, elem.value.data, key, ns, oid)
	if err != nil {
		panic(err)
	}

	return elem
}

// JavaScript creates a JavaScript code element with the given key and value.
func (ElementConstructor) JavaScript(key string, code string) *Element {
	size := uint32(1 + len(key) + 1 + 4 + len(code) + 1)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.JavaScript.Element(0, elem.value.data, key, code)
	if err != nil {
		panic(err)
	}

	return elem
}

// Symbol creates a symbol element with the given key and value.
func (ElementConstructor) Symbol(key string, symbol string) *Element {
	size := uint32(1 + len(key) + 1 + 4 + len(symbol) + 1)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Symbol.Element(0, elem.value.data, key, symbol)
	if err != nil {
		panic(err)
	}

	return elem
}

// CodeWithScope creates a JavaScript code with scope element with the given key and value.
func (ElementConstructor) CodeWithScope(key string, code string, scope *Document) *Element {
	size := uint32(1 + len(key) + 1 + 4 + 4 + len(code) + 1)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)
	elem.value.d = scope

	_, err := elements.Byte.Encode(0, elem.value.data, '\x0F')
	if err != nil {
		panic(err)
	}

	_, err = elements.CString.Encode(1, elem.value.data, key)
	if err != nil {
		panic(err)
	}

	_, err = elements.Int32.Encode(1+uint(len(key))+1, elem.value.data, int32(size))
	if err != nil {
		panic(err)
	}

	_, err = elements.String.Encode(1+uint(len(key))+1+4, elem.value.data, code)
	if err != nil {
		panic(err)
	}

	return elem
}

// Int32 creates a int32 element with the given key and value.
func (ElementConstructor) Int32(key string, i int32) *Element {
	size := uint32(1 + len(key) + 1 + 4)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Int32.Element(0, elem.value.data, key, i)
	if err != nil {
		panic(err)
	}

	return elem
}

// Timestamp creates a timestamp element with the given key and value.
func (ElementConstructor) Timestamp(key string, t uint32, i uint32) *Element {
	size := uint32(1 + len(key) + 1 + 8)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Timestamp.Element(0, elem.value.data, key, t, i)
	if err != nil {
		panic(err)
	}

	return elem
}

// Int64 creates a int64 element with the given key and value.
func (ElementConstructor) Int64(key string, i int64) *Element {
	size := uint32(1 + len(key) + 1 + 8)
	elem := newElement(0, 1+uint32(len(key))+1)
	elem.value.data = make([]byte, size)

	_, err := elements.Int64.Element(0, elem.value.data, key, i)
	if err != nil {
		panic(err)
	}

	return elem
}

// Decimal128 creates a decimal element with the given key and value.
func (ElementConstructor) Decimal128(key string, d decimal.Decimal128) *Element {
	size := uint32(1 + len(key) + 1 + 16)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Decimal128.Element(0, elem.value.data, key, d)
	if err != nil {
		panic(err)
	}

	return elem
}

// MinKey creates a minkey element with the given key and value.
func (ElementConstructor) MinKey(key string) *Element {
	size := uint32(1 + len(key) + 1)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Byte.Encode(0, elem.value.data, '\xFF')
	if err != nil {
		panic(err)
	}

	_, err = elements.CString.Encode(1, elem.value.data, key)
	if err != nil {
		panic(err)
	}

	return elem
}

// MaxKey creates a maxkey element with the given key and value.
func (ElementConstructor) MaxKey(key string) *Element {
	size := uint32(1 + len(key) + 1)
	elem := newElement(0, uint32(1+len(key)+1))
	elem.value.data = make([]byte, size)

	_, err := elements.Byte.Encode(0, elem.value.data, '\x7F')
	if err != nil {
		panic(err)
	}

	_, err = elements.CString.Encode(1, elem.value.data, key)
	if err != nil {
		panic(err)
	}

	return elem
}

// Value constructs an element using the underlying value.
func (ElementConstructor) Value(key string, value *Value) *Element {
	return convertValueToElem(key, value)
}

func (ElementConstructor) ValueErr(key string, value *Value) (*Element, error) {
	elem := EC.Value(key, value)
	if elem == nil {
		return nil, errors.Errorf("could not convert '%s' value to an element", key)
	}

	return elem, nil
}

// Double creates a double element with the given value.
func (ValueConstructor) Double(f float64) *Value {
	return EC.Double("", f).value
}

// String creates a string element with the given value.
func (ValueConstructor) String(val string) *Value {
	return EC.String("", val).value
}

// Document creates a subdocument value from the argument.
func (ValueConstructor) Document(d *Document) *Value {
	return EC.SubDocument("", d).value
}

// DocumentFromReader creates a subdocument element from the given value.
func (ValueConstructor) DocumentFromReader(r Reader) *Value {
	return EC.SubDocumentFromReader("", r).value
}

// DocumentFromElements creates a subdocument element from the given elements.
func (ValueConstructor) DocumentFromElements(elems ...*Element) *Value {
	return EC.SubDocumentFromElements("", elems...).value
}

// Array creates an array value from the argument.
func (ValueConstructor) Array(a *Array) *Value {
	return EC.Array("", a).value
}

// ArrayFromValues creates an array element from the given the elements.
func (ValueConstructor) ArrayFromValues(values ...*Value) *Value {
	return EC.ArrayFromElements("", values...).value
}

// Binary creates a binary value from the argument.
func (ValueConstructor) Binary(b []byte) *Value {
	return VC.BinaryWithSubtype(b, 0)
}

// BinaryWithSubtype creates a new binary element with the given data and subtype.
func (ValueConstructor) BinaryWithSubtype(b []byte, btype byte) *Value {
	return EC.BinaryWithSubtype("", b, btype).value
}

// Undefined creates a undefined element.
func (ValueConstructor) Undefined() *Value {
	return EC.Undefined("").value
}

// ObjectID creates a objectid value from the argument.
func (ValueConstructor) ObjectID(oid types.ObjectID) *Value {
	return EC.ObjectID("", oid).value
}

// Boolean creates a boolean value from the argument.
func (ValueConstructor) Boolean(b bool) *Value {
	return EC.Boolean("", b).value
}

// Time creates a datetime value from the argument.
func (ValueConstructor) Time(t time.Time) *Value {
	return EC.Time("", t).value
}

// DateTime creates a datetime value from the argument.
func (ValueConstructor) DateTime(dt int64) *Value {
	return EC.DateTime("", dt).value
}

// Null creates a null value from the argument.
func (ValueConstructor) Null() *Value {
	return EC.Null("").value
}

// Regex creates a regex value from the arguments.
func (ValueConstructor) Regex(pattern, options string) *Value {
	return EC.Regex("", pattern, options).value
}

// DBPointer creates a dbpointer value from the arguments.
func (ValueConstructor) DBPointer(ns string, oid types.ObjectID) *Value {
	return EC.DBPointer("", ns, oid).value
}

// JavaScript creates a JavaScript code value from the argument.
func (ValueConstructor) JavaScript(code string) *Value {
	return EC.JavaScript("", code).value
}

// Symbol creates a symbol value from the argument.
func (ValueConstructor) Symbol(symbol string) *Value {
	return EC.Symbol("", symbol).value
}

// CodeWithScope creates a JavaScript code with scope value from the arguments.
func (ValueConstructor) CodeWithScope(code string, scope *Document) *Value {
	return EC.CodeWithScope("", code, scope).value
}

// Int32 creates a int32 value from the argument.
func (ValueConstructor) Int32(i int32) *Value {
	return EC.Int32("", i).value
}

// Timestamp creates a timestamp value from the arguments.
func (ValueConstructor) Timestamp(t uint32, i uint32) *Value {
	return EC.Timestamp("", t, i).value
}

// Int64 creates a int64 value from the argument.
func (ValueConstructor) Int64(i int64) *Value {
	return EC.Int64("", i).value
}

// Decimal128 creates a decimal value from the argument.
func (ValueConstructor) Decimal128(d decimal.Decimal128) *Value {
	return EC.Decimal128("", d).value
}

// MinKey creates a minkey value from the argument.
func (ValueConstructor) MinKey() *Value {
	return EC.MinKey("").value
}

// MaxKey creates a maxkey value from the argument.
func (ValueConstructor) MaxKey() *Value {
	return EC.MaxKey("").value
}

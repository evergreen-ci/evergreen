// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Represents JSON data structure using native Go types: booleans, floats,
// strings, arrays, and maps.

package json

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v.
//
// Unmarshal uses the inverse of the encodings that
// Marshal uses, allocating maps, slices, and pointers as necessary,
// with the following additional rules:
//
// To unmarshal JSON into a pointer, Unmarshal first handles the case of
// the JSON being the JSON literal null. In that case, Unmarshal sets
// the pointer to nil. Otherwise, Unmarshal unmarshals the JSON into
// the value pointed at by the pointer. If the pointer is nil, Unmarshal
// allocates a new value for it to point to.
//
// To unmarshal JSON into a struct, Unmarshal matches incoming object
// keys to the keys used by Marshal (either the struct field name or its tag),
// preferring an exact match but also accepting a case-insensitive match.
// Unmarshal will only set exported fields of the struct.
//
// To unmarshal JSON into an interface value,
// Unmarshal stores one of these in the interface value:
//
//	bool, for JSON booleans
//	float64, for JSON numbers
//	string, for JSON strings
//	[]interface{}, for JSON arrays
//	map[string]interface{}, for JSON objects
//	nil for JSON null
//
// To unmarshal a JSON array into a slice, Unmarshal resets the slice length
// to zero and then appends each element to the slice.
// As a special case, to unmarshal an empty JSON array into a slice,
// Unmarshal replaces the slice with a new empty slice.
//
// To unmarshal a JSON array into a Go array, Unmarshal decodes
// JSON array elements into corresponding Go array elements.
// If the Go array is smaller than the JSON array,
// the additional JSON array elements are discarded.
// If the JSON array is smaller than the Go array,
// the additional Go array elements are set to zero values.
//
// To unmarshal a JSON object into a map, Unmarshal first establishes a map to
// use, If the map is nil, Unmarshal allocates a new map. Otherwise Unmarshal
// reuses the existing map, keeping existing entries. Unmarshal then stores key-
// value pairs from the JSON object into the map.  The map's key type must
// either be a string or implement encoding.TextUnmarshaler.
//
// If a JSON value is not appropriate for a given target type,
// or if a JSON number overflows the target type, Unmarshal
// skips that field and completes the unmarshaling as best it can.
// If no more serious errors are encountered, Unmarshal returns
// an UnmarshalTypeError describing the earliest such error.
//
// The JSON null value unmarshals into an interface, map, pointer, or slice
// by setting that Go value to nil. Because null is often used in JSON to mean
// “not present,” unmarshaling a JSON null into any other Go type has no effect
// on the value and produces no error.
//
// When unmarshaling quoted strings, invalid UTF-8 or
// invalid UTF-16 surrogate pairs are not treated as an error.
// Instead, they are replaced by the Unicode replacement
// character U+FFFD.
func Unmarshal(data []byte, v any) error {
	// Check for well-formedness.
	// Avoids filling out half a data structure
	// before discovering a JSON syntax error.
	var d decodeState
	err := checkValid(data, &d.scan)
	if err != nil {
		return err
	}

	d.init(data)
	return d.unmarshal(v)
}

// Unmarshaler is the interface implemented by types
// that can unmarshal a JSON description of themselves.
// The input can be assumed to be a valid encoding of
// a JSON value. UnmarshalJSON must copy the JSON data
// if it wishes to retain the data after returning.
type Unmarshaler interface {
	UnmarshalJSON([]byte) error
}

// An UnmarshalTypeError describes a JSON value that was
// not appropriate for a value of a specific Go type.
type UnmarshalTypeError struct {
	Value  string       // description of JSON value - "bool", "array", "number -5"
	Type   reflect.Type // type of Go value it could not be assigned to
	Offset int64        // error occurred after reading Offset bytes
}

func (e *UnmarshalTypeError) Error() string {
	return "json: cannot unmarshal " + e.Value + " into Go value of type " + e.Type.String()
}

// An UnmarshalFieldError describes a JSON object key that
// led to an unexported (and therefore unwritable) struct field.
// (No longer used; kept for compatibility.)
type UnmarshalFieldError struct {
	Key   string
	Type  reflect.Type
	Field reflect.StructField
}

func (e *UnmarshalFieldError) Error() string {
	return "json: cannot unmarshal object key " + strconv.Quote(e.Key) + " into unexported field " + e.Field.Name + " of type " + e.Type.String()
}

// An InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// (The argument to Unmarshal must be a non-nil pointer.)
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "json: Unmarshal(nil)"
	}

	if e.Type.Kind() != reflect.Ptr {
		return "json: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "json: Unmarshal(nil " + e.Type.String() + ")"
}

func (d *decodeState) unmarshal(v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}

	d.scan.reset()
	// We decode rv not rv.Elem because the Unmarshaler interface
	// test must be applied at the top level of the value.
	d.value(rv)
	return d.savedError
}

// A Number represents a JSON number literal.
type Number string

// String returns the literal text of the number.
func (n Number) String() string { return string(n) }

// Float64 returns the number as a float64.
func (n Number) Float64() (float64, error) {
	return strconv.ParseFloat(string(n), 64)
}

// Int64 returns the number as an int64.
func (n Number) Int64() (int64, error) {
	return strconv.ParseInt(string(n), 10, 64)
}

// isValidNumber reports whether s is a valid JSON number literal.
func isValidNumber(s string) bool {
	// This function implements the JSON numbers grammar.
	// See https://tools.ietf.org/html/rfc7159#section-6
	// and http://json.org/number.gif

	if s == "" {
		return false
	}

	// Optional -
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}

	// Digits
	switch {
	default:
		return false

	case s[0] == '0':
		s = s[1:]

	case '1' <= s[0] && s[0] <= '9':
		s = s[1:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// . followed by 1 or more digits.
	if len(s) >= 2 && s[0] == '.' && '0' <= s[1] && s[1] <= '9' {
		s = s[2:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// e or E followed by an optional - or + and
	// 1 or more digits.
	if len(s) >= 2 && (s[0] == 'e' || s[0] == 'E') {
		s = s[1:]
		if s[0] == '+' || s[0] == '-' {
			s = s[1:]
			if s == "" {
				return false
			}
		}
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// Make sure we are at the end.
	return s == ""
}

// decodeState represents the state while decoding a JSON value.
type decodeState struct {
	data       []byte
	off        int // read offset in data
	scan       scanner
	nextscan   scanner // for calls to nextValue
	savedError error
	useNumber  bool
	ext        Extension
}

// errPhase is used for errors that should not happen unless
// there is a bug in the JSON decoder or something is editing
// the data slice while the decoder executes.
var errPhase = errors.New("JSON decoder out of sync - data changing underfoot?")

func (d *decodeState) init(data []byte) *decodeState {
	d.data = data
	d.off = 0
	d.savedError = nil
	return d
}

// error aborts the decoding by panicking with err.
func (d *decodeState) error(err error) {
	panic(err)
}

// saveError saves the first err it is called with,
// for reporting at the end of the unmarshal.
func (d *decodeState) saveError(err error) {
	if d.savedError == nil {
		d.savedError = err
	}
}

// next cuts off and returns the next full JSON value in d.data[d.off:].
// The next value is known to be an object or array, not a literal.
func (d *decodeState) next() []byte {
	c := d.data[d.off]
	item, rest, err := nextValue(d.data[d.off:], &d.nextscan)
	if err != nil {
		d.error(err)
	}
	d.off = len(d.data) - len(rest)

	// Our scanner has seen the opening brace/bracket
	// and thinks we're still in the middle of the object.
	// invent a closing brace/bracket to get it out.
	if c == '{' {
		d.scan.step(&d.scan, '}')
	} else if c == '[' {
		d.scan.step(&d.scan, ']')
	} else {
		// Was inside a function name. Get out of it.
		d.scan.step(&d.scan, '(')
		d.scan.step(&d.scan, ')')
	}

	return item
}

// scanWhile processes bytes in d.data[d.off:] until it
// receives a scan code not equal to op.
// It updates d.off and returns the new scan code.
func (d *decodeState) scanWhile(op int) int {
	var newOp int
	for {
		if d.off >= len(d.data) {
			newOp = d.scan.eof()
			d.off = len(d.data) + 1 // mark processed EOF with len+1
		} else {
			c := d.data[d.off]
			d.off++
			newOp = d.scan.step(&d.scan, c)
		}
		if newOp != op {
			break
		}
	}
	return newOp
}

// value decodes a JSON value from d.data[d.off:] into the value.
// it updates d.off to point past the decoded value.
func (d *decodeState) value(v reflect.Value) {
	if !v.IsValid() {
		_, rest, err := nextValue(d.data[d.off:], &d.nextscan)
		if err != nil {
			d.error(err)
		}
		d.off = len(d.data) - len(rest)

		// d.scan thinks we're still at the beginning of the item.
		// Feed in an empty string - the shortest, simplest value -
		// so that it knows we got to the end of the value.
		if d.scan.redo {
			// rewind.
			d.scan.redo = false
			d.scan.step = stateBeginValue
		}
		d.scan.step(&d.scan, '"')
		d.scan.step(&d.scan, '"')

		n := len(d.scan.parseState)
		if n > 0 && d.scan.parseState[n-1] == parseObjectKey {
			// d.scan thinks we just read an object key; finish the object
			d.scan.step(&d.scan, ':')
			d.scan.step(&d.scan, '"')
			d.scan.step(&d.scan, '"')
			d.scan.step(&d.scan, '}')
		}

		return
	}

	switch op := d.scanWhile(scanSkipSpace); op {
	default:
		d.error(errPhase)

	case scanBeginArray:
		d.array(v)

	case scanBeginObject:
		d.object(v)

	case scanBeginLiteral:
		d.literal(v)

	case scanBeginName:
		d.name(v)
	}
}

type unquotedValue struct{}

// valueQuoted is like value but decodes a
// quoted string literal or literal null into an interface value.
// If it finds anything other than a quoted string literal or null,
// valueQuoted returns unquotedValue{}.
func (d *decodeState) valueQuoted() any {
	switch op := d.scanWhile(scanSkipSpace); op {
	default:
		d.error(errPhase)

	case scanBeginArray:
		d.array(reflect.Value{})

	case scanBeginObject:
		d.object(reflect.Value{})

	case scanBeginName:
		switch v := d.nameInterface().(type) {
		case nil, string:
			return v
		}

	case scanBeginLiteral:
		switch v := d.literalInterface().(type) {
		case nil, string:
			return v
		}
	}
	return unquotedValue{}
}

// indirect walks down v allocating pointers as needed,
// until it gets to a non-pointer.
// if it encounters an Unmarshaler, indirect stops and returns that.
// if decodingNull is true, indirect stops at the last pointer so it can be set to nil.
func (d *decodeState) indirect(v reflect.Value, decodingNull bool) (Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	// If v is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() && (!decodingNull || e.Elem().Kind() == reflect.Ptr) {
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if v.Elem().Kind() != reflect.Ptr && decodingNull && v.CanSet() {
			break
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, nil, v
			}
			if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
				return nil, u, v
			}
		}
		v = v.Elem()
	}
	return nil, nil, v
}

// array consumes an array from d.data[d.off-1:], decoding into the value v.
// the first byte of the array ('[') has been read already.
func (d *decodeState) array(v reflect.Value) {
	// Check for unmarshaler.
	u, ut, pv := d.indirect(v, false)
	if u != nil {
		d.off--
		err := u.UnmarshalJSON(d.next())
		if err != nil {
			d.error(err)
		}
		return
	}
	if ut != nil {
		d.saveError(&UnmarshalTypeError{"array", v.Type(), int64(d.off)})
		d.off--
		d.next()
		return
	}

	v = pv

	// Check type of target.
	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() == 0 {
			// Decoding into nil interface?  Switch to non-reflect code.
			v.Set(reflect.ValueOf(d.arrayInterface()))
			return
		}
		// Otherwise it's invalid.
		fallthrough
	default:
		d.saveError(&UnmarshalTypeError{"array", v.Type(), int64(d.off)})
		d.off--
		d.next()
		return
	case reflect.Array:
	case reflect.Slice:
		break
	}

	i := 0
	for {
		// Look ahead for ] - can only happen on first iteration.
		op := d.scanWhile(scanSkipSpace)
		if op == scanEndArray {
			break
		}

		// Back up so d.value can have the byte we just read.
		d.off--
		d.scan.undo(op)

		// Get element of array, growing if necessary.
		if v.Kind() == reflect.Slice {
			// Grow slice if necessary
			if i >= v.Cap() {
				newcap := v.Cap() + v.Cap()/2
				if newcap < 4 {
					newcap = 4
				}
				newv := reflect.MakeSlice(v.Type(), v.Len(), newcap)
				reflect.Copy(newv, v)
				v.Set(newv)
			}
			if i >= v.Len() {
				v.SetLen(i + 1)
			}
		}

		if i < v.Len() {
			// Decode into element.
			d.value(v.Index(i))
		} else {
			// Ran out of fixed array: skip.
			d.value(reflect.Value{})
		}
		i++

		// Next token must be , or ].
		op = d.scanWhile(scanSkipSpace)
		if op == scanEndArray {
			break
		}
		if op != scanArrayValue {
			d.error(errPhase)
		}
	}

	if i < v.Len() {
		if v.Kind() == reflect.Array {
			// Array. Zero the rest.
			z := reflect.Zero(v.Type().Elem())
			for ; i < v.Len(); i++ {
				v.Index(i).Set(z)
			}
		} else {
			v.SetLen(i)
		}
	}
	if i == 0 && v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
}

var nullLiteral = []byte("null")
var textUnmarshalerType = reflect.TypeOf(new(encoding.TextUnmarshaler)).Elem()

// object consumes an object from d.data[d.off-1:], decoding into the value v.
// the first byte ('{') of the object has been read already.
func (d *decodeState) object(v reflect.Value) {
	// Check for unmarshaler.
	u, ut, pv := d.indirect(v, false)
	if d.storeKeyed(pv) {
		return
	}
	if u != nil {
		d.off--
		err := u.UnmarshalJSON(d.next())
		if err != nil {
			d.error(err)
		}
		return
	}
	if ut != nil {
		d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
		d.off--
		d.next() // skip over { } in input
		return
	}
	v = pv

	// Decoding into nil interface?  Switch to non-reflect code.
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		v.Set(reflect.ValueOf(d.objectInterface()))
		return
	}

	// Check type of target:
	//   struct or
	//   map[string]T or map[encoding.TextUnmarshaler]T
	switch v.Kind() {
	case reflect.Map:
		// Map key must either have string kind or be an encoding.TextUnmarshaler.
		t := v.Type()
		if t.Key().Kind() != reflect.String &&
			!reflect.PtrTo(t.Key()).Implements(textUnmarshalerType) {
			d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
			d.off--
			d.next() // skip over { } in input
			return
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:

	default:
		d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
		d.off--
		d.next() // skip over { } in input
		return
	}

	var mapElem reflect.Value

	empty := true
	for {
		// Read opening " of string key or closing }.
		op := d.scanWhile(scanSkipSpace)
		if op == scanEndObject {
			if !empty && !d.ext.trailingCommas {
				d.syntaxError("beginning of object key string")
			}
			break
		}
		empty = false
		if op == scanBeginName {
			if !d.ext.unquotedKeys {
				d.syntaxError("beginning of object key string")
			}
		} else if op != scanBeginLiteral {
			d.error(errPhase)
		}
		unquotedKey := op == scanBeginName

		// Read key.
		start := d.off - 1
		op = d.scanWhile(scanContinue)
		item := d.data[start : d.off-1]
		var key []byte
		if unquotedKey {
			key = item
			// TODO Fix code below to quote item when necessary.
		} else {
			var ok bool
			key, ok = unquoteBytes(item)
			if !ok {
				d.error(errPhase)
			}
		}

		// Figure out field corresponding to key.
		var subv reflect.Value
		destring := false // whether the value is wrapped in a string to be decoded first

		if v.Kind() == reflect.Map {
			elemType := v.Type().Elem()
			if !mapElem.IsValid() {
				mapElem = reflect.New(elemType).Elem()
			} else {
				mapElem.Set(reflect.Zero(elemType))
			}
			subv = mapElem
		} else {
			var f *field
			fields := cachedTypeFields(v.Type())
			for i := range fields {
				ff := &fields[i]
				if bytes.Equal(ff.nameBytes, key) {
					f = ff
					break
				}
				if f == nil && ff.equalFold(ff.nameBytes, key) {
					f = ff
				}
			}
			if f != nil {
				subv = v
				destring = f.quoted
				for _, i := range f.index {
					if subv.Kind() == reflect.Ptr {
						if subv.IsNil() {
							subv.Set(reflect.New(subv.Type().Elem()))
						}
						subv = subv.Elem()
					}
					subv = subv.Field(i)
				}
			}
		}

		// Read : before value.
		if op == scanSkipSpace {
			op = d.scanWhile(scanSkipSpace)
		}
		if op != scanObjectKey {
			d.error(errPhase)
		}

		// Read value.
		if destring {
			switch qv := d.valueQuoted().(type) {
			case nil:
				d.literalStore(nullLiteral, subv, false)
			case string:
				d.literalStore([]byte(qv), subv, true)
			default:
				d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal unquoted value into %v", subv.Type()))
			}
		} else {
			d.value(subv)
		}

		// Write value back to map;
		// if using struct, subv points into struct already.
		if v.Kind() == reflect.Map {
			kt := v.Type().Key()
			var kv reflect.Value
			switch {
			case kt.Kind() == reflect.String:
				kv = reflect.ValueOf(key).Convert(v.Type().Key())
			case reflect.PtrTo(kt).Implements(textUnmarshalerType):
				kv = reflect.New(v.Type().Key())
				d.literalStore(item, kv, true)
				kv = kv.Elem()
			default:
				panic("json: Unexpected key type") // should never occur
			}
			v.SetMapIndex(kv, subv)
		}

		// Next token must be , or }.
		op = d.scanWhile(scanSkipSpace)
		if op == scanEndObject {
			break
		}
		if op != scanObjectValue {
			d.error(errPhase)
		}
	}
}

// isNull returns whether there's a null literal at the provided offset.
func (d *decodeState) isNull(off int) bool {
	if off+4 >= len(d.data) || d.data[off] != 'n' || d.data[off+1] != 'u' || d.data[off+2] != 'l' || d.data[off+3] != 'l' {
		return false
	}
	d.nextscan.reset()
	for i, c := range d.data[off:] {
		if i > 4 {
			return false
		}
		switch d.nextscan.step(&d.nextscan, c) {
		case scanContinue, scanBeginName:
			continue
		}
		break
	}
	return true
}

// name consumes a const or function from d.data[d.off-1:], decoding into the value v.
// the first byte of the function name has been read already.
func (d *decodeState) name(v reflect.Value) {
	if d.isNull(d.off - 1) {
		d.literal(v)
		return
	}

	// Check for unmarshaler.
	u, ut, pv := d.indirect(v, false)
	if d.storeKeyed(pv) {
		return
	}
	if u != nil {
		d.off--
		err := u.UnmarshalJSON(d.next())
		if err != nil {
			d.error(err)
		}
		return
	}
	if ut != nil {
		d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
		d.off--
		d.next() // skip over function in input
		return
	}
	v = pv

	// Decoding into nil interface?  Switch to non-reflect code.
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		out := d.nameInterface()
		if out == nil {
			v.Set(reflect.Zero(v.Type()))
		} else {
			v.Set(reflect.ValueOf(out))
		}
		return
	}

	nameStart := d.off - 1

	op := d.scanWhile(scanContinue)

	name := d.data[nameStart : d.off-1]
	if op != scanParam {
		// Back up so the byte just read is consumed next.
		d.off--
		d.scan.undo(op)
		if l, ok := d.convertLiteral(name); ok {
			d.storeValue(v, l)
			return
		}
		d.error(&SyntaxError{fmt.Sprintf("json: unknown constant %q", name), int64(d.off)})
	}

	funcName := string(name)
	funcData := d.ext.funcs[funcName]
	if funcData.key == "" {
		d.error(fmt.Errorf("json: unknown function %q", funcName))
	}

	// Check type of target:
	//   struct or
	//   map[string]T or map[encoding.TextUnmarshaler]T
	switch v.Kind() {
	case reflect.Map:
		// Map key must either have string kind or be an encoding.TextUnmarshaler.
		t := v.Type()
		if t.Key().Kind() != reflect.String &&
			!reflect.PtrTo(t.Key()).Implements(textUnmarshalerType) {
			d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
			d.off--
			d.next() // skip over { } in input
			return
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:

	default:
		d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
		d.off--
		d.next() // skip over { } in input
		return
	}

	// TODO Fix case of func field as map.
	//topv := v

	// Figure out field corresponding to function.
	key := []byte(funcData.key)
	if v.Kind() == reflect.Map {
		elemType := v.Type().Elem()
		v = reflect.New(elemType).Elem()
	} else {
		var f *field
		fields := cachedTypeFields(v.Type())
		for i := range fields {
			ff := &fields[i]
			if bytes.Equal(ff.nameBytes, key) {
				f = ff
				break
			}
			if f == nil && ff.equalFold(ff.nameBytes, key) {
				f = ff
			}
		}
		if f != nil {
			for _, i := range f.index {
				if v.Kind() == reflect.Ptr {
					if v.IsNil() {
						v.Set(reflect.New(v.Type().Elem()))
					}
					v = v.Elem()
				}
				v = v.Field(i)
			}
			if v.Kind() == reflect.Ptr {
				if v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
		}
	}

	// Check for unmarshaler on func field itself.
	//nolint:all
	u, ut, pv = d.indirect(v, false)
	if u != nil {
		d.off = nameStart
		err := u.UnmarshalJSON(d.next())
		if err != nil {
			d.error(err)
		}
		return
	}

	var mapElem reflect.Value

	// Parse function arguments.
	for i := 0; ; i++ {
		// closing ) - can only happen on first iteration.
		op := d.scanWhile(scanSkipSpace)
		if op == scanEndParams {
			break
		}

		// Back up so d.value can have the byte we just read.
		d.off--
		d.scan.undo(op)

		if i >= len(funcData.args) {
			d.error(fmt.Errorf("json: too many arguments for function %s", funcName))
		}
		key := []byte(funcData.args[i])

		// Figure out field corresponding to key.
		var subv reflect.Value
		destring := false // whether the value is wrapped in a string to be decoded first

		if v.Kind() == reflect.Map {
			elemType := v.Type().Elem()
			if !mapElem.IsValid() {
				mapElem = reflect.New(elemType).Elem()
			} else {
				mapElem.Set(reflect.Zero(elemType))
			}
			subv = mapElem
		} else {
			var f *field
			fields := cachedTypeFields(v.Type())
			for i := range fields {
				ff := &fields[i]
				if bytes.Equal(ff.nameBytes, key) {
					f = ff
					break
				}
				if f == nil && ff.equalFold(ff.nameBytes, key) {
					f = ff
				}
			}
			if f != nil {
				subv = v
				destring = f.quoted
				for _, i := range f.index {
					if subv.Kind() == reflect.Ptr {
						if subv.IsNil() {
							subv.Set(reflect.New(subv.Type().Elem()))
						}
						subv = subv.Elem()
					}
					subv = subv.Field(i)
				}
			}
		}

		// Read value.
		if destring {
			switch qv := d.valueQuoted().(type) {
			case nil:
				d.literalStore(nullLiteral, subv, false)
			case string:
				d.literalStore([]byte(qv), subv, true)
			default:
				d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal unquoted value into %v", subv.Type()))
			}
		} else {
			d.value(subv)
		}

		// Write value back to map;
		// if using struct, subv points into struct already.
		if v.Kind() == reflect.Map {
			kt := v.Type().Key()
			var kv reflect.Value
			switch {
			case kt.Kind() == reflect.String:
				kv = reflect.ValueOf(key).Convert(v.Type().Key())
			case reflect.PtrTo(kt).Implements(textUnmarshalerType):
				kv = reflect.New(v.Type().Key())
				d.literalStore(key, kv, true)
				kv = kv.Elem()
			default:
				panic("json: Unexpected key type") // should never occur
			}
			v.SetMapIndex(kv, subv)
		}

		// Next token must be , or ).
		op = d.scanWhile(scanSkipSpace)
		if op == scanEndParams {
			break
		}
		if op != scanParam {
			d.error(errPhase)
		}
	}
}

// keyed attempts to decode an object or function using a keyed doc extension,
// and returns the value and true on success, or nil and false otherwise.
func (d *decodeState) keyed() (any, bool) {
	if len(d.ext.keyed) == 0 {
		return nil, false
	}

	unquote := false

	// Look-ahead first key to check for a keyed document extension.
	d.nextscan.reset()
	var start, end int
	for i, c := range d.data[d.off-1:] {
		switch op := d.nextscan.step(&d.nextscan, c); op {
		case scanSkipSpace, scanContinue, scanBeginObject:
			continue
		case scanBeginLiteral, scanBeginName:
			unquote = op == scanBeginLiteral
			start = i
			continue
		}
		end = i
		break
	}

	name := bytes.Trim(d.data[d.off-1+start:d.off-1+end], " \n\t")

	var key []byte
	var ok bool
	if unquote {
		key, ok = unquoteBytes(name)
		if !ok {
			d.error(errPhase)
		}
	} else {
		funcData, ok := d.ext.funcs[string(name)]
		if !ok {
			return nil, false
		}
		key = []byte(funcData.key)
	}

	decode, ok := d.ext.keyed[string(key)]
	if !ok {
		return nil, false
	}

	d.off--
	out, err := decode(d.next())
	if err != nil {
		//nolint:all
		d.error(err)
	}
	return out, true
}

func (d *decodeState) storeKeyed(v reflect.Value) bool {
	keyed, ok := d.keyed()
	if !ok {
		return false
	}
	d.storeValue(v, keyed)
	return true
}

var (
	trueBytes  = []byte("true")
	falseBytes = []byte("false")
	nullBytes  = []byte("null")
)

func (d *decodeState) storeValue(v reflect.Value, from any) {
	switch from {
	case nil:
		d.literalStore(nullBytes, v, false)
		return
	case true:
		d.literalStore(trueBytes, v, false)
		return
	case false:
		d.literalStore(falseBytes, v, false)
		return
	}
	fromv := reflect.ValueOf(from)
	for fromv.Kind() == reflect.Ptr && !fromv.IsNil() {
		fromv = fromv.Elem()
	}
	fromt := fromv.Type()
	for v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}
	vt := v.Type()
	if fromt.AssignableTo(vt) {
		v.Set(fromv)
	} else if fromt.ConvertibleTo(vt) {
		v.Set(fromv.Convert(vt))
	} else {
		d.saveError(&UnmarshalTypeError{"object", v.Type(), int64(d.off)})
	}
}

func (d *decodeState) convertLiteral(name []byte) (any, bool) {
	if len(name) == 0 {
		return nil, false
	}
	switch name[0] {
	case 't':
		if bytes.Equal(name, trueBytes) {
			return true, true
		}
	case 'f':
		if bytes.Equal(name, falseBytes) {
			return false, true
		}
	case 'n':
		if bytes.Equal(name, nullBytes) {
			return nil, true
		}
	}
	if l, ok := d.ext.consts[string(name)]; ok {
		return l, true
	}
	return nil, false
}

// literal consumes a literal from d.data[d.off-1:], decoding into the value v.
// The first byte of the literal has been read already
// (that's how the caller knows it's a literal).
func (d *decodeState) literal(v reflect.Value) {
	// All bytes inside literal return scanContinue op code.
	start := d.off - 1
	op := d.scanWhile(scanContinue)

	// Scan read one byte too far; back up.
	d.off--
	d.scan.undo(op)

	d.literalStore(d.data[start:d.off], v, false)
}

// convertNumber converts the number literal s to a float64 or a Number
// depending on the setting of d.useNumber.
func (d *decodeState) convertNumber(s string) (any, error) {
	if d.useNumber {
		return Number(s), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, &UnmarshalTypeError{"number " + s, reflect.TypeOf(0.0), int64(d.off)}
	}
	return f, nil
}

var numberType = reflect.TypeOf(Number(""))

// literalStore decodes a literal stored in item into v.
//
// fromQuoted indicates whether this literal came from unwrapping a
// string from the ",string" struct tag option. this is used only to
// produce more helpful error messages.
func (d *decodeState) literalStore(item []byte, v reflect.Value, fromQuoted bool) {
	// Check for unmarshaler.
	if len(item) == 0 {
		//Empty string given
		d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
		return
	}
	wantptr := item[0] == 'n' // null
	u, ut, pv := d.indirect(v, wantptr)
	if u != nil {
		err := u.UnmarshalJSON(item)
		if err != nil {
			d.error(err)
		}
		return
	}
	if ut != nil {
		if item[0] != '"' {
			if fromQuoted {
				d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.saveError(&UnmarshalTypeError{"string", v.Type(), int64(d.off)})
			}
			return
		}
		s, ok := unquoteBytes(item)
		if !ok {
			if fromQuoted {
				d.error(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.error(errPhase)
			}
		}
		err := ut.UnmarshalText(s)
		if err != nil {
			d.error(err)
		}
		return
	}

	v = pv

	switch c := item[0]; c {
	case 'n': // null
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice:
			v.Set(reflect.Zero(v.Type()))
			// otherwise, ignore null for primitives/string
		}
	case 't', 'f': // true, false
		value := c == 't'
		switch v.Kind() {
		default:
			if fromQuoted {
				d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.saveError(&UnmarshalTypeError{"bool", v.Type(), int64(d.off)})
			}
		case reflect.Bool:
			v.SetBool(value)
		case reflect.Interface:
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(value))
			} else {
				d.saveError(&UnmarshalTypeError{"bool", v.Type(), int64(d.off)})
			}
		}

	case '"': // string
		s, ok := unquoteBytes(item)
		if !ok {
			if fromQuoted {
				d.error(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.error(errPhase)
			}
		}
		switch v.Kind() {
		default:
			d.saveError(&UnmarshalTypeError{"string", v.Type(), int64(d.off)})
		case reflect.Slice:
			if v.Type().Elem().Kind() != reflect.Uint8 {
				d.saveError(&UnmarshalTypeError{"string", v.Type(), int64(d.off)})
				break
			}
			b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
			n, err := base64.StdEncoding.Decode(b, s)
			if err != nil {
				d.saveError(err)
				break
			}
			v.SetBytes(b[:n])
		case reflect.String:
			v.SetString(string(s))
		case reflect.Interface:
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(string(s)))
			} else {
				d.saveError(&UnmarshalTypeError{"string", v.Type(), int64(d.off)})
			}
		}

	default: // number
		if c != '-' && (c < '0' || c > '9') {
			if fromQuoted {
				d.error(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.error(errPhase)
			}
		}
		s := string(item)
		switch v.Kind() {
		default:
			if v.Kind() == reflect.String && v.Type() == numberType {
				v.SetString(s)
				if !isValidNumber(s) {
					d.error(fmt.Errorf("json: invalid number literal, trying to unmarshal %q into Number", item))
				}
				break
			}
			if fromQuoted {
				d.error(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.error(&UnmarshalTypeError{"number", v.Type(), int64(d.off)})
			}
		case reflect.Interface:
			n, err := d.convertNumber(s)
			if err != nil {
				d.saveError(err)
				break
			}
			if v.NumMethod() != 0 {
				d.saveError(&UnmarshalTypeError{"number", v.Type(), int64(d.off)})
				break
			}
			v.Set(reflect.ValueOf(n))

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil || v.OverflowInt(n) {
				d.saveError(&UnmarshalTypeError{"number " + s, v.Type(), int64(d.off)})
				break
			}
			v.SetInt(n)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			n, err := strconv.ParseUint(s, 10, 64)
			if err != nil || v.OverflowUint(n) {
				d.saveError(&UnmarshalTypeError{"number " + s, v.Type(), int64(d.off)})
				break
			}
			v.SetUint(n)

		case reflect.Float32, reflect.Float64:
			n, err := strconv.ParseFloat(s, v.Type().Bits())
			if err != nil || v.OverflowFloat(n) {
				d.saveError(&UnmarshalTypeError{"number " + s, v.Type(), int64(d.off)})
				break
			}
			v.SetFloat(n)
		}
	}
}

// The xxxInterface routines build up a value to be stored
// in an empty interface. They are not strictly necessary,
// but they avoid the weight of reflection in this common case.

// valueInterface is like value but returns interface{}
func (d *decodeState) valueInterface() any {
	switch d.scanWhile(scanSkipSpace) {
	default:
		d.error(errPhase)
		panic("unreachable")
	case scanBeginArray:
		return d.arrayInterface()
	case scanBeginObject:
		return d.objectInterface()
	case scanBeginLiteral:
		return d.literalInterface()
	case scanBeginName:
		return d.nameInterface()
	}
}

func (d *decodeState) syntaxError(expected string) {
	msg := fmt.Sprintf("invalid character '%c' looking for %s", d.data[d.off-1], expected)
	d.error(&SyntaxError{msg, int64(d.off)})
}

// arrayInterface is like array but returns []interface{}.
func (d *decodeState) arrayInterface() []any {
	var v = make([]any, 0)
	for {
		// Look ahead for ] - can only happen on first iteration.
		op := d.scanWhile(scanSkipSpace)
		if op == scanEndArray {
			if len(v) > 0 && !d.ext.trailingCommas {
				d.syntaxError("beginning of value")
			}
			break
		}

		// Back up so d.value can have the byte we just read.
		d.off--
		d.scan.undo(op)

		v = append(v, d.valueInterface())

		// Next token must be , or ].
		op = d.scanWhile(scanSkipSpace)
		if op == scanEndArray {
			break
		}
		if op != scanArrayValue {
			d.error(errPhase)
		}
	}
	return v
}

// objectInterface is like object but returns map[string]interface{}.
func (d *decodeState) objectInterface() any {
	v, ok := d.keyed()
	if ok {
		return v
	}

	m := make(map[string]any)
	for {
		// Read opening " of string key or closing }.
		op := d.scanWhile(scanSkipSpace)
		if op == scanEndObject {
			if len(m) > 0 && !d.ext.trailingCommas {
				d.syntaxError("beginning of object key string")
			}
			break
		}
		if op == scanBeginName {
			if !d.ext.unquotedKeys {
				d.syntaxError("beginning of object key string")
			}
		} else if op != scanBeginLiteral {
			d.error(errPhase)
		}
		unquotedKey := op == scanBeginName

		// Read string key.
		start := d.off - 1
		op = d.scanWhile(scanContinue)
		item := d.data[start : d.off-1]
		var key string
		if unquotedKey {
			key = string(item)
		} else {
			var ok bool
			key, ok = unquote(item)
			if !ok {
				d.error(errPhase)
			}
		}

		// Read : before value.
		if op == scanSkipSpace {
			op = d.scanWhile(scanSkipSpace)
		}
		if op != scanObjectKey {
			d.error(errPhase)
		}

		// Read value.
		m[key] = d.valueInterface()

		// Next token must be , or }.
		op = d.scanWhile(scanSkipSpace)
		if op == scanEndObject {
			break
		}
		if op != scanObjectValue {
			d.error(errPhase)
		}
	}
	return m
}

// literalInterface is like literal but returns an interface value.
func (d *decodeState) literalInterface() any {
	// All bytes inside literal return scanContinue op code.
	start := d.off - 1
	op := d.scanWhile(scanContinue)

	// Scan read one byte too far; back up.
	d.off--
	d.scan.undo(op)
	item := d.data[start:d.off]

	switch c := item[0]; c {
	case 'n': // null
		return nil

	case 't', 'f': // true, false
		return c == 't'

	case '"': // string
		s, ok := unquote(item)
		if !ok {
			d.error(errPhase)
		}
		return s

	default: // number
		if c != '-' && (c < '0' || c > '9') {
			d.error(errPhase)
		}
		n, err := d.convertNumber(string(item))
		if err != nil {
			d.saveError(err)
		}
		return n
	}
}

// nameInterface is like function but returns map[string]interface{}.
func (d *decodeState) nameInterface() any {
	v, ok := d.keyed()
	if ok {
		return v
	}

	nameStart := d.off - 1

	op := d.scanWhile(scanContinue)

	name := d.data[nameStart : d.off-1]
	if op != scanParam {
		// Back up so the byte just read is consumed next.
		d.off--
		d.scan.undo(op)
		if l, ok := d.convertLiteral(name); ok {
			return l
		}
		d.error(&SyntaxError{fmt.Sprintf("json: unknown constant %q", name), int64(d.off)})
	}

	funcName := string(name)
	funcData := d.ext.funcs[funcName]
	if funcData.key == "" {
		d.error(fmt.Errorf("json: unknown function %q", funcName))
	}

	m := make(map[string]any)
	for i := 0; ; i++ {
		// Look ahead for ) - can only happen on first iteration.
		op := d.scanWhile(scanSkipSpace)
		if op == scanEndParams {
			break
		}

		// Back up so d.value can have the byte we just read.
		d.off--
		d.scan.undo(op)

		if i >= len(funcData.args) {
			d.error(fmt.Errorf("json: too many arguments for function %s", funcName))
		}
		m[funcData.args[i]] = d.valueInterface()

		// Next token must be , or ).
		op = d.scanWhile(scanSkipSpace)
		if op == scanEndParams {
			break
		}
		if op != scanParam {
			d.error(errPhase)
		}
	}
	return map[string]any{funcData.key: m}
}

// getu4 decodes \uXXXX from the beginning of s, returning the hex value,
// or it returns -1.
func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	r, err := strconv.ParseUint(string(s[2:6]), 16, 64)
	if err != nil {
		return -1
	}
	return rune(r)
}

// unquote converts a quoted JSON string literal s into an actual string t.
// The rules are different than for Go, so cannot use strconv.Unquote.
func unquote(s []byte) (t string, ok bool) {
	s, ok = unquoteBytes(s)
	t = string(s)
	return
}

func unquoteBytes(s []byte) (t []byte, ok bool) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return
	}
	s = s[1 : len(s)-1]

	// Check for unusual characters. If there are none,
	// then no unquoting is needed, so return a slice of the
	// original bytes.
	r := 0
	for r < len(s) {
		c := s[r]
		if c == '\\' || c == '"' || c < ' ' {
			break
		}
		if c < utf8.RuneSelf {
			r++
			continue
		}
		rr, size := utf8.DecodeRune(s[r:])
		if rr == utf8.RuneError && size == 1 {
			break
		}
		r += size
	}
	if r == len(s) {
		return s, true
	}

	b := make([]byte, len(s)+2*utf8.UTFMax)
	w := copy(b, s[0:r])
	for r < len(s) {
		// Out of room?  Can only happen if s is full of
		// malformed UTF-8 and we're replacing each
		// byte with RuneError.
		if w >= len(b)-2*utf8.UTFMax {
			nb := make([]byte, (len(b)+utf8.UTFMax)*2)
			copy(nb, b[0:w])
			b = nb
		}
		switch c := s[r]; {
		case c == '\\':
			r++
			if r >= len(s) {
				return
			}
			switch s[r] {
			default:
				return
			case '"', '\\', '/', '\'':
				b[w] = s[r]
				r++
				w++
			case 'b':
				b[w] = '\b'
				r++
				w++
			case 'f':
				b[w] = '\f'
				r++
				w++
			case 'n':
				b[w] = '\n'
				r++
				w++
			case 'r':
				b[w] = '\r'
				r++
				w++
			case 't':
				b[w] = '\t'
				r++
				w++
			case 'u':
				r--
				rr := getu4(s[r:])
				if rr < 0 {
					return
				}
				r += 6
				if utf16.IsSurrogate(rr) {
					rr1 := getu4(s[r:])
					if dec := utf16.DecodeRune(rr, rr1); dec != unicode.ReplacementChar {
						// A valid pair; consume.
						r += 6
						w += utf8.EncodeRune(b[w:], dec)
						break
					}
					// Invalid surrogate; fall back to replacement rune.
					rr = unicode.ReplacementChar
				}
				w += utf8.EncodeRune(b[w:], rr)
			}

		// Quote, control characters are invalid.
		case c == '"', c < ' ':
			return

		// ASCII
		case c < utf8.RuneSelf:
			b[w] = c
			r++
			w++

		// Coerce to well-formed UTF-8.
		default:
			rr, size := utf8.DecodeRune(s[r:])
			r += size
			w += utf8.EncodeRune(b[w:], rr)
		}
	}
	return b[0:w], true
}

// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/evergreen-ci/birch/bsonerr"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/evergreen-ci/birch/decimal"
	"github.com/evergreen-ci/birch/types"
	"github.com/pkg/errors"
)

// Value represents a BSON value. It can be obtained as part of a bson.Element or created for use
// in a bson.Array with the bson.VC constructors.
type Value struct {
	// NOTE: For subdocuments, arrays, and code with scope, the data slice of
	// bytes may contain just the key, or the key and the code in the case of
	// code with scope. If this is the case, the start will be 0, the value will
	// be the length of the slice, and d will be non-nil.

	// start is the offset into the data slice of bytes where this element
	// begins.
	start uint32
	// offset is the offset into the data slice of bytes where this element's
	// value begins.
	offset uint32
	// data is a potentially shared slice of bytes that contains the actual
	// element. Most of the methods of this type directly index into this slice
	// of bytes.
	data []byte

	d *Document
}

func (v *Value) Copy() *Value {
	return &Value{
		start:  v.start,
		offset: v.offset,
		data:   v.data,
		d:      v.d.Copy(),
	}
}

// Interface returns the Go value of this Value as an empty interface.
//
// For embedded documents and arrays, Interface will convert the
// elements to map[string]interface{} or []interface{} as possible.
//
// The underlying types of the values returned by this method are
// their native corresponding type when possible.
func (v *Value) Interface() interface{} {
	if v == nil {
		return nil
	}

	switch v.Type() {
	case bsontype.Double:
		return v.Double()
	case bsontype.String:
		return v.StringValue()
	case bsontype.EmbeddedDocument:
		return v.MutableDocument().ExportMap()
	case bsontype.Array:
		return v.MutableArray().Interface()
	case bsontype.Binary:
		_, data := v.Binary()
		return data
	case bsontype.Undefined:
		return nil
	case bsontype.ObjectID:
		return v.ObjectID()
	case bsontype.Boolean:
		return v.Boolean()
	case bsontype.DateTime:
		return v.DateTime()
	case bsontype.Null:
		return nil
	case bsontype.Regex:
		p, o := v.Regex()
		return types.Regex{Pattern: p, Options: o}
	case bsontype.DBPointer:
		db, pointer := v.DBPointer()
		return types.DBPointer{DB: db, Pointer: pointer}
	case bsontype.JavaScript:
		return v.JavaScript()
	case bsontype.Symbol:
		return v.Symbol()
	case bsontype.CodeWithScope:
		code, scope := v.MutableJavaScriptWithScope()
		val, _ := scope.MarshalBSON()
		return types.CodeWithScope{Code: code, Scope: val}
	case bsontype.Int32:
		return v.Int32()
	case bsontype.Timestamp:
		t, i := v.Timestamp()
		return types.Timestamp{T: t, I: i}
	case bsontype.Int64:
		return v.Int64()
	case bsontype.Decimal128:
		return v.Decimal128()
	case bsontype.MinKey:
		return nil
	case bsontype.MaxKey:
		return nil
	default:
		return nil
	}
}

// Validate validates the value.
func (v *Value) Validate() error {
	_, err := v.validate(false)
	return err
}

func (v *Value) validate(sizeOnly bool) (uint32, error) {
	if v.data == nil {
		return 0, bsonerr.UninitializedElement
	}

	var total uint32

	switch v.data[v.start] {
	case '\x06', '\x0A', '\xFF', '\x7F':
	case '\x01':
		if int(v.offset+8) > len(v.data) {
			return total, newErrTooSmall()
		}
		total += 8
	case '\x02', '\x0D', '\x0E':
		if int(v.offset+4) > len(v.data) {
			return total, newErrTooSmall()
		}
		l := readi32(v.data[v.offset : v.offset+4])
		total += 4
		if int32(v.offset)+4+l > int32(len(v.data)) {
			return total, newErrTooSmall()
		}
		// We check if the value that is the last element of the string is a
		// null terminator. We take the value offset, add 4 to account for the
		// length, add the length of the string, and subtract one since the size
		// isn't zero indexed.
		if !sizeOnly && v.data[v.offset+4+uint32(l)-1] != 0x00 {
			return total, bsonerr.InvalidString
		}
		total += uint32(l)
	case '\x03':
		if v.d != nil {
			n, err := v.d.Validate()
			total += n
			if err != nil {
				return total, err
			}
			break
		}

		if int(v.offset+4) > len(v.data) {
			return total, newErrTooSmall()
		}
		l := readi32(v.data[v.offset : v.offset+4])
		total += 4
		if l < 5 {
			return total, bsonerr.InvalidReadOnlyDocument
		}
		if int32(v.offset)+l > int32(len(v.data)) {
			return total, newErrTooSmall()
		}
		if !sizeOnly {
			n, err := Reader(v.data[v.offset : v.offset+uint32(l)]).Validate()
			total += n - 4
			if err != nil {
				return total, err
			}
			break
		}
		total += uint32(l) - 4
	case '\x04':
		if v.d != nil {
			n, err := (&Array{v.d}).Validate()
			total += n
			if err != nil {
				return total, err
			}
			break
		}

		if int(v.offset+4) > len(v.data) {
			return total, newErrTooSmall()
		}
		l := readi32(v.data[v.offset : v.offset+4])
		total += 4
		if l < 5 {
			return total, bsonerr.InvalidReadOnlyDocument
		}
		if int32(v.offset)+l > int32(len(v.data)) {
			return total, newErrTooSmall()
		}
		if !sizeOnly {
			n, err := Reader(v.data[v.offset : v.offset+uint32(l)]).Validate()
			total += n - 4
			if err != nil {
				return total, err
			}
			break
		}
		total += uint32(l) - 4
	case '\x05':
		if int(v.offset+5) > len(v.data) {
			return total, newErrTooSmall()
		}
		l := readi32(v.data[v.offset : v.offset+4])
		total += 5
		if v.data[v.offset+4] > '\x05' && v.data[v.offset+4] < '\x80' {
			return total, bsonerr.InvalidBinarySubtype
		}
		if int32(v.offset)+5+l > int32(len(v.data)) {
			return total, newErrTooSmall()
		}
		total += uint32(l)
	case '\x07':
		if int(v.offset+12) > len(v.data) {
			return total, newErrTooSmall()
		}
		total += 12
	case '\x08':
		if int(v.offset+1) > len(v.data) {
			return total, newErrTooSmall()
		}
		total++
		if v.data[v.offset] != '\x00' && v.data[v.offset] != '\x01' {
			return total, bsonerr.InvalidBooleanType
		}
	case '\x09':
		if int(v.offset+8) > len(v.data) {
			return total, newErrTooSmall()
		}
		total += 8
	case '\x0B':
		i := v.offset
		for ; int(i) < len(v.data) && v.data[i] != '\x00'; i++ {
			total++
		}
		if int(i) == len(v.data) || v.data[i] != '\x00' {
			return total, bsonerr.InvalidString
		}
		i++
		total++
		for ; int(i) < len(v.data) && v.data[i] != '\x00'; i++ {
			total++
		}
		if int(i) == len(v.data) || v.data[i] != '\x00' {
			return total, bsonerr.InvalidString
		}
		total++
	case '\x0C':
		if int(v.offset+4) > len(v.data) {
			return total, newErrTooSmall()
		}
		l := readi32(v.data[v.offset : v.offset+4])
		total += 4
		if int32(v.offset)+4+l+12 > int32(len(v.data)) {
			return total, newErrTooSmall()
		}
		total += uint32(l) + 12
	case '\x0F':
		if v.d != nil {
			// NOTE: For code with scope specifically, we write the length as
			// we are marshaling the element and the constructor doesn't know
			// the length of the document when it constructs the element.
			// Because of that we don't check the length here and just validate
			// the string and the document.
			if int(v.offset+8) > len(v.data) {
				return total, newErrTooSmall()
			}
			total += 8
			sLength := readi32(v.data[v.offset+4 : v.offset+8])
			if int(sLength) > len(v.data)+8 {
				return total, newErrTooSmall()
			}
			total += uint32(sLength)
			if !sizeOnly && v.data[v.offset+8+uint32(sLength)-1] != 0x00 {
				return total, bsonerr.InvalidString
			}

			n, err := v.d.Validate()
			total += n
			if err != nil {
				return total, err
			}
			break
		}
		if int(v.offset+4) > len(v.data) {
			return total, newErrTooSmall()
		}
		l := readi32(v.data[v.offset : v.offset+4])
		total += 4
		if int32(v.offset)+l > int32(len(v.data)) {
			return total, newErrTooSmall()
		}
		if !sizeOnly {
			sLength := readi32(v.data[v.offset+4 : v.offset+8])
			total += 4
			// If the length of the string is larger than the total length of the
			// field minus the int32 for length, 5 bytes for a minimum document
			// size, and an int32 for the string length the value is invalid.
			//
			// TODO(skriptble): We should actually validate that the string
			// doesn't consume any of the bytes used by the document.
			if sLength > l-13 {
				return total, bsonerr.StringLargerThanContainer
			}
			// We check if the value that is the last element of the string is a
			// null terminator. We take the value offset, add 4 to account for the
			// length, add the length of the string, and subtract one since the size
			// isn't zero indexed.
			if v.data[v.offset+8+uint32(sLength)-1] != 0x00 {
				return total, bsonerr.InvalidString
			}
			total += uint32(sLength)
			n, err := Reader(v.data[v.offset+8+uint32(sLength) : v.offset+uint32(l)]).Validate()
			total += n
			if err != nil {
				return total, err
			}
			break
		}
		total += uint32(l) - 4
	case '\x10':
		if int(v.offset+4) > len(v.data) {
			return total, newErrTooSmall()
		}
		total += 4
	case '\x11', '\x12':
		if int(v.offset+8) > len(v.data) {
			return total, newErrTooSmall()
		}
		total += 8
	case '\x13':
		if int(v.offset+16) > len(v.data) {
			return total, newErrTooSmall()
		}
		total += 16

	default:
		return total, bsonerr.InvalidElement
	}

	return total, nil
}

// valueSize returns the size of the value in bytes.
func (v *Value) valueSize() (uint32, error) {
	return v.validate(true)
}

// Type returns the identifying element byte for this element.
// It panics if e is uninitialized.
func (v *Value) Type() bsontype.Type {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	return bsontype.Type(v.data[v.start])
}

// Double returns the float64 value for this element.
// It panics if e's BSON type is not double ('\x01') or if e is uninitialized.
func (v *Value) Double() float64 {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x01' {
		panic(bsonerr.ElementType{"compact.Element.double", bsontype.Type(v.data[v.start])})
	}
	return math.Float64frombits(v.getUint64())
}

// DoubleOK is the same as Double, but returns a boolean instead of panicking.
func (v *Value) DoubleOK() (float64, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Double {
		return 0, false
	}
	return v.Double(), true
}

// StringValue returns the string balue for this element.
// It panics if e's BSON type is not StringValue ('\x02') or if e is uninitialized.
//
// NOTE: This method is called StringValue to avoid it implementing the
// fmt.Stringer interface.
func (v *Value) StringValue() string {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x02' {
		panic(bsonerr.ElementType{"compact.Element.String", bsontype.Type(v.data[v.start])})
	}
	l := readi32(v.data[v.offset : v.offset+4])
	return string(v.data[v.offset+4 : int32(v.offset)+4+l-1])
}

// StringValueOK is the same as StringValue, but returns a boolean instead of
// panicking.
func (v *Value) StringValueOK() (string, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.String {
		return "", false
	}
	return v.StringValue(), true
}

// ReaderDocument returns the BSON document the Value represents as a bson.Reader. It panics if the
// value is a BSON type other than document.
func (v *Value) ReaderDocument() Reader {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}

	if v.data[v.start] != '\x03' {
		panic(bsonerr.ElementType{"compact.Element.Document", bsontype.Type(v.data[v.start])})
	}

	return v.getReader()
}

// Reader returns a reader for the payload of the value regardless of
// type, panicing only if the value is not initialized or otherwise
// corrupt.
func (v *Value) Reader() Reader {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}

	return v.getReader()
}

func (v *Value) getReader() Reader {
	var r Reader
	if v.d == nil {
		l := readi32(v.data[v.offset : v.offset+4])
		r = Reader(v.data[v.offset : v.offset+uint32(l)])
	} else {
		scope, err := v.d.MarshalBSON()
		if err != nil {
			panic(err)
		}

		r = Reader(scope)
	}

	return r
}

// ReaderDocumentOK is the same as ReaderDocument, except it returns a boolean
// instead of panicking.
func (v *Value) ReaderDocumentOK() (Reader, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.EmbeddedDocument {
		return nil, false
	}
	return v.ReaderDocument(), true
}

// MutableDocument returns the subdocument for this element.
func (v *Value) MutableDocument() *Document {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x03' {
		panic(bsonerr.ElementType{"compact.Element.Document", bsontype.Type(v.data[v.start])})
	}
	if v.d == nil {
		var err error
		l := int32(binary.LittleEndian.Uint32(v.data[v.offset : v.offset+4]))
		v.d, err = ReadDocument(v.data[v.offset : v.offset+uint32(l)])
		if err != nil {
			panic(err)
		}
	}
	return v.d
}

// MutableDocumentOK is the same as MutableDocument, except it returns a boolean
// instead of panicking.
func (v *Value) MutableDocumentOK() (*Document, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.EmbeddedDocument {
		return nil, false
	}
	return v.MutableDocument(), true
}

// ReaderArray returns the BSON document the Value represents as a bson.Reader. It panics if the
// value is a BSON type other than array.
func (v *Value) ReaderArray() Reader {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}

	if v.data[v.start] != '\x04' {
		panic(bsonerr.ElementType{"compact.Element.Array", bsontype.Type(v.data[v.start])})
	}

	return v.getReader()
}

// ReaderArrayOK is the same as ReaderArray, except it returns a boolean instead
// of panicking.
func (v *Value) ReaderArrayOK() (Reader, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Array {
		return nil, false
	}
	return v.ReaderArray(), true
}

// MutableArray returns the array for this element.
func (v *Value) MutableArray() *Array {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x04' {
		panic(bsonerr.ElementType{"compact.Element.Array", bsontype.Type(v.data[v.start])})
	}
	if v.d == nil {
		var err error
		l := int32(binary.LittleEndian.Uint32(v.data[v.offset : v.offset+4]))
		v.d, err = ReadDocument(v.data[v.offset : v.offset+uint32(l)])
		if err != nil {
			panic(err)
		}
	}
	return &Array{v.d}
}

// MutableArrayOK is the same as MutableArray, except it returns a boolean
// instead of panicking.
func (v *Value) MutableArrayOK() (*Array, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Array {
		return nil, false
	}
	return v.MutableArray(), true
}

// Binary returns the BSON binary value the Value represents. It panics if the value is a BSON type
// other than binary.
func (v *Value) Binary() (subtype byte, data []byte) {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x05' {
		panic(bsonerr.ElementType{"compact.Element.binary", bsontype.Type(v.data[v.start])})
	}
	l := readi32(v.data[v.offset : v.offset+4])
	st := v.data[v.offset+4]
	offset := uint32(5)
	if st == 0x02 {
		offset += 4
		l = readi32(v.data[v.offset+5 : v.offset+9])
	}
	b := make([]byte, l)
	copy(b, v.data[v.offset+offset:int32(v.offset)+int32(offset)+l])
	return st, b
}

// BinaryOK is the same as Binary, except it returns a boolean instead of
// panicking.
func (v *Value) BinaryOK() (subtype byte, data []byte, ok bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Binary {
		return 0x00, nil, false
	}
	st, b := v.Binary()
	return st, b, true
}

// ObjectID returns the BSON objectid value the Value represents. It panics if the value is a BSON
// type other than objectid.
func (v *Value) ObjectID() types.ObjectID {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x07' {
		panic(bsonerr.ElementType{"compact.Element.ObejctID", bsontype.Type(v.data[v.start])})
	}
	var arr [12]byte
	copy(arr[:], v.data[v.offset:v.offset+12])
	return arr
}

// ObjectIDOK is the same as ObjectID, except it returns a boolean instead of
// panicking.
func (v *Value) ObjectIDOK() (types.ObjectID, bool) {
	var empty types.ObjectID
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.ObjectID {
		return empty, false
	}
	return v.ObjectID(), true
}

// Boolean returns the boolean value the Value represents. It panics if the
// value is a BSON type other than boolean.
func (v *Value) Boolean() bool {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x08' {
		panic(bsonerr.ElementType{"compact.Element.Boolean", bsontype.Type(v.data[v.start])})
	}
	return v.data[v.offset] == '\x01'
}

// BooleanOK is the same as Boolean, except it returns a boolean instead of
// panicking.
func (v *Value) BooleanOK() (bool, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Boolean {
		return false, false
	}
	return v.Boolean(), true
}

// DateTime returns the BSON datetime value the Value represents as a
// unix timestamp. It panics if the value is a BSON type other than datetime.
func (v *Value) DateTime() int64 {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x09' {
		panic(bsonerr.ElementType{"compact.Element.dateTime", bsontype.Type(v.data[v.start])})
	}
	return int64(v.getUint64())
}

// Time returns the BSON datetime value the Value represents. It panics if the value is a BSON
// type other than datetime.
func (v *Value) Time() time.Time {
	i := v.DateTime()
	return time.Unix(i/1000, i%1000*1000000)
}

// TimeOK is the same as Time, except it returns a boolean instead of
// panicking.
func (v *Value) TimeOK() (time.Time, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.DateTime {
		return time.Time{}, false
	}
	return v.Time(), true
}

// Regex returns the BSON regex value the Value represents. It panics if the value is a BSON
// type other than regex.
func (v *Value) Regex() (pattern, options string) {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x0B' {
		panic(bsonerr.ElementType{"compact.Element.regex", bsontype.Type(v.data[v.start])})
	}
	// TODO(skriptble): Use the elements package here.
	var pstart, pend, ostart, oend uint32
	i := v.offset
	pstart = i
	for ; v.data[i] != '\x00'; i++ {
	}
	pend = i
	i++
	ostart = i
	for ; v.data[i] != '\x00'; i++ {
	}
	oend = i

	return string(v.data[pstart:pend]), string(v.data[ostart:oend])
}

// DateTimeOK is the same as DateTime, except it returns a boolean instead of
// panicking.
func (v *Value) DateTimeOK() (int64, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.DateTime {
		return 0, false
	}
	return v.DateTime(), true
}

// DBPointer returns the BSON dbpointer value the Value represents. It panics if the value is a BSON
// type other than DBPointer.
func (v *Value) DBPointer() (string, types.ObjectID) {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x0C' {
		panic(bsonerr.ElementType{"compact.Element.dbPointer", bsontype.Type(v.data[v.start])})
	}
	l := readi32(v.data[v.offset : v.offset+4])
	var p [12]byte
	copy(p[:], v.data[v.offset+4+uint32(l):v.offset+4+uint32(l)+12])
	return string(v.data[v.offset+4 : int32(v.offset)+4+l-1]), p
}

// DBPointerOK is the same as DBPoitner, except that it returns a boolean
// instead of panicking.
func (v *Value) DBPointerOK() (string, types.ObjectID, bool) {
	var empty types.ObjectID
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.DBPointer {
		return "", empty, false
	}
	s, o := v.DBPointer()
	return s, o, true
}

// JavaScript returns the BSON JavaScript code value the Value represents. It panics if the value is
// a BSON type other than JavaScript code.
func (v *Value) JavaScript() string {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x0D' {
		panic(bsonerr.ElementType{"compact.Element.JavaScript", bsontype.Type(v.data[v.start])})
	}
	l := readi32(v.data[v.offset : v.offset+4])
	return string(v.data[v.offset+4 : int32(v.offset)+4+l-1])
}

// JavaScriptOK is the same as Javascript, excepti that it returns a boolean
// instead of panicking.
func (v *Value) JavaScriptOK() (string, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.JavaScript {
		return "", false
	}
	return v.JavaScript(), true
}

// Symbol returns the BSON symbol value the Value represents. It panics if the value is a BSON
// type other than symbol.
func (v *Value) Symbol() string {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x0E' {
		panic(bsonerr.ElementType{"compact.Element.symbol", bsontype.Type(v.data[v.start])})
	}
	l := readi32(v.data[v.offset : v.offset+4])
	return string(v.data[v.offset+4 : int32(v.offset)+4+l-1])
}

// ReaderJavaScriptWithScope returns the BSON JavaScript code with scope the Value represents, with
// the scope being returned as a bson.Reader. It panics if the value is a BSON type other than
// JavaScript code with scope.
func (v *Value) ReaderJavaScriptWithScope() (string, Reader) {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}

	if v.data[v.start] != '\x0F' {
		panic(bsonerr.ElementType{"compact.Element.JavaScriptWithScope", bsontype.Type(v.data[v.start])})
	}

	sLength := readi32(v.data[v.offset+4 : v.offset+8])
	// If the length of the string is larger than the total length of the
	// field minus the int32 for length, 5 bytes for a minimum document
	// size, and an int32 for the string length the value is invalid.
	str := string(v.data[v.offset+8 : v.offset+8+uint32(sLength)-1])

	var r Reader
	if v.d == nil {
		l := readi32(v.data[v.offset : v.offset+4])
		r = Reader(v.data[v.offset+8+uint32(sLength) : v.offset+uint32(l)])
	} else {
		scope, err := v.d.MarshalBSON()
		if err != nil {
			panic(err)
		}

		r = Reader(scope)
	}

	return str, r
}

// ReaderJavaScriptWithScopeOK is the same as ReaderJavaScriptWithScope,
// except that it returns a boolean instead of panicking.
func (v *Value) ReaderJavaScriptWithScopeOK() (string, Reader, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.CodeWithScope {
		return "", nil, false
	}
	s, r := v.ReaderJavaScriptWithScope()
	return s, r, true
}

// MutableJavaScriptWithScope returns the javascript code and the scope document for
// this element.
func (v *Value) MutableJavaScriptWithScope() (code string, d *Document) {
	if v == nil || v.offset == 0 {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x0F' {
		panic(bsonerr.ElementType{"compact.Element.JavaScriptWithScope", bsontype.Type(v.data[v.start])})
	}
	// TODO(skriptble): This is wrong and could cause a panic.
	l := int32(binary.LittleEndian.Uint32(v.data[v.offset : v.offset+4]))
	// TODO(skriptble): This is wrong and could cause a panic.
	sLength := int32(binary.LittleEndian.Uint32(v.data[v.offset+4 : v.offset+8]))
	// If the length of the string is larger than the total length of the
	// field minus the int32 for length, 5 bytes for a minimum document
	// size, and an int32 for the string length the value is invalid.
	str := string(v.data[v.offset+4+4 : v.offset+4+4+uint32(sLength)-1]) // offset + total length + string length + bytes - null byte
	if v.d == nil {
		var err error
		v.d, err = ReadDocument(v.data[v.offset+4+4+uint32(sLength) : v.offset+uint32(l)])
		if err != nil {
			panic(err)
		}
	}
	return str, v.d
}

// MutableJavaScriptWithScopeOK is the same as MutableJavascriptWithScope,
// except that it returns a boolean instead of panicking.
func (v *Value) MutableJavaScriptWithScopeOK() (string, *Document, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.CodeWithScope {
		return "", nil, false
	}
	s, d := v.MutableJavaScriptWithScope()
	return s, d, true
}

// Int32 returns the int32 the Value represents. It panics if the value is a BSON type other than
// int32.
func (v *Value) Int32() int32 {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x10' {
		panic(bsonerr.ElementType{"compact.Element.int32", bsontype.Type(v.data[v.start])})
	}
	return readi32(v.data[v.offset : v.offset+4])
}

// Int32OK is the same as Int32, except that it returns a boolean instead of
// panicking.
func (v *Value) Int32OK() (int32, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Int32 {
		return 0, false
	}
	return v.Int32(), true
}

// Timestamp returns the BSON timestamp value the Value represents. It panics if the value is a
// BSON type other than timestamp.
func (v *Value) Timestamp() (uint32, uint32) {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if bsontype.Type(v.data[v.start]) != bsontype.Timestamp {
		panic(bsonerr.ElementType{"compact.Element.timestamp", bsontype.Type(v.data[v.start])})
	}
	return binary.LittleEndian.Uint32(v.data[v.offset+4 : v.offset+8]), binary.LittleEndian.Uint32(v.data[v.offset : v.offset+4])
}

// TimestampOK is the same as Timestamp, except that it returns a boolean
// instead of panicking.
func (v *Value) TimestampOK() (uint32, uint32, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Timestamp {
		return 0, 0, false
	}
	t, i := v.Timestamp()
	return t, i, true
}

// Int64 returns the int64 the Value represents. It panics if the value is a BSON type other than
// int64.
func (v *Value) Int64() int64 {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x12' {
		panic(bsonerr.ElementType{"compact.Element.int64Type", bsontype.Type(v.data[v.start])})
	}
	return int64(v.getUint64())
}

func (v *Value) getUint64() uint64 {
	return binary.LittleEndian.Uint64(v.data[v.offset : v.offset+8])
}

func (v *Value) Int() int {
	if val, ok := v.Int32OK(); ok {
		return int(val)
	}

	if val, ok := v.Int64OK(); ok {
		return int(val)
	}

	panic(bsonerr.ElementType{"int", bsontype.Type(v.data[v.start])})
}

func (v *Value) IntOK() (int, bool) {
	if v == nil || v.offset == 0 || v.data == nil {
		return 0, false
	}

	if t := bsontype.Type(v.data[v.start]); t != bsontype.Int64 && t != bsontype.Int32 {
		return 0, false
	}

	return v.Int(), true
}

// Int64OK is the same as Int64, except that it returns a boolean instead of
// panicking.
func (v *Value) Int64OK() (int64, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Int64 {
		return 0, false
	}
	return v.Int64(), true
}

// Decimal128 returns the decimal the Value represents. It panics if the value is a BSON type other than
// decimal.
func (v *Value) Decimal128() decimal.Decimal128 {
	if v == nil || v.offset == 0 || v.data == nil {
		panic(bsonerr.UninitializedElement)
	}
	if v.data[v.start] != '\x13' {
		panic(bsonerr.ElementType{"compact.Element.Decimal128", bsontype.Type(v.data[v.start])})
	}
	l := binary.LittleEndian.Uint64(v.data[v.offset : v.offset+8])
	h := binary.LittleEndian.Uint64(v.data[v.offset+8 : v.offset+16])
	return decimal.NewDecimal128(h, l)
}

// Decimal128OK is the same as Decimal128, except that it returns a boolean
// instead of panicking.
func (v *Value) Decimal128OK() (decimal.Decimal128, bool) {
	if v == nil || v.offset == 0 || v.data == nil || bsontype.Type(v.data[v.start]) != bsontype.Decimal128 {
		return decimal.NewDecimal128(0, 0), false
	}
	return v.Decimal128(), true
}

// Equal compares v to v2 and returns true if they are equal. This method will
// ensure that the values are logically equal, even if their internal structure
// is different. This method should be used over reflect.DeepEqual which will
// not return true for Values that are logically the same but not internally the
// same.
func (v *Value) Equal(v2 *Value) bool {
	if v == nil && v2 == nil {
		return true
	}

	if v == nil || v2 == nil {
		return false
	}

	if v.data[v.start] != v2.data[v2.start] {
		return false
	}

	t1, t2 := bsontype.Type(v.data[v.start]), bsontype.Type(v2.data[v2.start])

	data1, err := v.docToBytes(t1)
	if err != nil {
		return false
	}
	data2, err := v2.docToBytes(t2)
	if err != nil {
		return false
	}

	return checkEqualVal(t1, t2, data1, data2)
}

func (v *Value) docToBytes(t bsontype.Type) ([]byte, error) {
	if v.d == nil {
		return v.data[v.offset:], nil
	}

	switch t {
	case bsontype.EmbeddedDocument:
		return v.d.MarshalBSON()
	case bsontype.Array:
		return (&Array{doc: v.d}).MarshalBSON()
	case bsontype.CodeWithScope:
		scope, err := v.d.MarshalBSON()
		if err != nil {
			return nil, err
		}
		code, _, ok := readJavaScriptValue(v.data[v.offset+4:])
		if !ok {
			return nil, errors.New("invalid code component")
		}
		return appendCodeWithScope(nil, code, scope), nil
	default:
		return v.data[v.offset:], nil
	}
}

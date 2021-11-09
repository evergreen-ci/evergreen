// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/evergreen-ci/birch/bsontype"
	"github.com/evergreen-ci/birch/decimal"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireElementsEqual(t *testing.T, expected *Element, actual *Element) {
	requireValuesEqual(t, expected.value, actual.value)
}

func requireValuesEqual(t *testing.T, expected *Value, actual *Value) {
	require.Equal(t, expected.start, actual.start)
	require.Equal(t, expected.offset, actual.offset)

	require.True(t, bytes.Equal(expected.data, actual.data))

	if expected.d == nil {
		require.Nil(t, actual.d)
	} else {
		require.NotNil(t, actual.d)
		require.Equal(t, expected.d.IgnoreNilInsert, actual.d.IgnoreNilInsert)

		require.Equal(t, len(expected.d.elems), len(actual.d.elems))
		for i := range expected.d.elems {
			requireElementsEqual(t, expected.d.elems[i], actual.d.elems[i])
		}

		require.Equal(t, len(expected.d.index), len(actual.d.index))
		for i := range expected.d.index {
			require.Equal(t, expected.d.index[i], actual.d.index[i])
		}
	}
}

func TestConstructor(t *testing.T) {
	t.Run("Document", func(t *testing.T) {
		t.Run("double", func(t *testing.T) {
			buf := []byte{
				// type
				0x1,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0x6e, 0x86, 0x1b, 0xf0, 0xf9,
				0x21, 0x9, 0x40,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Double("foo", 3.14159)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("String", func(t *testing.T) {
			buf := []byte{
				// type
				0x2,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - string length
				0x4, 0x0, 0x0, 0x0,
				// value - string
				0x62, 0x61, 0x72, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.String("foo", "bar")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("SubDocument", func(t *testing.T) {
			buf := []byte{
				// type
				0x3,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}
			d := NewDocument(EC.String("bar", "baz"))

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: d}}
			actual := EC.SubDocument("foo", d)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("SubDocumentFromElements", func(t *testing.T) {
			buf := []byte{
				// type
				0x3,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}
			e := EC.String("bar", "baz")
			d := NewDocument(e)

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: d}}
			actual := EC.SubDocumentFromElements("foo", e)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("SubDocumentFromReader", func(t *testing.T) {
			buf := []byte{
				// type
				0x3,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				0x0A, 0x00, 0x00, 0x00,
				0x0A, 'b', 'a', 'r', 0x00,
				0x00,
			}
			rdr := Reader{
				0x0A, 0x00, 0x00, 0x00,
				0x0A, 'b', 'a', 'r', 0x00,
				0x00,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf}}
			actual := EC.SubDocumentFromReader("foo", rdr)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("Array", func(t *testing.T) {
			buf := []byte{
				// type
				0x4,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}
			a := NewArray(VC.String("bar"), VC.Double(-2.7))

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: a.doc}}
			actual := EC.Array("foo", a)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("ArrayFromElements", func(t *testing.T) {
			buf := []byte{
				// type
				0x4,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}
			e1 := VC.String("bar")
			e2 := VC.Double(-2.7)
			a := NewArray(e1, e2)

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: a.doc}}
			actual := EC.ArrayFromElements("foo", e1, e2)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("binary", func(t *testing.T) {
			buf := []byte{
				// type
				0x5,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - binary length
				0x7, 0x0, 0x0, 0x0,
				// value - binary subtype
				0x0,
				// value - binary data
				0x8, 0x6, 0x7, 0x5, 0x3, 0x0, 0x9,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Binary("foo", []byte{8, 6, 7, 5, 3, 0, 9})

			requireElementsEqual(t, expected, actual)
		})

		t.Run("BinaryWithSubtype", func(t *testing.T) {
			buf := []byte{
				// type
				0x5,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - binary length
				0xb, 0x0, 0x0, 0x0,
				// value - binary subtype
				0x2,
				//
				0x07, 0x00, 0x00, 0x00,
				// value - binary data
				0x8, 0x6, 0x7, 0x5, 0x3, 0x0, 0x9,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.BinaryWithSubtype("foo", []byte{8, 6, 7, 5, 3, 0, 9}, 2)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("undefined", func(t *testing.T) {
			buf := []byte{
				// type
				0x6,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Undefined("foo")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("objectID", func(t *testing.T) {
			buf := []byte{
				// type
				0x7,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.ObjectID(
				"foo",
				[12]byte{0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89},
			)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("Boolean", func(t *testing.T) {
			buf := []byte{
				// type
				0x8,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Boolean("foo", false)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("dateTime", func(t *testing.T) {
			buf := []byte{
				// type
				0x9,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.DateTime("foo", 17)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("time", func(t *testing.T) {
			buf := []byte{
				// type
				0x9,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0xC9, 0x6C, 0x3C, 0xAF, 0x60, 0x1, 0x0, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}

			date := time.Date(2018, 1, 1, 1, 1, 1, int(1*time.Millisecond), time.UTC)
			actualTime := EC.Time("foo", date)
			actualDateTime := EC.DateTime("foo", date.UnixNano()/1e6)

			requireElementsEqual(t, expected, actualTime)
			requireElementsEqual(t, expected, actualDateTime)
		})

		t.Run("Null", func(t *testing.T) {
			buf := []byte{
				// type
				0xa,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Null("foo")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("regex", func(t *testing.T) {
			buf := []byte{
				// type
				0xb,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - pattern
				0x62, 0x61, 0x72, 0x0,
				// value - options
				0x69, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Regex("foo", "bar", "i")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("dbPointer", func(t *testing.T) {
			buf := []byte{
				// type
				0xc,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - namespace length
				0x4, 0x0, 0x0, 0x0,
				// value - namespace
				0x62, 0x61, 0x72, 0x0,
				// value - oid
				0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.DBPointer(
				"foo",
				"bar",
				[12]byte{0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89},
			)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("JavaScriptCode", func(t *testing.T) {
			buf := []byte{
				// type
				0xd,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - code length
				0xd, 0x0, 0x0, 0x0,
				// value - code
				0x76, 0x61, 0x72, 0x20, 0x62, 0x61, 0x72, 0x20, 0x3d, 0x20, 0x33, 0x3b, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.JavaScript("foo", "var bar = 3;")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("symbol", func(t *testing.T) {
			buf := []byte{
				// type
				0xe,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - string length
				0x4, 0x0, 0x0, 0x0,
				// value - string
				0x62, 0x61, 0x72, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Symbol("foo", "bar")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("CodeWithScope", func(t *testing.T) {
			buf := []byte{
				0xf,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value - code length
				0x1a, 0x0, 0x0, 0x0,
				// value - length
				0xd, 0x0, 0x0, 0x0,
				// value - code
				0x76, 0x61, 0x72, 0x20, 0x62, 0x61, 0x72, 0x20, 0x3d, 0x20, 0x78, 0x3b, 0x0,
			}
			scope := NewDocument(EC.Null("x"))

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: scope}}
			actual := EC.CodeWithScope("foo", "var bar = x;", scope)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("int32", func(t *testing.T) {
			buf := []byte{
				// type
				0x10,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0xe5, 0xff, 0xff, 0xff,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Int32("foo", -27)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("timestamp", func(t *testing.T) {
			buf := []byte{
				// type
				0x11,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0x11, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Timestamp("foo", 8, 17)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("int64Type", func(t *testing.T) {
			buf := []byte{
				// type
				0x12,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0xe5, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Int64("foo", -27)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("Decimal128", func(t *testing.T) {
			buf := []byte{
				// type
				0x13,
				// key
				0x66, 0x6f, 0x6f, 0x0,
				// value
				0xee, 0x02, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3c, 0xb0,
			}
			d, _ := decimal.ParseDecimal128("-7.50")

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.Decimal128("foo", d)

			requireElementsEqual(t, expected, actual)
		})

		t.Run("minKey", func(t *testing.T) {
			buf := []byte{
				// type
				0xff,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.MinKey("foo")

			requireElementsEqual(t, expected, actual)
		})

		t.Run("maxKey", func(t *testing.T) {
			buf := []byte{
				// type
				0x7f,
				// key
				0x66, 0x6f, 0x6f, 0x0,
			}

			expected := &Element{&Value{start: 0, offset: 5, data: buf, d: nil}}
			actual := EC.MaxKey("foo")

			requireElementsEqual(t, expected, actual)
		})
	})

	t.Run("Array", func(t *testing.T) {
		t.Run("double", func(t *testing.T) {
			buf := []byte{
				// type
				0x1,
				// key
				0x0,
				// value
				0x6e, 0x86, 0x1b, 0xf0, 0xf9,
				0x21, 0x9, 0x40,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Double(3.14159)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("String", func(t *testing.T) {
			buf := []byte{
				// type
				0x2,
				// key
				0x0,
				// value - string length
				0x4, 0x0, 0x0, 0x0,
				// value - string
				0x62, 0x61, 0x72, 0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.String("bar")

			requireValuesEqual(t, expected, actual)
		})

		t.Run("SubDocument", func(t *testing.T) {
			buf := []byte{
				// type
				0x3,
				// key
				0x0,
			}
			d := NewDocument(EC.String("bar", "baz"))

			expected := &Value{start: 0, offset: 2, data: buf, d: d}
			actual := VC.Document(d)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("SubDocumentFromElements", func(t *testing.T) {
			buf := []byte{
				// type
				0x3,
				// key
				0x0,
			}
			e := EC.String("bar", "baz")
			d := NewDocument(e)

			expected := &Value{start: 0, offset: 2, data: buf, d: d}
			actual := VC.DocumentFromElements(e)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("SubDocumentFromReader", func(t *testing.T) {
			buf := []byte{
				// type
				0x3,
				// key
				0x0,
				0x10, 0x00, 0x00, 0x00,
				0x01, '0', 0x00,
				0x01, 0x02, 0x03, 0x04,
				0x05, 0x06, 0x07, 0x08,
				0x00,
			}
			rdr := Reader{
				0x10, 0x00, 0x00, 0x00,
				0x01, '0', 0x00,
				0x01, 0x02, 0x03, 0x04,
				0x05, 0x06, 0x07, 0x08,
				0x00,
			}

			expected := &Value{start: 0, offset: 2, data: buf}
			actual := VC.DocumentFromReader(rdr)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("Array", func(t *testing.T) {
			buf := []byte{
				// type
				0x4,
				// key
				0x0,
			}
			a := NewArray(VC.String("bar"), VC.Double(-2.7))

			expected := &Value{start: 0, offset: 2, data: buf, d: a.doc}
			actual := VC.Array(a)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("ArrayFromElements", func(t *testing.T) {
			buf := []byte{
				// type
				0x4,
				// key
				0x0,
			}
			e1 := VC.String("bar")
			e2 := VC.Double(-2.7)
			a := NewArray(e1, e2)

			expected := &Value{start: 0, offset: 2, data: buf, d: a.doc}
			actual := VC.ArrayFromValues(e1, e2)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("binary", func(t *testing.T) {
			buf := []byte{
				// type
				0x5,
				// key
				0x0,
				// value - binary length
				0x7, 0x0, 0x0, 0x0,
				// value - binary subtype
				0x0,
				// value - binary data
				0x8, 0x6, 0x7, 0x5, 0x3, 0x0, 0x9,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Binary([]byte{8, 6, 7, 5, 3, 0, 9})

			requireValuesEqual(t, expected, actual)
		})

		t.Run("BinaryWithSubtype", func(t *testing.T) {
			buf := []byte{
				// type
				0x5,
				// key
				0x0,
				// value - binary length
				0xb, 0x0, 0x0, 0x0,
				// value - binary subtype
				0x2,
				//
				0x07, 0x00, 0x00, 0x00,
				// value - binary data
				0x8, 0x6, 0x7, 0x5, 0x3, 0x0, 0x9,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.BinaryWithSubtype([]byte{8, 6, 7, 5, 3, 0, 9}, 2)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("undefined", func(t *testing.T) {
			buf := []byte{
				// type
				0x6,
				// key
				0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Undefined()

			requireValuesEqual(t, expected, actual)
		})

		t.Run("objectID", func(t *testing.T) {
			buf := []byte{
				// type
				0x7,
				// key
				0x0,
				// value
				0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.ObjectID(

				[12]byte{0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89},
			)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("Boolean", func(t *testing.T) {
			buf := []byte{
				// type
				0x8,
				// key
				0x0,
				// value
				0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Boolean(false)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("dateTime", func(t *testing.T) {
			buf := []byte{
				// type
				0x9,
				// key
				0x0,
				// value
				0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.DateTime(17)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("Null", func(t *testing.T) {
			buf := []byte{
				// type
				0xa,
				// key
				0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Null()

			requireValuesEqual(t, expected, actual)
		})

		t.Run("regex", func(t *testing.T) {
			buf := []byte{
				// type
				0xb,
				// key
				0x0,
				// value - pattern
				0x62, 0x61, 0x72, 0x0,
				// value - options
				0x69, 0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Regex("bar", "i")

			requireValuesEqual(t, expected, actual)
		})

		t.Run("dbPointer", func(t *testing.T) {
			buf := []byte{
				// type
				0xc,
				// key
				0x0,
				// value - namespace length
				0x4, 0x0, 0x0, 0x0,
				// value - namespace
				0x62, 0x61, 0x72, 0x0,
				// value - oid
				0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.DBPointer(

				"bar",
				[12]byte{0x5a, 0x15, 0xd0, 0xa4, 0xd5, 0xda, 0xa5, 0xf1, 0x0a, 0x5e, 0x10, 0x89},
			)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("JavaScriptCode", func(t *testing.T) {
			buf := []byte{
				// type
				0xd,
				// key
				0x0,
				// value - code length
				0xd, 0x0, 0x0, 0x0,
				// value - code
				0x76, 0x61, 0x72, 0x20, 0x62, 0x61, 0x72, 0x20, 0x3d, 0x20, 0x33, 0x3b, 0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.JavaScript("var bar = 3;")

			requireValuesEqual(t, expected, actual)
		})

		t.Run("symbol", func(t *testing.T) {
			buf := []byte{
				// type
				0xe,
				// key
				0x0,
				// value - string length
				0x4, 0x0, 0x0, 0x0,
				// value - string
				0x62, 0x61, 0x72, 0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Symbol("bar")

			requireValuesEqual(t, expected, actual)
		})

		t.Run("CodeWithScope", func(t *testing.T) {
			buf := []byte{
				0xf,
				// key
				0x0,
				// value - code length
				0x17, 0x0, 0x0, 0x0,
				// value - length
				0xd, 0x0, 0x0, 0x0,
				// value - code
				0x76, 0x61, 0x72, 0x20, 0x62, 0x61, 0x72, 0x20, 0x3d, 0x20, 0x78, 0x3b, 0x0,
			}
			scope := NewDocument(EC.Null("x"))

			expected := &Value{start: 0, offset: 2, data: buf, d: scope}
			actual := VC.CodeWithScope("var bar = x;", scope)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("int32", func(t *testing.T) {
			buf := []byte{
				// type
				0x10,
				// key
				0x0,
				// value
				0xe5, 0xff, 0xff, 0xff,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Int32(-27)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("timestamp", func(t *testing.T) {
			buf := []byte{
				// type
				0x11,
				// key
				0x0,
				// value
				0x11, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Timestamp(8, 17)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("int64Type", func(t *testing.T) {
			buf := []byte{
				// type
				0x12,
				// key
				0x0,
				// value
				0xe5, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Int64(-27)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("Decimal128", func(t *testing.T) {
			buf := []byte{
				// type
				0x13,
				// key
				0x0,
				// value
				0xee, 0x02, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3c, 0xb0,
			}
			d, _ := decimal.ParseDecimal128("-7.50")

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.Decimal128(d)

			requireValuesEqual(t, expected, actual)
		})

		t.Run("minKey", func(t *testing.T) {
			buf := []byte{
				// type
				0xff,
				// key
				0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.MinKey()

			requireValuesEqual(t, expected, actual)
		})

		t.Run("maxKey", func(t *testing.T) {
			buf := []byte{
				// type
				0x7f,
				// key
				0x0,
			}

			expected := &Value{start: 0, offset: 2, data: buf, d: nil}
			actual := VC.MaxKey()

			requireValuesEqual(t, expected, actual)
		})
	})
}

func TestDocumentConstructor(t *testing.T) {
	t.Run("Individual", func(t *testing.T) {
		for _, test := range []struct {
			Name        string
			Constructor func() (*Document, error)
			IsNil       bool
			Size        int
			Check       func(*testing.T, *Document)
		}{
			{
				Name: "EmptyMake",
				Size: 0,
				Constructor: func() (*Document, error) {
					return DC.Make(0), nil
				},
			},
			{
				Name: "EmptyMakeWithCapacity",
				Size: 0,
				Constructor: func() (*Document, error) {
					return DC.Make(100), nil
				},
			},
			{
				Name: "EmptyNew",
				Size: 0,
				Constructor: func() (*Document, error) {
					return DC.New(), nil
				},
			},
			{
				Name: "ReaderNil",
				Size: 0,
				Constructor: func() (*Document, error) {
					_, err := DC.ReaderErr(nil)
					if err == nil {
						return nil, errors.New("new reader")
					}
					return nil, nil
				},
				IsNil: true,
			},
			{
				Name: "ReaderEmpty",
				Size: 0,
				Constructor: func() (*Document, error) {
					_, err := DC.ReaderErr(Reader{})
					if err == nil {
						return nil, errors.New("new reader")
					}

					return nil, nil
				},
				IsNil: true,
			},
			{
				Name: "Reader",
				Size: 1,
				Constructor: func() (*Document, error) {
					doc := DC.Elements(EC.Int("foo", 42))
					bytes, err := doc.MarshalBSON()
					if err != nil {
						return nil, err
					}

					return DC.Reader(Reader(bytes)), nil
				},
				Check: func(t *testing.T, _ *Document) {
					assert.Panics(t, func() {
						DC.Reader(nil)
					})
				},
			},
			{
				Name: "MapString",
				Size: 2,
				Constructor: func() (*Document, error) {
					return DC.MapString(map[string]string{"a": "b", "b": "c"}), nil
				},
			},
			{
				Name: "MapInterface",
				Size: 2,
				Constructor: func() (*Document, error) {
					return DC.MapInterface(map[string]interface{}{"a": true, "b": "c"}), nil
				},
			},
			{
				Name: "MapInt",
				Size: 2,
				Constructor: func() (*Document, error) {
					return DC.MapInt(map[string]int{"a": math.MaxInt32 + 2, "b": math.MaxInt64}), nil
				},
			},
			{
				Name: "MapInt32",
				Size: 2,
				Constructor: func() (*Document, error) {
					return DC.MapInt32(map[string]int32{"a": 1, "b": 1000}), nil
				},
			},
			{
				Name: "MapInt64",
				Size: 2,
				Constructor: func() (*Document, error) {
					return DC.MapInt64(map[string]int64{"a": math.MaxInt64 - 4, "b": 1000}), nil
				},
			},
			{
				Name: "Marshaler",
				Size: 3,
				Constructor: func() (*Document, error) {
					return DC.Marshaler(DC.Elements(EC.Int("hi", 100), EC.String("there", "this"), EC.Time("now", time.Now()))), nil
				},
			},
			{
				Name: "MarshalerErr",
				Size: 3,
				Constructor: func() (*Document, error) {
					return DC.MarshalerErr(DC.Elements(EC.Int("hi", 100), EC.String("there", "this"), EC.Time("now", time.Now())))
				},
			},
			{
				Name:  "MarshalerErrNil",
				Size:  0,
				IsNil: true,
				Constructor: func() (*Document, error) {
					doc, err := DC.MarshalerErr(Reader(nil))
					if err == nil {
						return nil, errors.New("missed error")
					}

					return doc, nil
				},
			},
		} {
			t.Run(test.Name, func(t *testing.T) {
				doc, err := test.Constructor()
				require.NoError(t, err)
				if test.IsNil {
					require.Nil(t, doc)
					return
				}

				require.NotNil(t, doc)

				assert.Equal(t, test.Size, doc.Len())
				if test.Check != nil {
					t.Run("Check", func(t *testing.T) {
						test.Check(t, doc)
					})
				}
			})

		}

	})
	t.Run("Interface", func(t *testing.T) {
		for _, test := range []struct {
			Name      string
			Input     interface{}
			Size      int
			HasErrors bool
			Type      bsontype.Type
		}{
			{
				Name:  "Empty",
				Input: map[string]interface{}{},
				Size:  0,
				Type:  bsontype.EmbeddedDocument,
			},
			{
				Name:      "Nil",
				Input:     nil,
				Size:      0,
				Type:      bsontype.Null,
				HasErrors: true,
			},
			{
				Name:      "ReaderNil",
				Input:     Reader(nil),
				Size:      0,
				HasErrors: true,
				Type:      bsontype.Null,
			},
			{
				Name:  "MapString",
				Size:  1,
				Type:  bsontype.EmbeddedDocument,
				Input: map[string]string{"hi": "world"},
			},
			{
				Name:  "MapInterfaceInterfaceErrorKey",
				Size:  1,
				Input: map[interface{}]interface{}{errors.New("hi"): "world"},
				Type:  bsontype.EmbeddedDocument,
			},
			{
				Name:  "MapInterfaceInterfaceStrings",
				Size:  1,
				Input: map[interface{}]interface{}{"hi": "world"},
				Type:  bsontype.EmbeddedDocument,
			},
			{
				Name:  "MapInterfaceInterfaceBool",
				Size:  1,
				Input: map[interface{}]interface{}{true: "world"},
				Type:  bsontype.EmbeddedDocument,
			},
			{
				Name:  "MapInterfaceInterfaceStringer",
				Size:  1,
				Input: map[interface{}]interface{}{DC.New(): "world"},
				Type:  bsontype.EmbeddedDocument,
			},
			{
				Name:  "MapInterfaceStrings",
				Size:  2,
				Input: map[string]interface{}{"a": true, "b": "c"},
				Type:  bsontype.EmbeddedDocument,
			},
			{
				Name: "MapStringInterfaceSlice",
				Type: bsontype.EmbeddedDocument,
				Size: 2,
				Input: map[string][]interface{}{
					"a": []interface{}{"1", 2, "3"},
					"b": []interface{}{false, true, "1", 2, "3"},
				},
			},
			{
				Name:  "DocumentMarshaler",
				Type:  bsontype.EmbeddedDocument,
				Size:  3,
				Input: DC.Elements(EC.Int("hi", 100), EC.String("there", "this"), EC.Time("now", time.Now())),
			},
			{
				Name:  "Elements",
				Type:  bsontype.EmbeddedDocument,
				Size:  3,
				Input: []*Element{EC.Int("hi", 100), EC.String("there", "this"), EC.Time("now", time.Now())},
			},
			{
				Name:  "ElementSingle",
				Type:  bsontype.DateTime,
				Size:  1,
				Input: EC.Time("now", time.Now()),
			},
			{
				Name: "MapMarshaler",
				Size: 2,
				Type: bsontype.EmbeddedDocument,
				Input: map[string]Marshaler{
					"one": NewDocument(),
					"two": NewArray(),
				},
			},
			{
				Name: "MapMarshalerSlice",
				Size: 2,
				Type: bsontype.EmbeddedDocument,
				Input: map[string][]Marshaler{
					"one": []Marshaler{NewDocument(), NewDocument()},
					"two": []Marshaler{NewArray()},
				},
			},
			{
				Name:  "Marshaler",
				Size:  1,
				Type:  bsontype.EmbeddedDocument,
				Input: NewArray(VC.String("hi")),
			},
			{
				Name: "MapStringSlice",
				Size: 3,
				Type: bsontype.EmbeddedDocument,
				Input: map[string][]string{
					"hi":      []string{"hello", "world"},
					"results": []string{},
					"other":   []string{"one"},
				},
			},
			{
				Name: "MapStringInt64",
				Size: 5,
				Type: bsontype.EmbeddedDocument,
				Input: map[string]int64{
					"one":   400,
					"two":   math.MaxInt32,
					"three": math.MaxInt64 - 10,
					"four":  -100,
					"five":  -math.MaxInt64,
				},
			}, {
				Name: "MapStringInt",
				Size: 5,
				Type: bsontype.EmbeddedDocument,
				Input: map[string]int{
					"one":   400,
					"two":   math.MaxInt32,
					"three": math.MaxInt64 - 10,
					"four":  -100,
					"five":  -math.MaxInt64,
				},
			},
			{
				Name: "MapStringInt32",
				Size: 4,
				Type: bsontype.EmbeddedDocument,
				Input: map[string]int32{
					"one":  400,
					"two":  math.MaxInt32,
					"four": -100,
					"five": -math.MaxInt32,
				},
			},
		} {
			t.Run(test.Name, func(t *testing.T) {
				t.Run("NoErrors", func(t *testing.T) {
					doc := DC.Interface(test.Input)
					require.NotNil(t, doc)
					assert.Equal(t, test.Size, doc.Len())
				})
				t.Run("Errors", func(t *testing.T) {
					edoc, err := DC.InterfaceErr(test.Input)
					if test.HasErrors {
						assert.Error(t, err)
						assert.Nil(t, edoc)
					} else {
						assert.NoError(t, err)
						require.NotNil(t, edoc)
						assert.Equal(t, test.Size, edoc.Len())
					}
				})
				t.Run("ValueConstructor", func(t *testing.T) {
					val := VC.Interface(test.Input)
					require.NotNil(t, val)
					require.Equal(t, test.Type, val.Type())
				})
			})
		}
	})

}

// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/birch/bsonerr"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/evergreen-ci/birch/decimal"
	"github.com/evergreen-ci/birch/types"
	"github.com/stretchr/testify/assert"
)

func TestElement(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("nil Element", func(t *testing.T) {
			rdr := (*Element)(nil)
			want := bsonerr.NilElement
			_, got := rdr.Validate()
			if got != want {
				t.Errorf("Did not receive expected error. got %s; want %s", got, want)
			}
		})
		t.Run("validateKey error", func(t *testing.T) {
			rdr := Element{&Value{start: 0, offset: 1, data: []byte{0x0A, 'x'}}}
			want := bsonerr.InvalidKey
			_, got := rdr.Validate()
			if got != want {
				t.Errorf("Did not receive expected error. got %s; want %s", got, want)
			}
		})
		t.Run("Validate error", func(t *testing.T) {
			rdr := Element{&Value{start: 0, offset: 3, data: []byte{0x01, 'x', 0x00, 0x00}}}
			want := newErrTooSmall()
			_, got := rdr.Validate()
			if !IsTooSmall(got) {
				t.Errorf("Did not receive expected error. got %s; want %s", got, want)
			}
		})
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"string",
				&Element{&Value{
					start: 0, offset: 3,
					data: []byte{0x02, 'x', 0x00, 0x02, 0x00, 0x00, 0x00, 'y', 0x00},
				}},
				9, nil,
			},
			{"null", &Element{&Value{offset: 3, data: []byte{0x0A, 'x', 0x00}}}, 3, nil},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.Validate()
				if size != tc.size {
					t.Errorf("Incorrect size returned for validated element. got %d; want %d", size, tc.size)
				}
				if err != tc.err {
					t.Errorf("Incorrect error returned from Validate. got %s; want %s", err, tc.err)
				}
			})
		}
	})
	t.Run("validateKey", func(t *testing.T) {
		testCases := []struct {
			name  string
			elem  *Element
			total uint32
			err   error
		}{
			{
				"does not run off end of data", &Element{&Value{start: 0, offset: 100, data: []byte{0x0A, 'f', 'o', 'o'}}},
				3, bsonerr.InvalidKey,
			},

			{
				"stops iteration at start of value",
				&Element{&Value{start: 0, offset: 4, data: []byte{0x0A, 'f', 'o', 'o', 0x00}}},
				3, bsonerr.InvalidKey,
			},
			{
				"returns invalid key error", &Element{&Value{start: 0, offset: 4, data: []byte{0x0A, 'f', 'o', 'o'}}},
				3, bsonerr.InvalidKey,
			},
			{
				"returns correct size on success",
				&Element{&Value{start: 0, offset: 5, data: []byte{0x0A, 'f', 'o', 'o', 0x00}}},
				4, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				total, err := tc.elem.validateKey()
				if total != tc.total {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", total, tc.total)
				}
				if err != tc.err {
					t.Errorf("Did not receive correct error. got %v; want %v", err, tc.err)
				}
			})
		}
	})
	t.Run("valueSize", func(t *testing.T) {
		t.Run("returns too small", func(t *testing.T) {
			testCases := []struct {
				name string
				elem *Element
				size uint32
			}{
				{"subdoc <4", &Element{&Value{start: 0, offset: 2, data: []byte{0x03, 0x00, 0x00, 0x00}}}, 0},
				{"array <4", &Element{&Value{start: 0, offset: 2, data: []byte{0x04, 0x00, 0x00, 0x00}}}, 0},
				{"code-with-scope <4", &Element{&Value{start: 0, offset: 2, data: []byte{0x0F, 0x00, 0x00, 0x00}}}, 0},
				{"subdoc >4", &Element{&Value{start: 0, offset: 2, data: []byte{0x03, 0x00, 0xFF, 0x00, 0x00, 0x00}}}, 4},
				{"array >4", &Element{&Value{start: 0, offset: 2, data: []byte{0x04, 0x00, 0xFF, 0x00, 0x00, 0x00}}}, 4},
				{"code-with-scope >4", &Element{&Value{start: 0, offset: 2, data: []byte{0x0F, 0x00, 0xFF, 0x00, 0x00, 0x00}}}, 4},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					want := newErrTooSmall()
					size, got := tc.elem.value.valueSize()
					if size != tc.size {
						t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
					}
					if !IsTooSmall(got) {
						t.Errorf("Did not return correct error. got %v; want %v", got, want)
					}
				})
			}
		})
	})
	t.Run("Validate", testValidateValue)
	t.Run("convertValueToElem", testConvertValueToElem)
	t.Run("MarshalBSON", func(t *testing.T) {
		t.Run("Nil Value", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Nil Value", &Element{nil}, nil, bsonerr.UninitializedElement},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("Empty", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, nil, bsonerr.UninitializedElement,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("double", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x01, 0x00, 0x6E, 0x86, 0x1B, 0xF0, 0xF9, 0x21, 0x9, 0x40},
					}},
					[]byte{0x01, 0x00, 0x6E, 0x86, 0x1B, 0xF0, 0xF9, 0x21, 0x9, 0x40},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("String", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x02, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					}},
					[]byte{0x02, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("Embedded Document", func(t *testing.T) {
			subdoc := NewDocument(EC.String("bar", "baz"))

			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Document in data",
					&Element{
						&Value{
							start:  0,
							offset: 5,
							data: []byte{
								// type
								0x3,
								// key
								0x66, 0x6f, 0x6f, 0x0,

								// length
								0x12, 0x0, 0x0, 0x0,
								// type
								0x2,
								// key
								0x62, 0x61, 0x72, 0x0,
								// value - string length
								0x4, 0x0, 0x0, 0x0,
								// value - string
								0x62, 0x61, 0x7a, 0x0,

								// null terminator
								0x0,
							},
						},
					},
					[]byte{
						// type
						0x3,
						// key
						0x66, 0x6f, 0x6f, 0x0,

						// length
						0x12, 0x0, 0x0, 0x0,
						// type
						0x2,
						// key
						0x62, 0x61, 0x72, 0x0,
						// value - string length
						0x4, 0x0, 0x0, 0x0,
						// value - string
						0x62, 0x61, 0x7a, 0x0,

						// null terminator
						0x0,
					},
					nil,
				},
				{"Document in d",
					&Element{
						&Value{
							start:  0,
							offset: 5,
							data: []byte{
								// type
								0x3,
								// key
								0x66, 0x6f, 0x6f, 0x0,
							},
							d: subdoc,
						},
					},
					[]byte{
						// type
						0x3,
						// key
						0x66, 0x6f, 0x6f, 0x0,

						// length
						0x12, 0x0, 0x0, 0x0,
						// type
						0x2,
						// key
						0x62, 0x61, 0x72, 0x0,
						// value - string length
						0x4, 0x0, 0x0, 0x0,
						// value - string
						0x62, 0x61, 0x7a, 0x0,

						// null terminator
						0x0,
					},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("Array", func(t *testing.T) {
			// TODO: implement array test when array is implemented
		})

		t.Run("binary", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x05, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
					}},
					[]byte{0x05, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("objectID", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x07, 0x00,
							0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
							0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
						}}},
					[]byte{
						0x07, 0x00,
						0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
						0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
					},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("Boolean", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x08, 0x00, 0x01},
					}},
					[]byte{0x08, 0x00, 0x01},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("UTC Datetime", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x09, 0x00, 0x80, 0x38, 0x17, 0xB0, 0x60, 0x01, 0x00, 0x00},
					}},
					[]byte{0x09, 0x00, 0x80, 0x38, 0x17, 0xB0, 0x60, 0x01, 0x00, 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("regex", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x0B, 0x00, 'f', 'o', 'o', 0x00, 'b', 'a', 'r', 0x00},
					}},
					[]byte{0x0B, 0x00, 'f', 'o', 'o', 0x00, 'b', 'a', 'r', 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("dbPointer", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x0C, 0x00,
							0x04, 0x00, 0x00, 0x00,
							'f', 'o', 'o', 0x00,
							0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
							0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
						}}},
					[]byte{
						0x0C, 0x00,
						0x04, 0x00, 0x00, 0x00,
						'f', 'o', 'o', 0x00,
						0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
						0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
					},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("JavaScript", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x0D, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					}},
					[]byte{0x0D, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})
		t.Run("symbol", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x0E, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					}},
					[]byte{0x0E, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("CodeWithScope", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Scope in data",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x0F, 0x00,
							0x11, 0x00, 0x00, 0x00,
							0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
							0x05, 0x00, 0x00, 0x00, 0x00,
						}}},
					[]byte{
						0x0F, 0x00,
						0x11, 0x00, 0x00, 0x00,
						0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
						0x05, 0x00, 0x00, 0x00, 0x00,
					},
					nil,
				},
				{"Scope in d",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x0F, 0x00,
							0x11, 0x00, 0x00, 0x00,
							0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
						},
						d: NewDocument(),
					}},
					[]byte{
						0x0F, 0x00,
						0x11, 0x00, 0x00, 0x00,
						0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
						0x05, 0x00, 0x00, 0x00, 0x00,
					},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("int32", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x10, 0x00, 0xFF, 0x00, 0x00, 0x00},
					}},
					[]byte{0x10, 0x00, 0xFF, 0x00, 0x00, 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("timestamp", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x11, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
					}},
					[]byte{0x11, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("int64Type", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x12, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
					}},
					[]byte{0x12, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})

		t.Run("Decimal128", func(t *testing.T) {
			testCases := []struct {
				name          string
				elem          *Element
				expectedBytes []byte
				err           error
			}{
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x13, 0x00,
							0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
							0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
						}}},
					[]byte{
						0x13, 0x00,
						0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
						0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					},
					nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					actualBytes, err := tc.elem.MarshalBSON()
					if err != tc.err {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", err, tc.err)
					}

					if !bytes.Equal(actualBytes, tc.expectedBytes) {
						t.Errorf("Did not return correct value. got %#v; want %#v", actualBytes, tc.expectedBytes)
					}
				})
			}
		})
	})
	t.Run("Value Methods", func(t *testing.T) {
		t.Run("double", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   float64
				fault error
			}{
				{"Nil Value", &Element{nil}, 0, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, 0, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, 0, bsonerr.UninitializedElement,
				},
				{"Not double",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x02, 0x00}}}, 0,
					bsonerr.ElementType{"compact.Element.double", bsontype.Type(0x02)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x01, 0x00, 0x6E, 0x86, 0x1B, 0xF0, 0xF9, 0x21, 0x9, 0x40},
					}},
					3.14159, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Double()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %.5f; want %.5f", val, tc.val)
					}
				})
			}
		})
		t.Run("String", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   string
				fault error
			}{
				{"Nil Value", &Element{nil}, "", bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, "", bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, "", bsonerr.UninitializedElement,
				},
				{"Not String",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, "",
					bsonerr.ElementType{"compact.Element.String", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x02, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					}},
					"foo", nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.StringValue()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %s; want %s", val, tc.val)
					}
				})
			}
		})
		t.Run("Embedded Document", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   Reader
				fault error
			}{
				{"Nil Value", &Element{nil}, nil, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, nil, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, nil, bsonerr.UninitializedElement,
				},
				{"Not Document",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, nil,
					bsonerr.ElementType{"compact.Element.Document", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x03, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00},
					}},
					Reader{0x05, 0x00, 0x00, 0x00, 0x00}, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.ReaderDocument()
					if !bytes.Equal(val, tc.val) {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("Array", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   Reader
				fault error
			}{
				{"Nil Value", &Element{nil}, nil, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, nil, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, nil, bsonerr.UninitializedElement,
				},
				{"Not Array",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, nil,
					bsonerr.ElementType{"compact.Element.Array", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x04, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00},
					}},
					Reader{0x05, 0x00, 0x00, 0x00, 0x00}, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.ReaderArray()
					if !bytes.Equal(val, tc.val) {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("binary", func(t *testing.T) {
			testCases := []struct {
				name    string
				elem    *Element
				subtype byte
				val     []byte
				fault   error
			}{
				{"Nil Value", &Element{nil}, 0x00, nil, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, 0x00, nil, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, 0x00, nil, bsonerr.UninitializedElement,
				},
				{"Not binary",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, 0x00, nil,
					bsonerr.ElementType{"compact.Element.binary", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x05, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
					}},
					0x00, []byte{'f', 'o', 'o'}, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					subtype, val := tc.elem.value.Binary()
					if subtype != tc.subtype {
						t.Errorf("Did not return correct subtype. got %v; want %v", subtype, tc.subtype)
					}
					if !bytes.Equal(val, tc.val) {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("objectID", func(t *testing.T) {
			var empty [12]byte
			testCases := []struct {
				name  string
				elem  *Element
				val   types.ObjectID
				fault error
			}{
				{"Nil Value", &Element{nil}, empty, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Not objectID",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, empty,
					bsonerr.ElementType{"compact.Element.ObejctID", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x07, 0x00,
							0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
							0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
						}}},
					[12]byte{
						0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
						0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
					}, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.ObjectID()
					if !bytes.Equal(val[:], tc.val[:]) {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("Boolean", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   bool
				fault error
			}{
				{"Nil Value", &Element{nil}, false, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, false, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, false, bsonerr.UninitializedElement,
				},
				{"Not Boolean",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, false,
					bsonerr.ElementType{"compact.Element.Boolean", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x08, 0x00, 0x01},
					}},
					true, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Boolean()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("UTC dateTime as time.Time", func(t *testing.T) {
			var empty time.Time
			testCases := []struct {
				name  string
				elem  *Element
				val   time.Time
				fault error
			}{
				{"Nil Value", &Element{nil}, empty, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Not UTC dateTime",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, empty,
					bsonerr.ElementType{"compact.Element.dateTime", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x09, 0x00, 0x80, 0x38, 0x17, 0xB0, 0x60, 0x01, 0x00, 0x00},
					}},
					time.Unix(1514782800000/1000, 1514782800000%1000*1000000), nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Time()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("UTC dateTime", func(t *testing.T) {
			var empty int64
			testCases := []struct {
				name  string
				elem  *Element
				val   int64
				fault error
			}{
				{"Nil Value", &Element{nil}, empty, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Not UTC dateTime",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, empty,
					bsonerr.ElementType{"compact.Element.dateTime", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x09, 0x00, 0x80, 0x38, 0x17, 0xB0, 0x60, 0x01, 0x00, 0x00},
					}},
					1514782800000, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.DateTime()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %v; want %v", val, tc.val)
					}
				})
			}
		})
		t.Run("regex", func(t *testing.T) {
			testCases := []struct {
				name    string
				elem    *Element
				pattern string
				options string
				fault   error
			}{
				{"Nil Value", &Element{nil}, "", "", bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, "", "", bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, "", "", bsonerr.UninitializedElement,
				},
				{"Not regex",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, "", "",
					bsonerr.ElementType{"compact.Element.regex", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x0B, 0x00, 'f', 'o', 'o', 0x00, 'b', 'a', 'r', 0x00},
					}},
					"foo", "bar", nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					pattern, options := tc.elem.value.Regex()
					if pattern != tc.pattern {
						t.Errorf("Did not return correct pattern. got %v; want %v", pattern, tc.pattern)
					}
					if options != tc.options {
						t.Errorf("Did not return correct value. got %v; want %v", options, tc.options)
					}
				})
			}
		})
		t.Run("dbPointer", func(t *testing.T) {
			var empty [12]byte
			testCases := []struct {
				name    string
				elem    *Element
				ns      string
				pointer types.ObjectID
				fault   error
			}{
				{"Nil Value", &Element{nil}, "", empty, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, "", empty, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, "", empty, bsonerr.UninitializedElement,
				},
				{"Not dbPointer",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, "", empty,
					bsonerr.ElementType{"compact.Element.dbPointer", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x0C, 0x00,
							0x04, 0x00, 0x00, 0x00,
							'f', 'o', 'o', 0x00,
							0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
							0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
						}}},
					"foo", [12]byte{
						0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
						0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
					}, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					ns, pointer := tc.elem.value.DBPointer()
					if ns != tc.ns {
						t.Errorf("Did not return correct namespace. got %v; want %v", ns, tc.ns)
					}
					if !bytes.Equal(pointer[:], tc.pointer[:]) {
						t.Errorf("Did not return correct pointer. got %v; want %v", pointer, tc.pointer)
					}
				})
			}
		})
		t.Run("JavaScript", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   string
				fault error
			}{
				{"Nil Value", &Element{nil}, "", bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, "", bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, "", bsonerr.UninitializedElement,
				},
				{"Not JavaScript",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, "",
					bsonerr.ElementType{"compact.Element.JavaScript", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x0D, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					}},
					"foo", nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.JavaScript()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %s; want %s", val, tc.val)
					}
				})
			}
		})
		t.Run("symbol", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   string
				fault error
			}{
				{"Nil Value", &Element{nil}, "", bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, "", bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, "", bsonerr.UninitializedElement,
				},
				{"Not JavaScript",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, "",
					bsonerr.ElementType{"compact.Element.symbol", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x0E, 0x00, 0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
					}},
					"foo", nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Symbol()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %s; want %s", val, tc.val)
					}
				})
			}
		})
		t.Run("code With Scope", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				code  string
				scope Reader
				fault error
			}{
				{"Nil Value", &Element{nil}, "", nil, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, "", nil, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, "", nil, bsonerr.UninitializedElement,
				},
				{"Not JavascriptWithScope",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x01, 0x00}}}, "", nil,
					bsonerr.ElementType{"compact.Element.JavaScriptWithScope", bsontype.Type(0x01)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x0F, 0x00,
							0x11, 0x00, 0x00, 0x00,
							0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
							0x05, 0x00, 0x00, 0x00, 0x00,
						}}},
					"foo", Reader{0x05, 0x00, 0x00, 0x00, 0x00}, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					code, scope := tc.elem.value.ReaderJavaScriptWithScope()
					if code != tc.code {
						t.Errorf("Did not return correct code. got %s; want %s", code, tc.code)
					}
					if !bytes.Equal(scope, tc.scope) {
						t.Errorf("Did not return correct scope. got %v; want %v", scope, tc.scope)
					}
				})
			}
		})
		t.Run("int32", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   int32
				fault error
			}{
				{"Nil Value", &Element{nil}, 0, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, 0, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, 0, bsonerr.UninitializedElement,
				},
				{"Not int32",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x02, 0x00}}}, 0,
					bsonerr.ElementType{"compact.Element.int32", bsontype.Type(0x02)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x10, 0x00, 0xFF, 0x00, 0x00, 0x00},
					}},
					255, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Int32()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %d; want %d", val, tc.val)
					}
				})
			}
		})
		t.Run("timestamp", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				t     uint32
				i     uint32
				fault error
			}{
				{"Nil Value", &Element{nil}, 0, 0, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, 0, 0, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, 0, 0, bsonerr.UninitializedElement,
				},
				{"Not timestamp",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x02, 0x00}}}, 0, 0,
					bsonerr.ElementType{"compact.Element.timestamp", bsontype.Type(0x02)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x11, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x1, 0x00, 0x0},
					}},
					256, 255, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					ti, inc := tc.elem.value.Timestamp()
					if ti != tc.t || inc != tc.i {
						t.Errorf("Did not return correct value. got (%d, %d); want (%d, %d)",
							ti, inc, tc.t, tc.i)
					}
				})
			}
		})
		t.Run("int64Type", func(t *testing.T) {
			testCases := []struct {
				name  string
				elem  *Element
				val   int64
				fault error
			}{
				{"Nil Value", &Element{nil}, 0, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, 0, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, 0, bsonerr.UninitializedElement,
				},
				{"Not int64Type",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x02, 0x00}}}, 0,
					bsonerr.ElementType{"compact.Element.int64Type", bsontype.Type(0x02)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{0x12, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
					}},
					255, nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Int64()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %d; want %d", val, tc.val)
					}
				})
			}
		})
		t.Run("Decimal128", func(t *testing.T) {
			var empty decimal.Decimal128
			testCases := []struct {
				name  string
				elem  *Element
				val   decimal.Decimal128
				fault error
			}{
				{"Nil Value", &Element{nil}, empty, bsonerr.UninitializedElement},
				{"Empty Element value",
					&Element{&Value{start: 0, offset: 0, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Empty Element data",
					&Element{&Value{start: 0, offset: 2, data: nil}}, empty, bsonerr.UninitializedElement,
				},
				{"Not int64Type",
					&Element{&Value{start: 0, offset: 2, data: []byte{0x02, 0x00}}}, empty,
					bsonerr.ElementType{"compact.Element.Decimal128", bsontype.Type(0x02)},
				},
				{"Success",
					&Element{&Value{
						start: 0, offset: 2,
						data: []byte{
							0x13, 0x00,
							0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
							0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
						}}},
					decimal.NewDecimal128(255, 255), nil,
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					defer func() {
						fault := recover()
						if fault != tc.fault {
							t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
						}
					}()

					val := tc.elem.value.Decimal128()
					if val != tc.val {
						t.Errorf("Did not return correct value. got %#v; want %#v", val, tc.val)
					}
				})
			}
		})
	})
	t.Run("Key", func(t *testing.T) {
		testCases := []struct {
			name  string
			elem  *Element
			key   string
			fault error
		}{
			{"Nil Value", &Element{nil}, "", bsonerr.UninitializedElement},
			{"Empty Element value",
				&Element{&Value{start: 0, offset: 0, data: nil}}, "", bsonerr.UninitializedElement,
			},
			{"Empty Element data",
				&Element{&Value{start: 0, offset: 2, data: nil}}, "", bsonerr.UninitializedElement,
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 5,
					data: []byte{0x01, 'f', 'o', 'o', 0x00, 0x6E, 0x86, 0x1B, 0xF0, 0xF9, 0x21, 0x9, 0x40},
				}},
				"foo", nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				defer func() {
					fault := recover()
					if fault != tc.fault {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
					}
				}()

				key := tc.elem.Key()
				if key != tc.key {
					t.Errorf("Did not return correct key. got %s; want %s", key, tc.key)
				}
			})
		}
	})
	t.Run("Type", func(t *testing.T) {
		testCases := []struct {
			name  string
			elem  *Element
			etype bsontype.Type
			fault error
		}{
			{"Nil Value", &Element{nil}, 0x0, bsonerr.UninitializedElement},
			{"Empty Element value",
				&Element{&Value{start: 0, offset: 0, data: nil}}, 0x00, bsonerr.UninitializedElement,
			},
			{"Empty Element data",
				&Element{&Value{start: 0, offset: 2, data: nil}}, 0x00, bsonerr.UninitializedElement,
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 5,
					data: []byte{0x01, 'f', 'o', 'o', 0x00, 0x6E, 0x86, 0x1B, 0xF0, 0xF9, 0x21, 0x9, 0x40},
				}},
				0x01, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				defer func() {
					fault := recover()
					if fault != tc.fault {
						t.Errorf("Did not return the correct error for panic. got %v; want %v", fault, tc.fault)
					}
				}()

				etype := tc.elem.value.Type()
				if etype != tc.etype {
					t.Errorf("Did not return correct type. got %v; want %v", etype, tc.etype)
				}
			})
		}
	})
	t.Run("SetValue", func(t *testing.T) {
		elem := EC.Int("foo", 300)
		elem.SetValue(VC.Int32(42))
		assert.Equal(t, 42, elem.Value().Int())
	})

}

func testConvertValueToElem(t *testing.T) {
	testCases := []struct {
		name string
		key  string
		val  *Value
		elem *Element
	}{
		{"nil", "", nil, nil},
		{"double", "foo", VC.Double(3.14159), EC.Double("foo", 3.14159)},
		{"int64", "foo", VC.Int64(1234567890), EC.Int64("foo", 1234567890)},
		{"interface-array", "test", VC.ArrayFromValues(VC.String("test01")), EC.ArrayFromElements("test", VC.String("test01"))},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := convertValueToElem(tc.key, tc.val)
			want := tc.elem

			if !got.Equal(want) {
				t.Errorf("Expected elements to be equal but they are not. got %v; want %v", got, want)
			}
		})
	}
}

func testValidateValue(t *testing.T) {
	t.Run("double", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x01, 0x00, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x01, 0x00, 0x6E, 0x86, 0x1B, 0xF0, 0xF9, 0x21, 0x9, 0x40},
				}},
				8, nil,
			},
		}

		for _, tc := range testCases {
			size, err := tc.elem.value.validate(false)
			if size != tc.size {
				t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
			}
			requireErrEqual(t, tc.err, err)
		}
	})
	t.Run("String", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			deep bool
			size uint32
			err  error
		}{
			{"Too Small <4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x02, 0x00, 0x00, 0x00},
				}},
				true, 0, newErrTooSmall(),
			},
			{"Too Small >4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x02, 0x00, 0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
				}},
				true, 4, newErrTooSmall(),
			},
			{"Invalid String Value",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				false, 4, bsonerr.InvalidString,
			},
			{"Shouldn't Deep Validate",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				true, 7, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(tc.deep)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("Embedded Document/Array", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			deep bool
			size uint32
			err  error
		}{
			{"Document/too small <4",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x03, 0x00, 0x00, 0x00},
				}}, true, 0, newErrTooSmall(),
			},
			{"Document/too small >4",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x03, 0x00, 0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
				}}, true, 4, newErrTooSmall(),
			},
			{"Document/invalid document <5",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x03, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}}, true, 4, bsonerr.InvalidReadOnlyDocument,
			},
			{"Document/shouldn't deep validate",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x03, 0x00, 0x09, 0x00, 0x00, 0x00, 'f', 'o', 'o', 'o', 'o'},
				}}, true, 9, nil,
			},
			{"Document/should deep validate",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x03, 0x00, 0x09, 0x00, 0x00, 0x00, 'f', 'o', 'o', 'o', 'o'},
				}}, false, 9, bsonerr.InvalidKey,
			},
			{"Document/success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x03, 0x00, 0x0A, 0x00, 0x00, 0x00, 0x0A, 'f', 'o', 'o', 0x00, 0x00},
				}}, false, 10, nil,
			},
			{"Array/too small <4",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00, 0x00, 0x00},
				}}, true, 0, newErrTooSmall(),
			},
			{"Array/too small >4",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00, 0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
				}}, true, 4, newErrTooSmall(),
			},
			{"Array/invalid document <5",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}}, true, 4, bsonerr.InvalidReadOnlyDocument,
			},
			{"Array/shouldn't deep validate",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00, 0x09, 0x00, 0x00, 0x00, 'f', 'o', 'o', 'o', 'o'},
				}}, true, 9, nil,
			},
			{"Array/should deep validate",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00, 0x09, 0x00, 0x00, 0x00, 'f', 'o', 'o', 'o', 'o'},
				}}, false, 9, bsonerr.InvalidKey,
			},
			{"Array/success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00, 0x08, 0x00, 0x00, 0x00, 0x0A, '0', 0x00, 0x00},
				}}, false, 8, nil,
			},
			{"ArrayWithDocument/success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x04, 0x00},
					d: NewArray(VC.Null()).doc,
				}}, false, 8, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(tc.deep)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}

	})
	t.Run("binary", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Value Too Small",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x05, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Invalid binary Subtype",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7F},
				}},
				5, bsonerr.InvalidBinarySubtype,
			},
			{"Length Too Small",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x05, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00},
				}},
				5, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x05, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 'h', 'i'},
				}},
				7, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("undefined", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x06, 0x00},
				}},
				0, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				if err != tc.err {
					t.Errorf("Did not return correct error. got %v; want %v", err, tc.err)
				}
			})
		}
	})
	t.Run("objectID", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Value Too Small",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x07, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{
						0x07, 0x00,
						0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
						0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					}}},
				12, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("Boolean", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x08, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Invalid binary Type",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x08, 0x00, 0x03},
				}},
				1, bsonerr.InvalidBooleanType,
			},
			{"True",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x08, 0x00, 0x01},
				}},
				1, nil,
			},
			{"False",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x08, 0x00, 0x00},
				}},
				1, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("UTC dateTime", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x09, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{
						0x09, 0x00,
						0x01, 0x02, 0x03, 0x04,
						0x05, 0x06, 0x07, 0x08,
					}}},
				8, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("Null", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x0A, 0x00},
				}},
				0, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				if err != tc.err {
					t.Errorf("Did not return correct error. got %v; want %v", err, tc.err)
				}
			})
		}
	})
	t.Run("regex", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"First Invalid String",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x0B, 0x00, 'f', 'o', 'o'},
				}},
				3, bsonerr.InvalidString,
			},
			{"Second Invalid String",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x0B, 0x00, 'f', 'o', 'o', 0x00, 'b', 'a', 'r'},
				}},
				7, bsonerr.InvalidString,
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x0B, 0x00, 'f', 'o', 'o', 0x00, 'b', 'a', 'r', 0x00},
				}},
				8, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				if err != tc.err {
					t.Errorf("Did not return correct error. got %v; want %v", err, tc.err)
				}
			})
		}
	})
	t.Run("dbPointer", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x0C, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Length Too Large",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x0C, 0x00, 0xFF, 0x00, 0x00, 0x00, 0x00},
				}},
				4, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{
						0x0C, 0x00,
						0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
						0x01, 0x02, 0x03, 0x04,
						0x05, 0x06, 0x07, 0x08,
						0x09, 0x0A, 0x0B, 0x0C,
					}}},
				20, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("JavaScript", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			deep bool
			size uint32
			err  error
		}{
			{"Too Small <4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0D, 0x00, 0x00, 0x00},
				}},
				true, 0, newErrTooSmall(),
			},
			{"Too Small >4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0D, 0x00, 0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
				}},
				true, 4, newErrTooSmall(),
			},
			{"Invalid String Value",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0D, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				false, 4, bsonerr.InvalidString,
			},
			{"Shouldn't Deep Validate",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0D, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				true, 7, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(tc.deep)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("symbol", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			deep bool
			size uint32
			err  error
		}{
			{"Too Small <4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0E, 0x00, 0x00, 0x00},
				}},
				true, 0, newErrTooSmall(),
			},
			{"Too Small >4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0E, 0x00, 0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
				}},
				true, 4, newErrTooSmall(),
			},
			{"Invalid String Value",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0E, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				false, 4, bsonerr.InvalidString,
			},
			{"Shouldn't Deep Validate",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0E, 0x00, 0x03, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				true, 7, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(tc.deep)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("code With Scope", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			deep bool
			size uint32
			err  error
		}{
			{"Too Small <4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0F, 0x00, 0x00, 0x00},
				}},
				true, 0, newErrTooSmall(),
			},
			{"Too Small >4",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0F, 0x00, 0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00},
				}},
				true, 4, newErrTooSmall(),
			},
			{"Shouldn't Deep Validate",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x0F, 0x00, 0x07, 0x00, 0x00, 0x00, 'f', 'o', 'o'},
				}},
				true, 7, nil,
			},
			{"Deep Validate String Too Large",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{
						0x0F, 0x00,
						0x0C, 0x00, 0x00, 0x00,
						0xFF, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
					}}},
				false, 8, bsonerr.StringLargerThanContainer,
			},
			{"Deep Validate Invalid String",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{
						0x0F, 0x00,
						0x10, 0x00, 0x00, 0x00,
						0x02, 0x00, 0x00, 0x00, 'f', 'o', 'o',
						0xFF, 0x01, 0x02, 0x03, 0x04,
					}}},
				false, 8, bsonerr.InvalidString,
			},
			{"Deep Validate Invalid Document",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{
						0x0F, 0x00,
						0x11, 0x00, 0x00, 0x00,
						0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
						0xFF, 0x00, 0x00, 0x00, 0x00,
					}}},
				false, 12, bsonerr.InvalidLength,
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{
						0x0F, 0x00,
						0x11, 0x00, 0x00, 0x00,
						0x04, 0x00, 0x00, 0x00, 'f', 'o', 'o', 0x00,
						0x05, 0x00, 0x00, 0x00, 0x00,
					}}},
				false, 17, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(tc.deep)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("int32", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x10, 0x00, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x10, 0x00, 0x01, 0x02, 0x03, 0x04},
				}},
				4, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("timestamp", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x11, 0x00, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x11, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				}},
				8, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("int64Type", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x12, 0x00, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x12, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				}},
				8, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("Decimal128", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Too Small",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{0x13, 0x00, 0x00, 0x00},
				}},
				0, newErrTooSmall(),
			},
			{"Success",
				&Element{&Value{
					start: 0, offset: 2,
					data: []byte{
						0x13, 0x00,
						0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
						0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x0F,
					}}},
				16, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				requireErrEqual(t, tc.err, err)
			})
		}
	})
	t.Run("minKey", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0xFF, 0x00},
				}},
				0, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				if err != tc.err {
					t.Errorf("Did not return correct error. got %v; want %v", err, tc.err)
				}
			})
		}
	})
	t.Run("maxKey", func(t *testing.T) {
		testCases := []struct {
			name string
			elem *Element
			size uint32
			err  error
		}{
			{"Success",
				&Element{&Value{
					start: 0, offset: 2, data: []byte{0x7F, 0x00},
				}},
				0, nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				size, err := tc.elem.value.validate(false)
				if size != tc.size {
					t.Errorf("Did not return correct number of bytes read. got %d; want %d", size, tc.size)
				}
				if err != tc.err {
					t.Errorf("Did not return correct error. got %v; want %v", err, tc.err)
				}
			})
		}
	})
	t.Run("Invalid Element", func(t *testing.T) {
		want := bsonerr.InvalidElement
		var wantSize uint32
		gotSize, got := (&Value{start: 0, offset: 2, data: []byte{0xEE, 0x00}}).validate(false)
		if gotSize != wantSize {
			t.Errorf("Did not return correct number of bytes read. got %d; want %d", gotSize, wantSize)
		}
		if got != want {
			t.Errorf("Did not return correct error. got %v; want %v", got, want)
		}
	})
	t.Run("fmt.Stringer", func(t *testing.T) {
		var rdr Reader
		var err error
		rdr, err = NewDocument(EC.String("foo", "bar"),
			EC.SubDocumentFromElements("fooer",
				EC.SubDocumentFromElements("barer", EC.Int32("ok", 1)),
			),
		).MarshalBSON()
		if err != nil {
			t.Errorf("Unexpected error while marshaling document: %v", err)
		}
		testCases := []struct {
			name string
			doc  interface{}
			want string
		}{
			{
				name: "nested document",
				doc: NewDocument(
					EC.String("foo", "bar"),
					EC.SubDocumentFromElements("fooer",
						EC.SubDocumentFromElements("barer", EC.Int32("ok", 1)),
					),
				),
				want: `bson.Document{bson.Element{[string]"foo": "bar"}, bson.Element{[embedded document]"fooer": map[barer:map[ok:1]]}}`,
			},
			{
				name: "nested reader",
				doc:  rdr,
				want: `bson.Reader{bson.Element{[string]"foo": "bar"}, bson.Element{[embedded document]"fooer": map[barer:map[ok:1]]}}`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got := fmt.Sprintf("%s", tc.doc)
				if got != tc.want {
					t.Errorf("Output from fmt.Stringer implementation does not match.\ngot :%s\nwant:%s", got, tc.want)
				}
			})
		}
	})
}

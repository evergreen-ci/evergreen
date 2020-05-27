// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/evergreen-ci/birch/bsonerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArray(t *testing.T) {
	t.Run("Append", func(t *testing.T) {
		t.Run("Nil Insert", func(t *testing.T) {
			func() {
				defer func() {
					r := recover()
					if r != bsonerr.NilElement {
						t.Errorf("Did not received expected error from panic. got %#v; want %#v", r, bsonerr.NilElement)
					}
				}()
				a := NewArray()
				a.Append(nil)
			}()
		})
		t.Run("Ignore Nil Insert", func(t *testing.T) {
			func() {
				defer func() {
					r := recover()
					if r != nil {
						t.Errorf("Received unexpected panic from nil insert. got %#v; want %#v", r, nil)
					}
				}()
				want := NewArray()
				want.doc.IgnoreNilInsert = true

				got := NewArray()
				got.doc.IgnoreNilInsert = true
				got.Append(nil)

				require.Equal(t, want, got)
				// if diff := cmp.Diff(got, want, cmp.AllowUnexported(Document{}, Array{})); diff != "" {
				// 	t.Errorf("Documents differ: (-got +want)\n%s", diff)
				// }
			}()
		})
		testCases := []struct {
			name   string
			values [][]*Value
			want   []byte
		}{
			{"one-one", tapag.oneOne(), tapag.oneOneBytes(0)},
			{"two-one", tapag.twoOne(), tapag.twoOneAppendBytes(0)},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				a := NewArray()
				for _, elems := range tc.values {
					a.Append(elems...)
				}

				got, err := a.MarshalBSON()
				if err != nil {
					t.Errorf("Received an unexpected error while marshaling BSON: %s", err)
				}
				if !bytes.Equal(got, tc.want) {
					t.Errorf("Output from Append is not correct. got %#v; want %#v", got, tc.want)
				}
			})
		}
	})
	t.Run("Prepend", func(t *testing.T) {
		t.Run("Nil Insert", func(t *testing.T) {
			func() {
				defer func() {
					r := recover()
					if r != bsonerr.NilElement {
						t.Errorf("Did not received expected error from panic. got %#v; want %#v", r, bsonerr.NilElement)
					}
				}()
				a := NewArray()
				a.Prepend(nil)
			}()
		})
		t.Run("Ignore Nil Insert", func(t *testing.T) {
			testCases := []struct {
				name   string
				values []*Value
				want   *Array
			}{
				{
					"first element nil",
					nil,
					&Array{
						&Document{
							IgnoreNilInsert: true,
							elems:           make([]*Element, 0), index: make([]uint32, 0),
						},
					},
				},
			}

			for _, tc := range testCases {
				var got *Array
				func() {
					defer func() {
						r := recover()
						if r != nil {
							t.Errorf("Did not received expected error from panic. got %#v; want %#v", r, nil)
						}

						require.Equal(t, tc.want, got)
						// if diff := cmp.Diff(got, tc.want, cmp.AllowUnexported(Document{}, Array{})); diff != "" {
						// 	t.Errorf("Documents differ: (-got +want)\n%s", diff)
						// }
					}()
					got = NewArray()
					got.doc.IgnoreNilInsert = true
					got.Prepend(tc.values...)
				}()
			}
		})
		testCases := []struct {
			name   string
			values [][]*Value
			want   []byte
		}{
			{"one-one", tapag.oneOne(), tapag.oneOneBytes(0)},
			{"two-one", tapag.twoOne(), tapag.twoOnePrependBytes(0)},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				a := NewArray()
				for _, elems := range tc.values {
					a.Prepend(elems...)
				}
				got, err := a.MarshalBSON()
				if err != nil {
					t.Errorf("Received an unexpected error while marshaling BSON: %s", err)
				}
				if !bytes.Equal(got, tc.want) {
					t.Errorf("Output from Prepend is not correct. got %#v; want %#v", got, tc.want)
				}
			})
		}
	})
	t.Run("Lookup", func(t *testing.T) {
		testCases := []struct {
			name string
			a    *Array
			key  uint
			want *Value
			err  error
		}{
			{
				"first",
				NewArray(VC.Null()),
				0,
				&Value{start: 0, offset: 2, data: []byte{0xa, 0x0}},
				nil,
			},
			{
				"not-found",
				NewArray(VC.Null()),
				1,
				nil,
				bsonerr.OutOfBounds,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := tc.a.LookupErr(tc.key)
				if err != tc.err {
					t.Errorf("Returned error does not match. got %#v; want %#v", err, tc.err)
				}
				if !valueEqual(got, tc.want) {
					t.Errorf("Returned element does not match expected element. got %#v; want %#v", got, tc.want)
				}
			})
		}
	})
	t.Run("Delete", func(t *testing.T) {
		t.Run("empty key", func(t *testing.T) {
			d := NewDocument()
			var want *Element
			got := d.Delete()
			if got != want {
				t.Errorf("Delete should return nil element when deleting with empty key. got %#v, want %#v", got, want)
			}
		})
		testCases := []struct {
			name string
			a    *Array
			key  uint
			want *Value
		}{
			{
				"first",
				NewArray(VC.Null()),
				0,
				&Value{start: 0, offset: 2},
			},
			{
				"not-found",
				NewArray(VC.Null()),
				1,
				nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got := tc.a.Delete(tc.key)
				if !valueEqual(got, tc.want) {
					t.Errorf("Returned element does not match expected element. got %#v; want %#v", got, tc.want)
				}
			})
		}
	})
	t.Run("Iterator", func(t *testing.T) {
		iteratorTests := []struct {
			name   string
			values [][]*Value
		}{
			{"one-one", tapag.oneOne()},
			{"two-one", tapag.twoOne()},
		}

		for _, tc := range iteratorTests {
			t.Run(tc.name, func(t *testing.T) {
				a := NewArray()
				for _, elems := range tc.values {
					a.Prepend(elems...)
				}

				iter := a.Iterator()

				for _, elem := range tc.values {
					if !iter.Next() {
						t.Errorf("ArrayIterator.Next() returned false")
					}

					if err := iter.Err(); err != nil {
						t.Errorf("ArrayIterator.Err() returned non-nil error: %s", err)
					}

					for _, val := range elem {
						got := iter.Value()
						if !valueEqual(got, val) {
							t.Errorf("Returned element does not match expected element. got %#v; want %#v", got, val)
						}
					}
				}

				if iter.Next() {
					t.Errorf("ArrayIterator.Next() returned true. expected false")
				}

				if err := iter.Err(); err != nil {
					t.Errorf("ArrayIterator.Err() returned non-nil error: %s", err)
				}
			})
		}
	})
	t.Run("Constructors", func(t *testing.T) {
		t.Run("FromDocument", func(t *testing.T) {
			doc := NewDocument(EC.Int("foo", 42), EC.Int("bar", 84))
			require.Equal(t, 2, doc.Len())

			ar := ArrayFromDocument(doc)
			assert.Equal(t, 2, ar.Len())
			iter := ar.Iterator()
			require.NotNil(t, iter)
			total := 0
			for iter.Next() {
				total += iter.Value().Int()
			}
			assert.Equal(t, 126, total)
		})
		t.Run("Make", func(t *testing.T) {
			ar := MakeArray(42)
			assert.Equal(t, 0, ar.Len())
			assert.Equal(t, 42, cap(ar.doc.elems))
		})
	})
	t.Run("Reset", func(t *testing.T) {
		ar := NewArray(VC.Int(42))
		assert.Equal(t, 1, ar.Len())
		ar.Reset()
		assert.Equal(t, 0, ar.Len())
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("Passing", func(t *testing.T) {
			ar := NewArray(VC.Int(42), VC.Int(84))
			ln, err := ar.Validate()
			require.NoError(t, err)
			require.True(t, ln > 0)
		})
		t.Run("Fail", func(t *testing.T) {
			ar := NewArray(&Value{})
			ln, err := ar.Validate()
			require.Error(t, err)
			require.Zero(t, ln)
		})
		t.Run("Marshal", func(t *testing.T) {
			_, err := NewArray(&Value{}).MarshalBSON()
			assert.Error(t, err)
		})

	})
	t.Run("Lookup", func(t *testing.T) {
		t.Run("FindValue", func(t *testing.T) {
			ar := NewArray(VC.Int(42), VC.Int(84))

			assert.Equal(t, 42, ar.Lookup(0).Int())
			assert.Equal(t, 84, ar.Lookup(1).Int())
		})
		t.Run("MissingValue", func(t *testing.T) {
			ar := NewArray(VC.Int(42), VC.Int(84))

			assert.Panics(t, func() { ar.Lookup(3).Int() })
		})
		t.Run("FindElement", func(t *testing.T) {
			ar := NewArray(VC.Int(42), VC.Int(84))

			assert.Equal(t, 42, ar.LookupElement(0).Value().Int())
			assert.Equal(t, 84, ar.LookupElement(1).Value().Int())
		})
		t.Run("MissingElement", func(t *testing.T) {
			ar := NewArray(VC.Int(42), VC.Int(84))

			assert.Panics(t, func() { ar.LookupElement(3).Value().Int() })
		})
		t.Run("ElementKeys", func(t *testing.T) {
			ar := NewArray(VC.Int(42), VC.Int(84))
			assert.Equal(t, "", ar.LookupElement(0).Key())
			assert.Equal(t, "", ar.LookupElement(1).Key())
		})
	})
	t.Run("InterfaceExport", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			ar := NewArray()
			assert.Len(t, ar.Interface(), 0)
		})
		t.Run("Value", func(t *testing.T) {
			slice := NewArray(VC.Int(42), VC.Int(84)).Interface()

			assert.Len(t, slice, 2)
			assert.EqualValues(t, slice[0], 42)
			assert.EqualValues(t, slice[1], 84)
		})
	})
	t.Run("String", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			ar := NewArray()
			assert.Equal(t, "bson.Array[]", ar.String())
		})
		t.Run("Content", func(t *testing.T) {
			ar := NewArray(VC.String("hello"), VC.String("world"))
			assert.Equal(t, "bson.Array[hello, world]", ar.String())
		})
	})
	t.Run("Set", func(t *testing.T) {
		t.Run("OutOfBounds", func(t *testing.T) {
			ar := NewArray()
			assert.Panics(t, func() { ar.Set(10, VC.String("hi")) })
		})
		t.Run("Empty", func(t *testing.T) {
			ar := NewArray()
			assert.Panics(t, func() { ar.Set(0, VC.String("hi")) })
		})
		t.Run("Replace", func(t *testing.T) {
			ar := NewArray(VC.Int(42))
			assert.EqualValues(t, 42, ar.Lookup(0).Interface())
			ar.Set(0, VC.Int(84))
			assert.EqualValues(t, 84, ar.Lookup(0).Interface())
		})

	})

}

type testArrayPrependAppendGenerator struct{}

var tapag testArrayPrependAppendGenerator

func (testArrayPrependAppendGenerator) oneOne() [][]*Value {
	return [][]*Value{
		{VC.Double(3.14159)},
	}
}

func (testArrayPrependAppendGenerator) oneOneBytes(index uint) []byte {
	a := []byte{
		// size
		0x0, 0x0, 0x0, 0x0,
		// type
		0x1,
	}

	// key
	a = append(a, []byte(strconv.FormatUint(uint64(index), 10))...)
	a = append(a, 0)

	a = append(a,
		// value
		0x6e, 0x86, 0x1b, 0xf0, 0xf9, 0x21, 0x9, 0x40,
		// null terminator
		0x0,
	)

	a[0] = byte(len(a))

	return a
}

func (testArrayPrependAppendGenerator) twoOne() [][]*Value {
	return [][]*Value{
		{VC.Double(1.234)},
		{VC.Double(5.678)},
	}
}

func (testArrayPrependAppendGenerator) twoOneAppendBytes(index uint) []byte {
	a := []byte{
		// size
		0x0, 0x0, 0x0, 0x0,
		// type
		0x1,
	}

	// key
	a = append(a, []byte(strconv.FormatUint(uint64(index), 10))...)
	a = append(a, 0)

	a = append(a,
		// value
		0x58, 0x39, 0xb4, 0xc8, 0x76, 0xbe, 0xf3, 0x3f,
		// type
		0x1,
	)

	// key
	a = append(a, []byte(strconv.FormatUint(uint64(index+1), 10))...)
	a = append(a, 0)

	a = append(a,
		// value
		0x83, 0xc0, 0xca, 0xa1, 0x45, 0xb6, 0x16, 0x40,
		// null terminator
		0x0,
	)

	a[0] = byte(len(a))

	return a
}

func (testArrayPrependAppendGenerator) twoOnePrependBytes(index uint) []byte {
	a := []byte{
		// size
		0x0, 0x0, 0x0, 0x0,
		// type
		0x1,
	}

	// key
	a = append(a, []byte(strconv.FormatUint(uint64(index), 10))...)
	a = append(a, 0)

	a = append(a,
		// value
		0x83, 0xc0, 0xca, 0xa1, 0x45, 0xb6, 0x16, 0x40,
		// type
		0x1,
	)

	// key
	a = append(a, []byte(strconv.FormatUint(uint64(index+1), 10))...)
	a = append(a, 0)

	a = append(a,
		// value
		0x58, 0x39, 0xb4, 0xc8, 0x76, 0xbe, 0xf3, 0x3f,
		// null terminator
		0x0,
	)

	a[0] = byte(len(a))

	return a
}

func ExampleArray() {
	internalVersion := "1234567"

	f := func(appName string) *Array {
		arr := NewArray()
		arr.Append(
			VC.DocumentFromElements(
				EC.String("name", "mongo-go-driver"),
				EC.String("version", internalVersion),
			),
			VC.DocumentFromElements(
				EC.String("type", "darwin"),
				EC.String("architecture", "amd64"),
			),
			VC.String("go1.9.2"),
		)
		if appName != "" {
			arr.Append(VC.DocumentFromElements(EC.String("name", appName)))
		}

		return arr
	}
	buf, err := f("hello-world").MarshalBSON()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(buf)

	// Output: [154 0 0 0 3 48 0 52 0 0 0 2 110 97 109 101 0 16 0 0 0 109 111 110 103 111 45 103 111 45 100 114 105 118 101 114 0 2 118 101 114 115 105 111 110 0 8 0 0 0 49 50 51 52 53 54 55 0 0 3 49 0 46 0 0 0 2 116 121 112 101 0 7 0 0 0 100 97 114 119 105 110 0 2 97 114 99 104 105 116 101 99 116 117 114 101 0 6 0 0 0 97 109 100 54 52 0 0 2 50 0 8 0 0 0 103 111 49 46 57 46 50 0 3 51 0 27 0 0 0 2 110 97 109 101 0 12 0 0 0 104 101 108 108 111 45 119 111 114 108 100 0 0 0]
}

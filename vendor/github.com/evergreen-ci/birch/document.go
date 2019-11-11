// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/evergreen-ci/birch/bsonerr"
	"github.com/evergreen-ci/birch/elements"
)

// Document is a mutable ordered map that compactly represents a BSON document.
type Document struct {
	// The default behavior or Append, Prepend, and Replace is to panic on the
	// insertion of a nil element. Setting IgnoreNilInsert to true will instead
	// silently ignore any nil paramet()ers to these methods.
	IgnoreNilInsert bool
	elems           []*Element
	index           []uint32
}

// NewDocument creates an empty Document. The numberOfElems parameter will
// preallocate the underlying storage which can prevent extra allocations.
func NewDocument(elems ...*Element) *Document { return DC.Elements(elems...) }

// ReadDocument will create a Document using the provided slice of bytes. If the
// slice of bytes is not a valid BSON document, this method will return an error.
func ReadDocument(b []byte) (*Document, error) {
	var doc = new(Document)
	err := doc.UnmarshalBSON(b)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// Copy makes a shallow copy of this document.
func (d *Document) Copy() *Document {
	if d == nil {
		return nil
	}

	doc := &Document{
		IgnoreNilInsert: d.IgnoreNilInsert,
		elems:           make([]*Element, len(d.elems), cap(d.elems)),
		index:           make([]uint32, len(d.index), cap(d.index)),
	}

	copy(doc.elems, d.elems)
	copy(doc.index, d.index)

	return doc
}

// Len returns the number of elements in the document.
func (d *Document) Len() int {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	return len(d.elems)
}

// Keys returns all of the element keys for this document. If recursive is true,
// this method will also return the keys of any subdocuments or arrays.
func (d *Document) Keys(recursive bool) (Keys, error) {
	if d == nil {
		return nil, bsonerr.NilDocument
	}

	return d.recursiveKeys(recursive)
}

// recursiveKeys handles the recursion case for retrieving all of a Document's
// keys.
func (d *Document) recursiveKeys(recursive bool, prefix ...string) (Keys, error) {
	if d == nil {
		return nil, bsonerr.NilDocument
	}

	ks := make(Keys, 0, len(d.elems))
	for _, elem := range d.elems {
		key := elem.Key()
		ks = append(ks, Key{Prefix: prefix, Name: key})
		if !recursive {
			continue
		}
		// TODO(skriptble): Should we support getting the keys of a code with
		// scope document?
		switch elem.value.Type() {
		case '\x03':
			subprefix := append(prefix, key)
			subkeys, err := elem.value.MutableDocument().recursiveKeys(recursive, subprefix...)
			if err != nil {
				return nil, err
			}
			ks = append(ks, subkeys...)
		case '\x04':
			subprefix := append(prefix, key)
			subkeys, err := elem.value.MutableArray().doc.recursiveKeys(recursive, subprefix...)
			if err != nil {
				return nil, err
			}
			ks = append(ks, subkeys...)
		}
	}
	return ks, nil
}

// Append adds each element to the end of the document, in order. If a nil element is passed
// as a parameter this method will panic. To change this behavior to silently
// ignore a nil element, set IgnoreNilInsert to true on the Document.
//
// If a nil element is inserted and this method panics, it does not remove the
// previously added elements.
func (d *Document) Append(elems ...*Element) *Document {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	for _, elem := range elems {
		if elem == nil {
			if d.IgnoreNilInsert {
				continue
			}
			// TODO(skriptble): Maybe Append and Prepend should return an error
			// instead of panicking here.
			panic(bsonerr.NilElement)
		}
		d.elems = append(d.elems, elem)
		i := sort.Search(len(d.index), func(i int) bool {
			return bytes.Compare(
				d.keyFromIndex(i), elem.value.data[elem.value.start+1:elem.value.offset]) >= 0
		})
		if i < len(d.index) {
			d.index = append(d.index, 0)
			copy(d.index[i+1:], d.index[i:])
			d.index[i] = uint32(len(d.elems) - 1)
		} else {
			d.index = append(d.index, uint32(len(d.elems)-1))
		}
	}
	return d
}

// Prepend adds each element to the beginning of the document, in order. If a nil element is passed
// as a parameter this method will panic. To change this behavior to silently
// ignore a nil element, set IgnoreNilInsert to true on the Document.
//
// If a nil element is inserted and this method panics, it does not remove the
// previously added elements.
func (d *Document) Prepend(elems ...*Element) *Document {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	// In order to insert the prepended elements in order we need to make space
	// at the front of the elements slice.
	d.elems = append(d.elems, elems...)
	copy(d.elems[len(elems):], d.elems)

	remaining := len(elems)
	for idx, elem := range elems {
		if elem == nil {
			if d.IgnoreNilInsert {
				// Having nil elements in a document would be problematic.
				copy(d.elems[idx:], d.elems[idx+1:])
				d.elems[len(d.elems)-1] = nil
				d.elems = d.elems[:len(d.elems)-1]
				continue
			}
			// Not very efficient, but we're about to blow up so ¯\_(ツ)_/¯
			for j := idx; j < remaining; j++ {
				copy(d.elems[j:], d.elems[j+1:])
				d.elems[len(d.elems)-1] = nil
				d.elems = d.elems[:len(d.elems)-1]
			}
			panic(bsonerr.NilElement)
		}
		remaining--
		d.elems[idx] = elem
		for idx := range d.index {
			d.index[idx]++
		}
		i := sort.Search(len(d.index), func(i int) bool {
			return bytes.Compare(
				d.keyFromIndex(i), elem.value.data[elem.value.start+1:elem.value.offset]) >= 0
		})
		if i < len(d.index) {
			d.index = append(d.index, 0)
			copy(d.index[i+1:], d.index[i:])
			d.index[i] = 0
		} else {
			d.index = append(d.index, 0)
		}
	}
	return d
}

// Set replaces an element of a document. If an element with a matching key is
// found, the element will be replaced with the one provided. If the document
// does not have an element with that key, the element is appended to the
// document instead. If a nil element is passed as a parameter this method will
// panic. To change this behavior to silently ignore a nil element, set
// IgnoreNilInsert to true on the Document.
//
// If a nil element is inserted and this method panics, it does not remove the
// previously added elements.
func (d *Document) Set(elem *Element) *Document {
	if elem == nil {
		if d.IgnoreNilInsert {
			return d
		}
		panic(bsonerr.NilElement)
	}

	key := elem.Key() + "\x00"
	i := sort.Search(len(d.index), func(i int) bool { return bytes.Compare(d.keyFromIndex(i), []byte(key)) >= 0 })
	if i < len(d.index) && bytes.Equal(d.keyFromIndex(i), []byte(key)) {
		d.elems[d.index[i]] = elem
		return d
	}

	d.elems = append(d.elems, elem)
	position := uint32(len(d.elems) - 1)
	if i < len(d.index) {
		d.index = append(d.index, 0)
		copy(d.index[i+1:], d.index[i:])
		d.index[i] = position
	} else {
		d.index = append(d.index, position)
	}

	return d
}

// RecursiveLookup searches the document and potentially subdocuments or arrays for the
// provided key. Each key provided to this method represents a layer of depth.
//
// RecursiveLookup will return nil if it encounters an error.
func (d *Document) RecursiveLookup(key ...string) *Value {
	elem, err := d.RecursiveLookupElementErr(key...)
	if err != nil {
		return nil
	}

	return elem.value
}

// RecursiveLookupErr searches the document and potentially subdocuments or arrays for the
// provided key. Each key provided to this method represents a layer of depth.
func (d *Document) RecursiveLookupErr(key ...string) (*Value, error) {
	elem, err := d.RecursiveLookupElementErr(key...)
	if err != nil {
		return nil, err
	}

	return elem.value, nil
}

// RecursiveLookupElement searches the document and potentially subdocuments or arrays for the
// provided key. Each key provided to this method represents a layer of depth.
//
// RecursiveLookupElement will return nil if it encounters an error.
func (d *Document) RecursiveLookupElement(key ...string) *Element {
	elem, err := d.RecursiveLookupElementErr(key...)
	if err != nil {
		return nil
	}

	return elem
}

// RecursiveLookupElementErr searches the document and potentially subdocuments or arrays for the
// provided key. Each key provided to this method represents a layer of depth.
func (d *Document) RecursiveLookupElementErr(key ...string) (*Element, error) {
	if d == nil {
		return nil, bsonerr.NilDocument
	}

	if len(key) == 0 {
		return nil, bsonerr.EmptyKey
	}
	var elem *Element
	var err error
	first := []byte(key[0] + "\x00")
	i := sort.Search(len(d.index), func(i int) bool { return bytes.Compare(d.keyFromIndex(i), first) >= 0 })
	if i < len(d.index) && bytes.Equal(d.keyFromIndex(i), first) {
		elem = d.elems[d.index[i]]
		if len(key) == 1 {
			return elem, nil
		}
		switch elem.value.Type() {
		case '\x03':
			elem, err = elem.value.MutableDocument().RecursiveLookupElementErr(key[1:]...)
		case '\x04':
			index, err := strconv.ParseUint(key[1], 10, 0)
			if err != nil {
				return nil, bsonerr.InvalidArrayKey
			}

			val, err := elem.value.MutableArray().lookupTraverse(uint(index), key[2:]...)
			if err != nil {
				return nil, err
			}

			elem = &Element{value: val}

		default:
			// TODO(skriptble): This error message should be more clear, e.g.
			// include information about what depth was reached, what the
			// incorrect type was, etc...
			err = bsonerr.InvalidDepthTraversal
		}
	}
	if err != nil {
		return nil, err
	}
	if elem == nil {
		// TODO(skriptble): This should also be a clearer error message.
		// Preferably we should track the depth at which the key was not found.
		return nil, bsonerr.ElementNotFound
	}
	return elem, nil
}

// Delete removes the keys from the Document. The deleted element is
// returned. If the key does not exist, then nil is returned and the delete is
// a no-op. The same is true if something along the depth tree does not exist
// or is not a traversable type.
func (d *Document) Delete(key ...string) *Element {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	if len(key) == 0 {
		return nil
	}
	// Do a binary search through the index, delete the element from
	// the index and delete the element from the elems array.
	var elem *Element
	first := []byte(key[0] + "\x00")
	i := sort.Search(len(d.index), func(i int) bool { return bytes.Compare(d.keyFromIndex(i), first) >= 0 })
	if i < len(d.index) && bytes.Equal(d.keyFromIndex(i), first) {
		keyIndex := d.index[i]
		elem = d.elems[keyIndex]
		if len(key) == 1 {
			d.index = append(d.index[:i], d.index[i+1:]...)
			d.elems = append(d.elems[:keyIndex], d.elems[keyIndex+1:]...)
			for j := range d.index {
				if d.index[j] > keyIndex {
					d.index[j]--
				}
			}
			return elem
		}
		switch elem.value.Type() {
		case '\x03':
			elem = elem.value.MutableDocument().Delete(key[1:]...)
		case '\x04':
			elem = elem.value.MutableArray().doc.Delete(key[1:]...)
		default:
			elem = nil
		}
	}
	return elem
}

// ElementAt retrieves the element at the given index in a Document. It panics if the index is
// out-of-bounds.
//
// TODO(skriptble): This method could be variadic and return the element at the
// provided depth.
func (d *Document) ElementAt(index uint) *Element {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	return d.elems[index]
}

// ElementAtOK is the same as ElementAt, but returns a boolean instead of panicking.
func (d *Document) ElementAtOK(index uint) (*Element, bool) {
	if d == nil {
		return nil, false
	}

	if index >= uint(len(d.elems)) {
		return nil, false
	}

	return d.ElementAt(index), true
}

// Iterator creates an Iterator for this document and returns it.
func (d *Document) Iterator() Iterator {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	return newIterator(d)
}

func (d *Document) Extend(d2 *Document) *Document   { d.Append(d2.elems...); return d }
func (d *Document) ExtendReader(r Reader) *Document { d.Append(DC.Reader(r).elems...); return d }

// Reset clears a document so it can be reused. This method clears references
// to the underlying pointers to elements so they can be garbage collected.
func (d *Document) Reset() {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	for idx := range d.elems {
		d.elems[idx] = nil
	}
	d.elems = d.elems[:0]
	d.index = d.index[:0]
}

// Validate validates the document and returns its total size.
func (d *Document) Validate() (uint32, error) {
	if d == nil {
		return 0, bsonerr.NilDocument
	}

	// Header and Footer
	var size uint32 = 4 + 1
	for _, elem := range d.elems {
		n, err := elem.Validate()
		if err != nil {
			return 0, err
		}
		size += n
	}
	return size, nil
}

// WriteTo implements the io.WriterTo interface.
//
// TODO(skriptble): We can optimize this by having creating implementations of
// writeByteSlice that write directly to an io.Writer instead.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	if d == nil {
		return 0, bsonerr.NilDocument
	}

	b, err := d.MarshalBSON()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(b)
	return int64(n), err
}

// WriteDocument will serialize this document to the provided writer beginning
// at the provided start position.
func (d *Document) WriteDocument(start uint, writer interface{}) (int64, error) {
	if d == nil {
		return 0, bsonerr.NilDocument
	}

	var total int64
	var pos = start
	size, err := d.Validate()
	if err != nil {
		return total, err
	}
	switch w := writer.(type) {
	case []byte:
		n, err := d.writeByteSlice(pos, size, w)
		total += n
		if err != nil {
			return total, err
		}
	default:
		return 0, bsonerr.InvalidWriter
	}
	return total, nil
}

// writeByteSlice handles serializing this document to a slice of bytes starting
// at the given start position.
func (d *Document) writeByteSlice(start uint, size uint32, b []byte) (int64, error) {
	if d == nil {
		return 0, bsonerr.NilDocument
	}

	var total int64
	var pos = start
	if len(b) < int(start)+int(size) {
		return 0, newErrTooSmall()
	}
	n, err := elements.Int32.Encode(start, b, int32(size))
	total += int64(n)
	pos += uint(n)
	if err != nil {
		return total, err
	}
	for _, elem := range d.elems {
		n, err := elem.writeElement(true, pos, b)
		total += n
		pos += uint(n)
		if err != nil {
			return total, err
		}
	}

	n, err = elements.Byte.Encode(pos, b, '\x00')
	total += int64(n)
	if err != nil {
		return total, err
	}
	return total, nil
}

// MarshalBSON implements the Marshaler interface.
func (d *Document) MarshalBSON() ([]byte, error) {
	if d == nil {
		return nil, bsonerr.NilDocument
	}

	size, err := d.Validate()

	if err != nil {
		return nil, err
	}
	b := make([]byte, size)
	_, err = d.writeByteSlice(0, size, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UnmarshalBSON implements the Unmarshaler interface.
func (d *Document) UnmarshalBSON(b []byte) error {
	if d == nil {
		return bsonerr.NilDocument
	}

	// Read byte array
	//   - Create an Element for each element found
	//   - Update the index with the key of the element
	//   TODO: Maybe do 2 pass and alloc the elems and index once?
	// 		   We should benchmark 2 pass vs multiple allocs for growing the slice
	_, err := Reader(b).readElements(func(elem *Element) error {
		d.elems = append(d.elems, elem)
		i := sort.Search(len(d.index), func(i int) bool {
			return bytes.Compare(
				d.keyFromIndex(i), elem.value.data[elem.value.start+1:elem.value.offset]) >= 0
		})
		if i < len(d.index) {
			d.index = append(d.index, 0)
			copy(d.index[i+1:], d.index[i:])
			d.index[i] = uint32(len(d.elems) - 1)
		} else {
			d.index = append(d.index, uint32(len(d.elems)-1))
		}
		return nil
	})
	return err
}

// ReadFrom will read one BSON document from the given io.Reader.
func (d *Document) ReadFrom(r io.Reader) (int64, error) {
	if d == nil {
		return 0, bsonerr.NilDocument
	}

	var total int64
	sizeBuf := make([]byte, 4)
	n, err := io.ReadFull(r, sizeBuf)
	total += int64(n)
	if err != nil {
		return total, err
	}
	givenLength := readi32(sizeBuf)
	b := make([]byte, givenLength)
	copy(b[0:4], sizeBuf)
	n, err = io.ReadFull(r, b[4:])
	total += int64(n)
	if err != nil {
		return total, err
	}
	err = d.UnmarshalBSON(b)
	return total, err
}

// keyFromIndex returns the key for the element. The idx parameter is the
// position in the index property, not the elems property. This method is
// mainly used when calling sort.Search.
func (d *Document) keyFromIndex(idx int) []byte {
	if d == nil {
		panic(bsonerr.NilDocument)
	}

	haystack := d.elems[d.index[idx]]
	return haystack.value.data[haystack.value.start+1 : haystack.value.offset]
}

// String implements the fmt.Stringer interface.
func (d *Document) String() string {
	if d == nil {
		return "<nil>"
	}

	var buf bytes.Buffer
	buf.Write([]byte("bson.Document{"))
	for idx, elem := range d.elems {
		if idx > 0 {
			buf.Write([]byte(", "))
		}
		fmt.Fprintf(&buf, "%s", elem)
	}
	buf.WriteByte('}')

	return buf.String()
}

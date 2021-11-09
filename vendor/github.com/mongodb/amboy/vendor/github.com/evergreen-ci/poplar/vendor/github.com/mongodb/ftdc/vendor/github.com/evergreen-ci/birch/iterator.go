// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package birch

import "github.com/evergreen-ci/birch/bsonerr"

// Iterator describes the types used to iterate over a bson Document.
type Iterator interface {
	Next() bool
	Element() *Element
	Value() *Value
	Err() error
}

// ElementIterator facilitates iterating over a bson.Document.
type elementIterator struct {
	d     *Document
	index int
	elem  *Element
	err   error
}

func newIterator(d *Document) *elementIterator {
	return &elementIterator{d: d}
}

// Next fetches the next element of the document, returning whether or not the next element was able
// to be fetched. If true is returned, then call Element to get the element. If false is returned,
// call Err to check if an error occurred.
func (itr *elementIterator) Next() bool {
	if itr.index >= len(itr.d.elems) {
		return false
	}

	e := itr.d.elems[itr.index]

	_, err := e.Validate()
	if err != nil {
		itr.err = err
		return false
	}

	itr.elem = e
	itr.index++

	return true
}

// Element returns the current element of the Iterator. The pointer that it returns will
// _always_ be the same for a given Iterator.
func (itr *elementIterator) Element() *Element { return itr.elem }
func (itr *elementIterator) Value() *Value     { return itr.elem.value }
func (itr *elementIterator) Err() error        { return itr.err }

// readerIterator facilitates iterating over a bson.Reader.
type readerIterator struct {
	r    Reader
	pos  uint32
	end  uint32
	elem *Element
	err  error
}

// newReaderIterator constructors a new readerIterator over a given Reader.
func newReaderIterator(r Reader) (*readerIterator, error) {
	itr := new(readerIterator)
	if len(r) < 5 {
		return nil, newErrTooSmall()
	}
	givenLength := readi32(r[0:4])
	if len(r) < int(givenLength) {
		return nil, bsonerr.InvalidLength
	}

	itr.r = r
	itr.pos = 4
	itr.end = uint32(givenLength)
	itr.elem = &Element{value: &Value{}}

	return itr, nil
}

// Next fetches the next element of the Reader, returning whether or not the next element was able
// to be fetched. If true is returned, then call Element to get the element. If false is returned,
// call Err to check if an error occurred.
func (itr *readerIterator) Next() bool {
	if itr.pos >= itr.end {
		itr.err = bsonerr.InvalidReadOnlyDocument
		return false
	}
	if itr.r[itr.pos] == '\x00' {
		return false
	}
	elemStart := itr.pos
	itr.pos++
	n, err := itr.r.validateKey(itr.pos, itr.end)
	itr.pos += n
	if err != nil {
		itr.err = err
		return false
	}

	itr.elem.value.start = elemStart
	itr.elem.value.offset = itr.pos
	itr.elem.value.data = itr.r
	itr.elem.value.d = nil

	n, err = itr.elem.value.validate(false)
	itr.pos += n
	if err != nil {
		itr.err = err
		return false
	}
	return true
}

// Element returns the current element of the readerIterator. The pointer that it returns will
// _always_ be the same for a given readerIterator.
func (itr *readerIterator) Element() *Element { return itr.elem }
func (itr *readerIterator) Value() *Value     { return itr.elem.value }
func (itr *readerIterator) Err() error        { return itr.err }

// arrayIterator facilitates iterating over a bson.Array.
type arrayIterator struct {
	array *Array
	pos   uint
	elem  *Element
	err   error
}

func newArrayIterator(a *Array) *arrayIterator {
	return &arrayIterator{array: a}
}

// Next fetches the next value in the Array, returning whether or not it could be fetched successfully. If true is
// returned, call Value to get the value. If false is returned, call Err to check if an error occurred.
func (iter *arrayIterator) Next() bool {
	elem, err := iter.array.LookupElementErr(iter.pos)

	if err != nil {
		// error if out of bounds
		// don't assign iter.err
		return false
	}

	_, err = elem.value.validate(false)
	if err != nil {
		iter.err = err
		return false
	}

	iter.elem = elem
	iter.pos++

	return true
}

// Value returns the current value of the arrayIterator. The pointer returned will _always_ be the same for a given
// arrayIterator. The returned value will be nil if this function is called before the first successful call to Next().
func (iter *arrayIterator) Value() *Value     { return iter.elem.value }
func (iter *arrayIterator) Element() *Element { return iter.elem }
func (iter *arrayIterator) Err() error        { return iter.err }

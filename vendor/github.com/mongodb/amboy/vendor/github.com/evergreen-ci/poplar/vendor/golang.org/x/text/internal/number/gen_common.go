// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"unicode/utf8"

	"golang.org/x/text/internal/language/compact"
)

// A system identifies a CLDR numbering system.
type system byte

type systemData struct {
	id        system
	digitSize byte              // number of UTF-8 bytes per digit
	zero      [utf8.UTFMax]byte // UTF-8 sequence of zero digit.
}

// A SymbolType identifies a symbol of a specific kind.
type SymbolType int

const (
	SymDecimal SymbolType = iota
	SymGroup
	SymList
	SymPercentSign
	SymPlusSign
	SymMinusSign
	SymExponential
	SymSuperscriptingExponent
	SymPerMille
	SymInfinity
	SymNan
	SymTimeSeparator

	NumSymbolTypes
)

const hasNonLatnMask = 0x8000

// symOffset is an offset into altSymData if the bit indicated by hasNonLatnMask
// is not 0 (with this bit masked out), and an offset into symIndex otherwise.
//
// TODO: this type can be a byte again if we use an indirection into altsymData
// and introduce an alt -> offset slice (the length of this will be number of
// alternatives plus 1). This also allows getting rid of the compactTag field
// in altSymData. In total this will save about 1K.
type symOffset uint16

type altSymData struct {
	compactTag compact.ID
	symIndex   symOffset
	system     system
}

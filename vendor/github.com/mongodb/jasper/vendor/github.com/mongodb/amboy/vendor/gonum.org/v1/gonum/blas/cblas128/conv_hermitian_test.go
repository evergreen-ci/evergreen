// Code generated by "go generate gonum.org/v1/gonum/blas”; DO NOT EDIT.

// Copyright ©2015 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cblas128

import (
	math "math/cmplx"
	"testing"

	"gonum.org/v1/gonum/blas"
)

func newHermitianFrom(a HermitianCols) Hermitian {
	t := Hermitian{
		N:      a.N,
		Stride: a.N,
		Data:   make([]complex128, a.N*a.N),
		Uplo:   a.Uplo,
	}
	t.From(a)
	return t
}

func (m Hermitian) n() int { return m.N }
func (m Hermitian) at(i, j int) complex128 {
	if m.Uplo == blas.Lower && i < j && j < m.N {
		i, j = j, i
	}
	if m.Uplo == blas.Upper && i > j {
		i, j = j, i
	}
	return m.Data[i*m.Stride+j]
}
func (m Hermitian) uplo() blas.Uplo { return m.Uplo }

func newHermitianColsFrom(a Hermitian) HermitianCols {
	t := HermitianCols{
		N:      a.N,
		Stride: a.N,
		Data:   make([]complex128, a.N*a.N),
		Uplo:   a.Uplo,
	}
	t.From(a)
	return t
}

func (m HermitianCols) n() int { return m.N }
func (m HermitianCols) at(i, j int) complex128 {
	if m.Uplo == blas.Lower && i < j {
		i, j = j, i
	}
	if m.Uplo == blas.Upper && i > j && i < m.N {
		i, j = j, i
	}
	return m.Data[i+j*m.Stride]
}
func (m HermitianCols) uplo() blas.Uplo { return m.Uplo }

type hermitian interface {
	n() int
	at(i, j int) complex128
	uplo() blas.Uplo
}

func sameHermitian(a, b hermitian) bool {
	an := a.n()
	bn := b.n()
	if an != bn {
		return false
	}
	if a.uplo() != b.uplo() {
		return false
	}
	for i := 0; i < an; i++ {
		for j := 0; j < an; j++ {
			if a.at(i, j) != b.at(i, j) || math.IsNaN(a.at(i, j)) != math.IsNaN(b.at(i, j)) {
				return false
			}
		}
	}
	return true
}

var hermitianTests = []Hermitian{
	{N: 3, Stride: 3, Data: []complex128{
		1, 2, 3,
		4, 5, 6,
		7, 8, 9,
	}},
	{N: 3, Stride: 5, Data: []complex128{
		1, 2, 3, 0, 0,
		4, 5, 6, 0, 0,
		7, 8, 9, 0, 0,
	}},
}

func TestConvertHermitian(t *testing.T) {
	for _, test := range hermitianTests {
		for _, uplo := range []blas.Uplo{blas.Upper, blas.Lower} {
			test.Uplo = uplo
			colmajor := newHermitianColsFrom(test)
			if !sameHermitian(colmajor, test) {
				t.Errorf("unexpected result for row major to col major conversion:\n\tgot: %#v\n\tfrom:%#v",
					colmajor, test)
			}
			rowmajor := newHermitianFrom(colmajor)
			if !sameHermitian(rowmajor, test) {
				t.Errorf("unexpected result for col major to row major conversion:\n\tgot: %#v\n\twant:%#v",
					rowmajor, test)
			}
		}
	}
}
func newHermitianBandFrom(a HermitianBandCols) HermitianBand {
	t := HermitianBand{
		N:      a.N,
		K:      a.K,
		Stride: a.K + 1,
		Data:   make([]complex128, a.N*(a.K+1)),
		Uplo:   a.Uplo,
	}
	for i := range t.Data {
		t.Data[i] = math.NaN()
	}
	t.From(a)
	return t
}

func (m HermitianBand) n() (n int) { return m.N }
func (m HermitianBand) at(i, j int) complex128 {
	b := Band{
		Rows: m.N, Cols: m.N,
		Stride: m.Stride,
		Data:   m.Data,
	}
	switch m.Uplo {
	default:
		panic("cblas128: bad BLAS uplo")
	case blas.Upper:
		b.KU = m.K
		if i > j {
			i, j = j, i
		}
	case blas.Lower:
		b.KL = m.K
		if i < j {
			i, j = j, i
		}
	}
	return b.at(i, j)
}
func (m HermitianBand) bandwidth() (k int) { return m.K }
func (m HermitianBand) uplo() blas.Uplo    { return m.Uplo }

func newHermitianBandColsFrom(a HermitianBand) HermitianBandCols {
	t := HermitianBandCols{
		N:      a.N,
		K:      a.K,
		Stride: a.K + 1,
		Data:   make([]complex128, a.N*(a.K+1)),
		Uplo:   a.Uplo,
	}
	for i := range t.Data {
		t.Data[i] = math.NaN()
	}
	t.From(a)
	return t
}

func (m HermitianBandCols) n() (n int) { return m.N }
func (m HermitianBandCols) at(i, j int) complex128 {
	b := BandCols{
		Rows: m.N, Cols: m.N,
		Stride: m.Stride,
		Data:   m.Data,
	}
	switch m.Uplo {
	default:
		panic("cblas128: bad BLAS uplo")
	case blas.Upper:
		b.KU = m.K
		if i > j {
			i, j = j, i
		}
	case blas.Lower:
		b.KL = m.K
		if i < j {
			i, j = j, i
		}
	}
	return b.at(i, j)
}
func (m HermitianBandCols) bandwidth() (k int) { return m.K }
func (m HermitianBandCols) uplo() blas.Uplo    { return m.Uplo }

type hermitianBand interface {
	n() (n int)
	at(i, j int) complex128
	bandwidth() (k int)
	uplo() blas.Uplo
}

func sameHermitianBand(a, b hermitianBand) bool {
	an := a.n()
	bn := b.n()
	if an != bn {
		return false
	}
	if a.uplo() != b.uplo() {
		return false
	}
	ak := a.bandwidth()
	bk := b.bandwidth()
	if ak != bk {
		return false
	}
	for i := 0; i < an; i++ {
		for j := 0; j < an; j++ {
			if a.at(i, j) != b.at(i, j) || math.IsNaN(a.at(i, j)) != math.IsNaN(b.at(i, j)) {
				return false
			}
		}
	}
	return true
}

var hermitianBandTests = []HermitianBand{
	{N: 3, K: 0, Stride: 1, Uplo: blas.Upper, Data: []complex128{
		1,
		2,
		3,
	}},
	{N: 3, K: 0, Stride: 1, Uplo: blas.Lower, Data: []complex128{
		1,
		2,
		3,
	}},
	{N: 3, K: 1, Stride: 2, Uplo: blas.Upper, Data: []complex128{
		1, 2,
		3, 4,
		5, -1,
	}},
	{N: 3, K: 1, Stride: 2, Uplo: blas.Lower, Data: []complex128{
		-1, 1,
		2, 3,
		4, 5,
	}},
	{N: 3, K: 2, Stride: 3, Uplo: blas.Upper, Data: []complex128{
		1, 2, 3,
		4, 5, -1,
		6, -2, -3,
	}},
	{N: 3, K: 2, Stride: 3, Uplo: blas.Lower, Data: []complex128{
		-2, -1, 1,
		-3, 2, 4,
		3, 5, 6,
	}},

	{N: 3, K: 0, Stride: 5, Uplo: blas.Upper, Data: []complex128{
		1, 0, 0, 0, 0,
		2, 0, 0, 0, 0,
		3, 0, 0, 0, 0,
	}},
	{N: 3, K: 0, Stride: 5, Uplo: blas.Lower, Data: []complex128{
		1, 0, 0, 0, 0,
		2, 0, 0, 0, 0,
		3, 0, 0, 0, 0,
	}},
	{N: 3, K: 1, Stride: 5, Uplo: blas.Upper, Data: []complex128{
		1, 2, 0, 0, 0,
		3, 4, 0, 0, 0,
		5, -1, 0, 0, 0,
	}},
	{N: 3, K: 1, Stride: 5, Uplo: blas.Lower, Data: []complex128{
		-1, 1, 0, 0, 0,
		2, 3, 0, 0, 0,
		4, 5, 0, 0, 0,
	}},
	{N: 3, K: 2, Stride: 5, Uplo: blas.Upper, Data: []complex128{
		1, 2, 3, 0, 0,
		4, 5, -1, 0, 0,
		6, -2, -3, 0, 0,
	}},
	{N: 3, K: 2, Stride: 5, Uplo: blas.Lower, Data: []complex128{
		-2, -1, 1, 0, 0,
		-3, 2, 4, 0, 0,
		3, 5, 6, 0, 0,
	}},
}

func TestConvertHermBand(t *testing.T) {
	for _, test := range hermitianBandTests {
		colmajor := newHermitianBandColsFrom(test)
		if !sameHermitianBand(colmajor, test) {
			t.Errorf("unexpected result for row major to col major conversion:\n\tgot: %#v\n\tfrom:%#v",
				colmajor, test)
		}
		rowmajor := newHermitianBandFrom(colmajor)
		if !sameHermitianBand(rowmajor, test) {
			t.Errorf("unexpected result for col major to row major conversion:\n\tgot: %#v\n\twant:%#v",
				rowmajor, test)
		}
	}
}

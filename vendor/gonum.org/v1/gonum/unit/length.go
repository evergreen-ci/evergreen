// Code generated by "go generate gonum.org/v1/gonum/unit”; DO NOT EDIT.

// Copyright ©2014 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unit

import (
	"errors"
	"fmt"
	"math"
)

// Length represents a length in meters
type Length float64

const (
	Yottameter Length = 1e24
	Zettameter Length = 1e21
	Exameter   Length = 1e18
	Petameter  Length = 1e15
	Terameter  Length = 1e12
	Gigameter  Length = 1e9
	Megameter  Length = 1e6
	Kilometer  Length = 1e3
	Hectometer Length = 1e2
	Decameter  Length = 1e1
	Meter      Length = 1.0
	Decimeter  Length = 1e-1
	Centimeter Length = 1e-2
	Millimeter Length = 1e-3
	Micrometer Length = 1e-6
	Nanometer  Length = 1e-9
	Picometer  Length = 1e-12
	Femtometer Length = 1e-15
	Attometer  Length = 1e-18
	Zeptometer Length = 1e-21
	Yoctometer Length = 1e-24
)

// Unit converts the Length to a *Unit
func (l Length) Unit() *Unit {
	return New(float64(l), Dimensions{
		LengthDim: 1,
	})
}

// Length allows Length to implement a Lengther interface
func (l Length) Length() Length {
	return l
}

// From converts the unit into the receiver. From returns an
// error if there is a mismatch in dimension
func (l *Length) From(u Uniter) error {
	if !DimensionsMatch(u, Meter) {
		*l = Length(math.NaN())
		return errors.New("Dimension mismatch")
	}
	*l = Length(u.Unit().Value())
	return nil
}

func (l Length) Format(fs fmt.State, c rune) {
	switch c {
	case 'v':
		if fs.Flag('#') {
			fmt.Fprintf(fs, "%T(%v)", l, float64(l))
			return
		}
		fallthrough
	case 'e', 'E', 'f', 'F', 'g', 'G':
		p, pOk := fs.Precision()
		w, wOk := fs.Width()
		switch {
		case pOk && wOk:
			fmt.Fprintf(fs, "%*.*"+string(c), w, p, float64(l))
		case pOk:
			fmt.Fprintf(fs, "%.*"+string(c), p, float64(l))
		case wOk:
			fmt.Fprintf(fs, "%*"+string(c), w, float64(l))
		default:
			fmt.Fprintf(fs, "%"+string(c), float64(l))
		}
		fmt.Fprint(fs, " m")
	default:
		fmt.Fprintf(fs, "%%!%c(%T=%g m)", c, l, float64(l))
	}
}

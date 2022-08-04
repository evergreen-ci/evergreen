package util

import (
	"math"
	"strconv"

	"github.com/pkg/errors"
)

// min function for ints
func Min(a ...int) int {
	min := int(^uint(0) >> 1) // largest int
	for _, i := range a {
		if i < min {
			min = i
		}
	}
	return min
}

// TryParseFloat takes an input string and validates that it is a valid finite
// floating point number. The number is returned if valid, NaN if not
func TryParseFloat(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN(), errors.Wrapf(err, "parsing '%s' as float", s)
	}
	if math.IsNaN(f) {
		return math.NaN(), errors.Errorf("float '%s' is not a number", s)
	}
	if math.IsInf(f, 0) {
		return math.NaN(), errors.Errorf("float '%s' is either too large or too small", s)
	}
	return f, nil
}

// IsFiniteNumericFloat takes a float64 and checks that it is not +inf, -inf, or NaN
func IsFiniteNumericFloat(f float64) bool {
	if math.IsNaN(f) {
		return false
	}
	if math.IsInf(f, 0) {
		return false
	}
	return true
}

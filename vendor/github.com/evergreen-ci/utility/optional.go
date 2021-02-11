package utility

import "time"

// This file includes conversion functions for using pointers as optional
// values.

// TruePtr returns a pointer to a true value.
func TruePtr() *bool {
	res := true
	return &res
}

// FalsePtr returns a pointer to a false value.
func FalsePtr() *bool {
	res := false
	return &res
}

// ToBoolPtr returns a pointer to a bool value.
func ToBoolPtr(in bool) *bool {
	return &in
}

// FromBoolPtr returns the resolved boolean value from the input boolean
// pointer. For nil pointers, it returns false (i.e. the default value is false
// when the boolean is unspecified).
func FromBoolPtr(in *bool) bool {
	if in == nil {
		return false
	}
	return *in
}

// FromBoolTPtr returns the resolved boolean value from the input boolean
// pointer. For nil pointers, it returns true (i.e. the default value is true
// when the boolean is unspecified).
func FromBoolTPtr(in *bool) bool {
	if in == nil {
		return true
	}
	return *in
}

// ToIntPtr returns a pointer to an int value.
func ToIntPtr(in int) *int {
	return &in
}

// FromIntPtr returns the resolved int value from the input int pointer. For nil
// pointers, it returns 0.
func FromIntPtr(in *int) int {
	if in == nil {
		return 0
	}
	return *in
}

// ToUintPtr returns a pointer to a uint value.
func ToUintPtr(in uint) *uint {
	return &in
}

// FromUintPtr returns the resolved uint value from the input uint pointer. For
// nil pointers, it returns 0.
func FromUintPtr(in *uint) uint {
	if in == nil {
		return 0
	}
	return *in
}

// ToBytePtr returns a pointer to a byte value.
func ToBytePtr(in byte) *byte {
	return &in
}

// FromBytePtr returns the resolved byte value from the input byte pointer. For
// nil pointers, it returns 0.
func FromBytePtr(in *byte) byte {
	if in == nil {
		return 0
	}
	return *in
}

// ToFloat64Ptr returns a pointer to a float64 value.
func ToFloat64Ptr(in float64) *float64 {
	return &in
}

// FromFloat64Ptr returns the resolved float64 value from the input float64
// pointer. For nil pointers, it returns 0.
func FromFloat64Ptr(in *float64) float64 {
	if in == nil {
		return 0
	}
	return *in
}

// ToFloat32Ptr returns a pointer to a float32 value.
func ToFloat32Ptr(in float32) *float32 {
	return &in
}

// FromFloat32Ptr returns the resolved float32 value from the input float32
// pointer. For nil pointers, it returns 0.
func FromFloat32Ptr(in *float32) float32 {
	if in == nil {
		return 0
	}
	return *in
}

// ToTimeDurationPtr returns a pointer to a time.Duration value.
func ToTimeDurationPtr(in time.Duration) *time.Duration {
	return &in
}

// FromTimeDurationPtr returns the resolved time.Duration value from the input
// time.Duration pointer. For nil pointers, it returns time.Duration(0).
func FromTimeDurationPtr(in *time.Duration) time.Duration {
	if in == nil {
		return 0
	}
	return *in
}

// ToTimePtr returns a pointer to a time.Time value.
func ToTimePtr(in time.Time) *time.Time {
	return &in
}

// FromTimePtr returns the resolved time.Time value from the input time.Time
// pointer. For nil pointers, it returns the zero time.
func FromTimePtr(in *time.Time) time.Time {
	if in == nil {
		return time.Time{}
	}
	return *in
}

// ToStringPtr returns a pointer to a string.
func ToStringPtr(in string) *string {
	return &in
}

// FromStringPtr returns the resolved string value from the input string
// pointer. For nil pointers, it returns the empty string.
func FromStringPtr(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

// ToStringPtr returns a slice of string pointers from a slice of strings.
func ToStringPtrSlice(in []string) []*string {
	if in == nil {
		return nil
	}
	res := []*string{}
	for _, each := range in {
		res = append(res, ToStringPtr(each))
	}
	return res
}

// FromStringPtrSlice returns a slice of strings from a slice of string
// pointers.
func FromStringPtrSlice(in []*string) []string {
	if in == nil {
		return nil
	}
	res := []string{}
	for _, each := range in {
		res = append(res, FromStringPtr(each))
	}
	return res
}

package model

import (
	"go/types"

	"github.com/pkg/errors"
)

type convertType string

func stringToString(in string) string {
	return in
}

func stringToStringPtr(in string) *string {
	return &in
}

func stringPtrToString(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func stringPtrToStringPtr(in *string) *string {
	return in
}

func intToInt(in int) int {
	return in
}

func intToIntPtr(in int) *int {
	return &in
}

func intPtrToInt(in *int) int {
	if in == nil {
		return 0
	}
	return *in
}

func intPtrToIntPtr(in *int) *int {
	return in
}

func conversionFn(in types.Type, outIsPtr bool) (string, error) {
	if intype, inIsPrimitive := in.(*types.Basic); inIsPrimitive {
		switch intype.Kind() {
		case types.String:
			if outIsPtr {
				return "stringToStringPtr", nil
			}
			return "stringToString", nil
		case types.Int:
			if outIsPtr {
				return "intToIntPtr", nil
			}
			return "intToInt", nil
		}
	}
	if intype, inIsPtr := in.(*types.Pointer); inIsPtr {
		value, isPrimitive := intype.Elem().(*types.Basic)
		if !isPrimitive {
			return "", errors.New("pointers to complex objects not implemented yet")
		}
		switch value.Kind() {
		case types.String:
			if outIsPtr {
				return "stringPtrToStringPtr", nil
			}
			return "stringPtrToString", nil
		case types.Int:
			if outIsPtr {
				return "intPtrToIntPtr", nil
			}
			return "intPtrToInt", nil
		}
	}

	return "", errors.Errorf("converting type %s is not supported", in.String())
}

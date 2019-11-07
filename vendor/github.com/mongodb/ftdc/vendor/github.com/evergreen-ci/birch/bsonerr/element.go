package bsonerr

import (
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/pkg/errors"
)

// UninitializedElement is returned whenever any method is invoked on an uninitialized Element.
var UninitializedElement = errors.New("bson/ast/compact: Method call on uninitialized Element")

// InvalidWriter indicates that a type that can't be written into was passed to a writer method.
var InvalidWriter = errors.New("bson: invalid writer provided")

// InvalidString indicates that a BSON string value had an incorrect length.
var InvalidString = errors.New("invalid string value")

// InvalidBinarySubtype indicates that a BSON binary value had an undefined subtype.
var InvalidBinarySubtype = errors.New("invalid BSON binary Subtype")

// InvalidBooleanType indicates that a BSON boolean value had an incorrect byte.
var InvalidBooleanType = errors.New("invalid value for BSON Boolean Type")

// StringLargerThanContainer indicates that the code portion of a BSON JavaScript code with scope
// value is larger than the specified length of the entire value.
var StringLargerThanContainer = errors.New("string size is larger than the JavaScript code with scope container")

// InvalidElement indicates that a bson.Element had invalid underlying BSON.
var InvalidElement = errors.New("invalid Element")

// ElementType specifies that a method to obtain a BSON value an incorrect type was called on a bson.Value.
type ElementType struct {
	Method string
	Type   bsontype.Type
}

func NewElementTypeError(method string, t bsontype.Type) error {
	return ElementType{
		Method: method,
		Type:   t,
	}
}

// Error implements the error interface.
func (ete ElementType) Error() string {
	return "Call of " + ete.Method + " on " + ete.Type.String() + " type"
}

package bsonerr

import "github.com/pkg/errors"

// NilReader indicates that an operation was attempted on a nil bson.Reader.
var NilReader = errors.New("nil reader")

// InvalidReadOnlyDocument indicates that the underlying bytes of a bson.Reader are invalid.
var InvalidReadOnlyDocument = errors.New("invalid read-only document")

// InvalidKey indicates that the BSON representation of a key is missing a null terminator.
var InvalidKey = errors.New("invalid document key")

// InvalidArrayKey indicates that a key that isn't a positive integer was used to lookup an
// element in an array.
var InvalidArrayKey = errors.New("invalid array key")

// InvalidLength indicates that a length in a binary representation of a BSON document is invalid.
var InvalidLength = errors.New("document length is invalid")

// EmptyKey indicates that no key was provided to a Lookup method.
var EmptyKey = errors.New("empty key provided")

// NilElement indicates that a nil element was provided when none was expected.
var NilElement = errors.New("element is nil")

// NilDocument indicates that an operation was attempted on a nil *bson.Document.
var NilDocument = errors.New("document is nil")

// InvalidDocumentType indicates that a type which doesn't represent a BSON document was
// was provided when a document was expected.
var InvalidDocumentType = errors.New("invalid document type")

// InvalidDepthTraversal indicates that a provided path of keys to a nested value in a document
// does not exist.
//
// TODO(skriptble): This error message is pretty awful.
// Please fix.
var InvalidDepthTraversal = errors.New("invalid depth traversal")

// ElementNotFound indicates that an Element matching a certain condition does not exist.
var ElementNotFound = errors.New("element not found")

// OutOfBounds indicates that an index provided to access something was invalid.
var OutOfBounds = errors.New("out of bounds")

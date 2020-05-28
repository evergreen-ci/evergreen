package birch

// Marshaler describes types that know how to marshal a document
// representation of themselves into bson. Do not use this interface
// for types that would marshal themselves into values.
type Marshaler interface {
	MarshalBSON() ([]byte, error)
}

// Unmarshaler describes types that can take a byte slice
// representation of bson and poulate themselves from this data.
type Unmarshaler interface {
	UnmarshalBSON([]byte) error
}

// DocumentMarshaler describes types that are able to produce Document
// represntations of themselves.
type DocumentMarshaler interface {
	MarshalDocument() (*Document, error)
}

// DocumentUnmarshaler describes a type that can populate itself from
// a document.
type DocumentUnmarshaler interface {
	UnmarshalDocument(*Document) error
}

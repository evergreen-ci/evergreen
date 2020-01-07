package jsonx

// Type describes
type Type int

const (
	// String is a json string
	String Type = iota

	// Number is a json number.
	Number

	// Bool is a boolean value that is either true or false.
	Bool

	// Null is a null json value.
	Null

	// NumberInteger refers to integer values. This translates to a
	// JSON number.
	NumberInteger

	// NumberInteger refers to a float/double value. This translates to a
	// JSON number.
	NumberDouble

	// ObjectValues are json objects.
	ObjectValue

	// ArrayValues are json arrays.
	ArrayValue
)

// String returns a string representation of the type.
func (t Type) String() string {
	switch t {
	case Null:
		return "null"
	case Bool:
		return "bool"
	case Number:
		return "number"
	case String:
		return "string"
	case ObjectValue:
		return "object"
	case ArrayValue:
		return "array"
	case NumberInteger:
		return "integer"
	case NumberDouble:
		return "double"
	default:
		return ""
	}
}

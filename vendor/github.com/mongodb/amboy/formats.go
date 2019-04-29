package amboy

// Format defines a sequence of constants used to distinguish between
// different serialization formats for job objects used in the
// amboy.ConvertTo and amboy.ConvertFrom functions, which support the
// functionality of the Export and Import methods in the job
// interface.
type Format int

// Supported values of the Format type, which represent different
// supported serialization methods..
const (
	BSON Format = iota
	YAML
	JSON
	BSON2
)

// String implements fmt.Stringer and pretty prints the format name.
func (f Format) String() string {
	switch f {
	case JSON:
		return "json"
	case BSON, BSON2:
		return "bson"
	case YAML:
		return "yaml"
	default:
		return "INVALID"
	}
}

// IsValid returns true if when a valid format is specified, and false otherwise
func (f Format) IsValid() bool {
	switch f {
	case JSON, YAML, BSON, BSON2:
		return true
	default:
		return false
	}
}

package gimlet

import "net/http"

type httpMethod int

// Typed constants for specifying HTTP method types on routes.
const (
	get httpMethod = iota
	put
	post
	delete
	patch
	head
)

func (m httpMethod) String() string {
	switch m {
	case get:
		return http.MethodGet
	case put:
		return http.MethodPut
	case delete:
		return http.MethodDelete
	case patch:
		return http.MethodPatch
	case post:
		return http.MethodPost
	case head:
		return http.MethodHead
	default:
		return ""
	}
}

// OutputFormat enumerates output formats for response writers.
type OutputFormat int

// Enumerations of supported output formats used by gimlet rendering
// facilities.
const (
	JSON OutputFormat = iota
	TEXT
	HTML
	YAML
	BINARY
)

// IsValid provides a predicate to validate OutputFormat values.
func (o OutputFormat) IsValid() bool {
	switch o {
	case JSON, TEXT, HTML, BINARY, YAML:
		return true
	default:
		return false
	}
}

func (o OutputFormat) String() string {
	switch o {
	case JSON:
		return "json"
	case TEXT:
		return "text"
	case HTML:
		return "html"
	case BINARY:
		return "binary"
	case YAML:
		return "yaml"
	default:
		return "text"
	}
}

// ContentType returns a mime content-type string for output formats
// produced by gimlet's rendering.
func (o OutputFormat) ContentType() string {
	switch o {
	case JSON:
		return "application/json; charset=utf-8"
	case TEXT:
		return "text/plain; charset=utf-8"
	case HTML:
		return "application/html; charset=utf-8"
	case BINARY:
		return "application/octet-stream"
	case YAML:
		return "application/yaml; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

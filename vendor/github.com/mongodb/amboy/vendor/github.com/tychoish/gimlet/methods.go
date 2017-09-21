package gimlet

type httpMethod int

// Typed constants for specifying HTTP method types on routes.
const (
	get httpMethod = iota
	put
	post
	delete
	patch
)

func (m httpMethod) String() string {
	switch m {
	case get:
		return "GET"
	case put:
		return "PUT"
	case delete:
		return "DELETE"
	case patch:
		return "PATCH"
	case post:
		return "POST"
	default:
		return ""
	}
}

type OutputFormat int

const (
	JSON OutputFormat = iota
	TEXT
	HTML
	YAML
	BINARY
)

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

func (o OutputFormat) ContentType() string {
	switch o {
	case JSON:
		return "application/json; charset=utf-8"
	case TEXT:
		return "plain/text; charset=utf-8"
	case HTML:
		return "application/html; charset=utf-8"
	case BINARY:
		return "application/octet-stream"
	case YAML:
		return "application/yaml; charset=utf-8"
	default:
		return "plain/text; charset=utf-8"
	}
}

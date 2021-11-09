package gimlet

import (
	"io"
	"net/http"
)

// Renderer descibes an interface used to provide template caching and
// streaming for Go standard library templates.
type Renderer interface {
	GetTemplate(...string) (RenderTemplate, error)
	Render(io.Writer, interface{}, string, ...string) error
	Stream(http.ResponseWriter, int, interface{}, string, ...string)
	WriteResponse(http.ResponseWriter, int, interface{}, string, ...string)
}

// RenderTemplate describes the common interface used by Renderer
// implementations to interact with text or html templates.
type RenderTemplate interface {
	ExecuteTemplate(io.Writer, string, interface{}) error
}

// RendererOptions descibes the common options passed to the renderer
// constructors.
type RendererOptions struct {
	Functions    map[string]interface{}
	Directory    string
	Encoding     string
	DisableCache bool
}

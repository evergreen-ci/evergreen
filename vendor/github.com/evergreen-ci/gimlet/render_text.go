package gimlet

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type textRenderer struct {
	cache map[string]*template.Template
	mu    sync.Mutex
	opts  RendererOptions
}

// NewTextRenderer returns an implementation of Renderer that  and
// provides wrappers around  text/template for template caching and
// streaming.
func NewTextRenderer(opts RendererOptions) Renderer {
	if opts.Encoding == "" {
		opts.Encoding = "UTF-8"
	}

	return &textRenderer{
		cache: map[string]*template.Template{},
		opts:  opts,
	}
}

func (r *textRenderer) GetTemplate(filenames ...string) (RenderTemplate, error) {
	var (
		tmpl     *template.Template
		cacheKey string
		err      error
		ok       bool
	)

	if !r.opts.DisableCache {
		// generate a cache key by joining filenames with null byte (can't appear in filenames)
		cacheKey = strings.Join(filenames, "\x00")
		if tmpl, ok = r.cache[cacheKey]; ok {
			return tmpl.Clone()
		}
	}

	// cache miss (or cache is turned off) - try to load the templates from the filesystem
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := make([]string, 0, len(filenames))
	for _, v := range filenames {
		paths = append(paths, filepath.Join(r.opts.Directory, v))
	}

	tmpl = template.New(cacheKey).Funcs(r.opts.Functions)
	tmpl, err = tmpl.ParseFiles(paths...)
	if err != nil {
		return nil, err
	}

	if !r.opts.DisableCache {
		r.cache[cacheKey] = tmpl
	}

	return tmpl.Clone()
}

// Text loads the given set of template files and executes the template named entryPoint against
// the context data, writing the result to out. Returns error if the template could not be
// loaded or if executing the template failed.
func (r *textRenderer) Render(out io.Writer, data interface{}, entryPoint string, files ...string) error {
	t, err := r.GetTemplate(files...)
	if err != nil {
		return err
	}

	return t.ExecuteTemplate(out, entryPoint, data)
}

func (r *textRenderer) WriteResponse(w http.ResponseWriter, status int, data interface{}, entryPoint string, files ...string) {
	out := &bytes.Buffer{}
	err := r.Render(out, data, entryPoint, files...)
	if err != nil {
		WriteTextInternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

// StreamText calls Text() on its args and writes the output directly to the response with a text/plain Content-Type.
// It calls Text() so that it can accept go text templates rather than simply accepting text.
// Does not buffer the executed template before rendering, so it can be used for writing
// really large responses without consuming memory. If executing the template fails, the status
// code is not changed; it will remain set to the provided value.
func (r *textRenderer) Stream(w http.ResponseWriter, status int, data interface{}, entryPoint string, files ...string) {
	w.Header().Set("Content-Type", "text/plain; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	grip.Error(message.WrapError(r.Render(w, data, entryPoint, files...), message.Fields{
		"entry":     entryPoint,
		"files":     files,
		"operation": "stream rendering",
		"mode":      "text",
	}))
}

package gimlet

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type htmlRenderer struct {
	cache map[string]*template.Template
	mu    sync.Mutex
	opts  RendererOptions
}

// NewHTMLRenderer returns a Renderer implementation that wraps
// html/template and provides caching and streaming to http responses.
func NewHTMLRenderer(opts RendererOptions) Renderer {
	if opts.Encoding == "" {
		opts.Encoding = "UTF-8"
	}

	return &htmlRenderer{
		cache: map[string]*template.Template{},
		opts:  opts,
	}
}

func (r *htmlRenderer) GetTemplate(filenames ...string) (RenderTemplate, error) {
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

func (r *htmlRenderer) Render(out io.Writer, data interface{}, entryPoint string, files ...string) error {
	t, err := r.GetTemplate(files...)
	if err != nil {
		return err
	}

	return t.ExecuteTemplate(out, entryPoint, data)
}

func (r *htmlRenderer) WriteResponse(w http.ResponseWriter, status int, data interface{}, entryPoint string, files ...string) {
	out := &bytes.Buffer{}
	err := r.Render(out, data, entryPoint, files...)
	if err != nil {
		WriteTextInternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

func (r *htmlRenderer) Stream(w http.ResponseWriter, status int, data interface{}, entryPoint string, files ...string) {
	w.Header().Set("Content-Type", "text/html; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	grip.Error(message.WrapError(r.Render(w, data, entryPoint, files...), message.Fields{
		"entry":     entryPoint,
		"files":     files,
		"operation": "stream rendering",
		"mode":      "text",
	}))
}

package render

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

// Render maintains a template cache and exposes methods for rendering JSON or sets of templates
// to an HTTP response.
type Render struct {
	cache      map[string]*template.Template
	cacheMutex sync.Mutex
	opts       Options
}

type Options struct {
	// A set of functions that are available for templates to call during execution
	Funcs template.FuncMap

	// The root directory to load templates from.
	Directory string

	// The character set to specify in HTTP responses. Defaults to UTF-8.
	Encoding string

	// If DisableCache is true, templates will be reloaded on every call.
	// By default, caching is enabled, so files are only loaded and parsed the first time.
	DisableCache bool
}

//New creates a new instance of Render with the given options, and an empty template cache.
func New(opts Options) *Render {
	if opts.Encoding == "" {
		opts.Encoding = "UTF-8"
	}

	return &Render{
		opts:  opts,
		cache: map[string]*template.Template{},
	}
}

// Returns a template for the given set of filenames by loading it from cache if available,
// or loading and parsing the files from disk. Returns the template if found or an error if
// the template couldn't be loaded.
func (r *Render) getTemplate(filenames ...string) (*template.Template, error) {

	var cacheKey string
	if !r.opts.DisableCache {
		// generate a cache key by joining filenames with null byte (can't appear in filenames)
		cacheKey = strings.Join(filenames, "\x00")
		if template, ok := r.cache[cacheKey]; ok {
			return template, nil
		}
	}

	// cache miss (or cache is turned off) - try to load the templates from the filesystem
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()
	paths := make([]string, 0, len(filenames))
	for _, v := range filenames {
		paths = append(paths, filepath.Join(r.opts.Directory, v))
	}

	tmpl := template.New(cacheKey).Funcs(r.opts.Funcs)
	tmpl, err := tmpl.ParseFiles(paths...)
	if err != nil {
		return nil, err
	}

	if !r.opts.DisableCache {
		r.cache[cacheKey] = tmpl
	}
	return tmpl, nil
}

// HTML loads the given set of template files and executes the template named entryPoint against
// the context data, writing the result to out. Returns error if the template could not be
// loaded or if executing the template failed.
func (r *Render) HTML(out io.Writer, data interface{}, entryPoint string, files ...string) error {
	t, err := r.getTemplate(files...)
	if err != nil {
		return err
	}
	err = t.ExecuteTemplate(out, entryPoint, data)
	if err != nil {
		return err
	}
	return nil
}

// WriteHTML calls HTML() on its args and writes the output to the response with the given status.
// If the template can't be loaded or executed, the status is set to 500 and error details
// are written to the response body.
func (r *Render) WriteHTML(w http.ResponseWriter, status int, data interface{}, entryPoint string, files ...string) {
	out := &bytes.Buffer{}
	err := r.HTML(out, data, entryPoint, files...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	w.Write(out.Bytes())
}

// StreamHTML calls HTML() on its args and writes the output directly to the response.
// Does not buffer the executed template before rendering, so it can be used for writing
// really large responses without consuming memory. If executing the template fails, the status
// code is not changed; it will remain set to the provided value.
func (r *Render) StreamHTML(w http.ResponseWriter, status int, data interface{}, entryPoint string, files ...string) error {
	w.Header().Set("Content-Type", "text/html; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	err := r.HTML(w, data, entryPoint, files...)
	return err
}

// WriteJSON marshals data to JSON and writes it to the response with the given status code.
// If marshaling fails, the status is set to 500 and error details are written to the response body.
func (r *Render) WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset="+r.opts.Encoding)
	w.WriteHeader(status)
	w.Write(out)
}

// WriteBinary writes raw bytes as binary data to the reponse, with the given status code.
func (r *Render) WriteBinary(w http.ResponseWriter, status int, v []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(status)
	w.Write(v)
}

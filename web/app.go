package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/codegangsta/inject"
	"github.com/evergreen-ci/evergreen"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const JSONMarshalError = "Could not marshal data to JSON"

type App struct {
	Router         *http.Handler
	TemplateFolder string
	TemplateFuncs  map[string]interface{}
	TemplateCache  map[string]*template.Template
	CacheTemplates bool
	inject.Injector
}

type HandlerApp interface {
	RespondStreamTemplate(templateNames []string, entryPointName string, data interface{}) HTTPResponse
	RespondTemplate(templateNames []string, entryPointName string, data interface{}) HTTPResponse
	MakeHandler(handlers ...interface{}) http.HandlerFunc
}

func NewApp() *App {
	return &App{
		TemplateFuncs:  make(map[string]interface{}),
		TemplateCache:  make(map[string]*template.Template),
		CacheTemplates: true,
		Injector:       inject.New(),
	}
}

func (ae *App) RespondTemplate(templateNames []string, entryPointName string, data interface{}) HTTPResponse {
	cacheKey := strings.Join(templateNames, ":") + ":" + entryPointName
	tmpl := ae.TemplateCache[cacheKey]

	if ae.CacheTemplates && tmpl != nil { //cache hit
		return &TemplateResponse{entryPointName, tmpl, data, false}
	} else { // cache miss.
		templatePaths := make([]string, len(templateNames))
		for i, v := range templateNames {
			templatePaths[i] = filepath.Join(ae.TemplateFolder, v)
		}
		tmpl := template.New(templateNames[0]).Funcs(ae.TemplateFuncs)
		tmpl, err := tmpl.ParseFiles(templatePaths...)
		if err != nil {
			return StringResponse{"Error: " + err.Error(), 500}
		}
		ae.TemplateCache[cacheKey] = tmpl
		return &TemplateResponse{entryPointName, tmpl, data, false}
	}
}

func (ae *App) RespondStreamTemplate(templateNames []string, entryPointName string, data interface{}) HTTPResponse {
	cacheKey := strings.Join(templateNames, ":") + ":" + entryPointName
	tmpl := ae.TemplateCache[cacheKey]

	if tmpl != nil { //cache hit
		return &TemplateResponse{entryPointName, tmpl, data, true}
	} else { // cache miss.
		templatePaths := make([]string, len(templateNames))
		for i, v := range templateNames {
			templatePaths[i] = filepath.Join(ae.TemplateFolder, v)
		}
		tmpl := template.New(templateNames[0]).Funcs(ae.TemplateFuncs)
		tmpl, err := tmpl.ParseFiles(templatePaths...)
		if err != nil {
			return StringResponse{"Error: " + err.Error(), 500}
		}
		ae.TemplateCache[cacheKey] = tmpl
		return &TemplateResponse{entryPointName, tmpl, data, true}
	}
}

func (ae *App) MakeHandler(handlers ...interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		child := inject.New()
		child.SetParent(ae.Injector)
		child.MapTo(w, (*http.ResponseWriter)(nil))
		child.Map(r)
	handlerLoop:
		for _, reqProcessor := range handlers {
			result, err := child.Invoke(reqProcessor)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(os.Stderr, "error in url %v: %v\n", r.URL, err)
				return
			}

			if len(result) == 0 {
				continue
			} else {
				valAsInterface := result[0].Interface()
				if resp, ok := valAsInterface.(HTTPResponse); ok {
					err := resp.Render(w)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprintln(os.Stderr, "error: %v", err)
					}
					if reflect.TypeOf(valAsInterface) != reflect.TypeOf(ChainHttpResponse{}) {
						break handlerLoop
					}
				}
			}
		}
	}
}

//HTTPResponse is an interface that is responsible for encapsulating how a response is rendered to an HTTP client.
type HTTPResponse interface {
	Render(w http.ResponseWriter) error
}

//StringResponse will write a simple string directly to the output
type StringResponse struct {
	Body       string
	StatusCode int
}

//RawHTTPResponse will write a sequence of bytes directly to the output.
type RawHTTPResponse []byte

//JSONResponse will marshal "Data" as json to the output, and set StatusCode accordingly.
type JSONResponse struct {
	Data       interface{}
	StatusCode int
}

//YAMLResponse will marshal "Data" as json to the output, and set StatusCode accordingly.
type YAMLResponse struct {
	Data       interface{}
	StatusCode int
}

//TemplateResponse will execute TemplateSet against Data with the root template named by TemplateName.
type TemplateResponse struct {
	TemplateName string
	TemplateSet  *template.Template
	Data         interface{}
	Stream       bool
}

//NullHttpResponse doesn't do anything to the response.
//It can be used if the handler has already written the response directly
//so no further action needs to be taken.
type NullHttpResponse struct{}

type ChainHttpResponse struct{}

func (rw RawHTTPResponse) Render(w http.ResponseWriter) error {
	_, err := w.Write(rw)
	return err
}

func (nh NullHttpResponse) Render(w http.ResponseWriter) error {
	return nil
}

func (nh ChainHttpResponse) Render(w http.ResponseWriter) error {
	return nil
}

func (sr StringResponse) Render(w http.ResponseWriter) error {
	if sr.StatusCode != 0 {
		w.WriteHeader(sr.StatusCode)
	}
	_, err := w.Write([]byte(sr.Body))
	return err
}

func (tr TemplateResponse) Render(w http.ResponseWriter) error {
	var buf bytes.Buffer
	if tr.Stream {
		err := tr.TemplateSet.ExecuteTemplate(w, tr.TemplateName, tr.Data)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(fmt.Sprintf("error: \"%v\"", err)))
			return err
		}
	} else {
		err := tr.TemplateSet.ExecuteTemplate(&buf, tr.TemplateName, tr.Data)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(fmt.Sprintf("error: \"%v\"", err)))
			return err
		}
		w.Write(buf.Bytes())
	}
	return nil
}

func (jr JSONResponse) Render(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	statusCode := jr.StatusCode
	jsonBytes, err := json.Marshal(jr.Data)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "ERROR MARSHALING JSON: %v", err)
		statusCode = http.StatusInternalServerError
		jsonBytes = []byte(fmt.Sprintf(`{"error":"%v"}`, JSONMarshalError))
	}

	if statusCode != 0 {
		w.WriteHeader(statusCode)
	}

	w.Write(jsonBytes)
	return nil
}

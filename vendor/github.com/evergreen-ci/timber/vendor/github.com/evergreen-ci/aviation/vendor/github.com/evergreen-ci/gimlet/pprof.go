package gimlet

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	runtimeTrace "runtime/trace"
	"strconv"
	"strings"
	"time"
)

// GetPProfApp produces an APIApp that has routes registered for pprof
// access. The routes themselves are copied from the standard library
// but configured for integration into an existing gimlet-based application.
func GetPProfApp() *APIApp {
	app := NewApp()
	app.SetPrefix("/debug/pprof")
	app.NoVersions = true

	app.AddRoute("/").Get().Handler(pprofIndex)
	app.AddRoute("/heap").Get().Handler(pprofIndex)
	app.AddRoute("/block").Get().Handler(pprofIndex)
	app.AddRoute("/goroutine").Get().Handler(pprofIndex)
	app.AddRoute("/mutex").Get().Handler(pprofIndex)
	app.AddRoute("/threadcreate").Get().Handler(pprofIndex)
	app.AddRoute("/cmdline").Get().Handler(pprofCmdline)
	app.AddRoute("/profile").Get().Handler(pprofProfile)
	app.AddRoute("/symbol").Get().Handler(pprofSymbol)
	app.AddRoute("/trace").Get().Handler(pprofTrace)

	return app
}

// ******************************************************************************
// The below was copied from the standard library net/http/pprof because we want
// to use our own router. This is identical with the exception of the init
// function (which registered handlers), which has been deleted.
// ******************************************************************************

// cmdline responds with the running program's
// command line, with arguments separated by NUL bytes.
// The package initialization registers it as /debug/pprof/cmdline.
func pprofCmdline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, strings.Join(os.Args, "\x00"))
}

func pprofSleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// profile responds with the pprof-formatted cpu profile.
// The package initialization registers it as /debug/pprof/profile.
func pprofProfile(w http.ResponseWriter, r *http.Request) {
	sec, _ := strconv.ParseInt(r.FormValue("seconds"), 10, 64)
	if sec == 0 {
		sec = 30
	}

	// Set Content Type assuming StartCPUProfile will work,
	// because if it does it starts writing.
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := pprof.StartCPUProfile(w); err != nil {
		// StartCPUProfile failed, so no writes yet.
		// Can change header back to text content
		// and send error code.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not enable CPU profiling: %s\n", err)
		return
	}
	pprofSleep(r.Context(), time.Duration(sec)*time.Second)
	pprof.StopCPUProfile()
}

// trace responds with the execution trace in binary form.
// Tracing lasts for duration specified in seconds GET parameter, or for 1 second if not specified.
// The package initialization registers it as /debug/pprof/trace.
func pprofTrace(w http.ResponseWriter, r *http.Request) {
	sec, err := strconv.ParseFloat(r.FormValue("seconds"), 64)
	if sec <= 0 || err != nil {
		sec = 1
	}

	// Set Content Type assuming trace.Start will work,
	// because if it does it starts writing.
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := runtimeTrace.Start(w); err != nil {
		// runtimeTrace.Start failed, so no writes yet.
		// Can change header back to text content and send error code.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not enable tracing: %s\n", err)
		return
	}
	pprofSleep(r.Context(), time.Duration(sec*float64(time.Second)))
	runtimeTrace.Stop()
}

// symbol looks up the program counters listed in the request,
// responding with a table mapping program counters to function names.
// The package initialization registers it as /debug/pprof/symbol.
func pprofSymbol(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	// We have to read the whole POST body before
	// writing any output. Buffer the output here.
	var buf bytes.Buffer

	// We don't know how many symbols we have, but we
	// do have symbol information. Pprof only cares whether
	// this number is 0 (no symbols available) or > 0.
	fmt.Fprintf(&buf, "num_symbols: 1\n")

	var b *bufio.Reader
	if r.Method == "POST" {
		b = bufio.NewReader(r.Body)
	} else {
		b = bufio.NewReader(strings.NewReader(r.URL.RawQuery))
	}

	for {
		word, err := b.ReadSlice('+')
		if err == nil {
			word = word[0 : len(word)-1] // trim +
		}
		pc, _ := strconv.ParseUint(string(word), 0, 64)
		if pc != 0 {
			f := runtime.FuncForPC(uintptr(pc))
			if f != nil {
				fmt.Fprintf(&buf, "%#x %s\n", pc, f.Name())
			}
		}

		// Wait until here to check for err; the last
		// symbol will have an err because it doesn't end in +.
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(&buf, "reading request: %v\n", err)
			}
			break
		}
	}

	_, _ = w.Write(buf.Bytes())
}

type pprofHandler string

func (name pprofHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	debug, _ := strconv.Atoi(r.FormValue("debug"))
	p := pprof.Lookup(string(name))
	if p == nil {
		w.WriteHeader(404)
		fmt.Fprintf(w, "Unknown profile: %s\n", name)
		return
	}
	gc, _ := strconv.Atoi(r.FormValue("gc"))
	if name == "heap" && gc > 0 {
		runtime.GC()
	}
	_ = p.WriteTo(w, debug)
}

// index responds with the pprof-formatted profile named by the request.
// For example, "/debug/pprof/heap" serves the "heap" profile.
// Index responds to a request for "/debug/pprof/" with an HTML page
// listing the available profiles.
func pprofIndex(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
		name := strings.TrimPrefix(r.URL.Path, "/debug/pprof/")
		if name != "" {
			pprofHandler(name).ServeHTTP(w, r)
			return
		}
	}

	profiles := pprof.Profiles()
	if err := pprofIndexTmpl.Execute(w, profiles); err != nil {
		log.Print(err)
	}
}

var pprofIndexTmpl = template.Must(template.New("index").Parse(`<html>
<head>
<title>/debug/pprof/</title>
</head>
<body>
/debug/pprof/<br>
<br>
profiles:<br>
<table>
{{range .}}
<tr><td align=right>{{.Count}}<td><a href="{{.Name}}?debug=1">{{.Name}}</a>
{{end}}
</table>
<br>
<a href="goroutine?debug=2">full goroutine stack dump</a><br>
</body>
</html>
`))

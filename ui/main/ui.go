package main

import (
	"flag"
	"fmt"
	htmlTemplate "html/template"
	"net/http"
	"os"
	"path/filepath"
	textTemplate "text/template"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/ui"
	"github.com/evergreen-ci/render"
	"gopkg.in/tylerb/graceful.v1"
)

const UIPort = ":9090"

var (
	// requestTimeout is the duration to wait until killing
	// active requests and stopping the server.
	requestTimeout = 10 * time.Second
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s serves Evergreen's web interface.\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Supported flags are:\n")
		flag.PrintDefaults()
	}
}

func main() {

	settings := evergreen.GetSettingsOrExit()
	home := evergreen.FindEvergreenHome()
	uis, err := ui.New(settings, home)
	if err != nil {
		fmt.Println("Failed to create ui server: %v", err)
		os.Exit(1)
	}
	router, err := uis.NewRouter()
	if err != nil {
		fmt.Println("Failed to create router:", err)
		os.Exit(1)
	}

	webHome := filepath.Join(uis.Home, "public")

	functionOptions := ui.FuncOptions{webHome, uis.Settings.Ui.HelpUrl, true, router}

	functions, err := ui.MakeTemplateFuncs(functionOptions, uis.Settings.SuperUsers)
	htmlFunctions := htmlTemplate.FuncMap(functions)
	textFunctions := textTemplate.FuncMap(functions)

	if err != nil {
		fmt.Println("Failed to create template function map:", err)
		os.Exit(1)
	}

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(uis.Home, ui.WebRootPath, ui.Templates),
		DisableCache: !uis.Settings.Ui.CacheTemplates,
		HtmlFuncs:    htmlFunctions,
		TextFuncs:    textFunctions,
	})
	err = uis.InitPlugins()
	if err != nil {
		fmt.Println("WARNING: Error initializing plugins:", err)
	}

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(webHome)))
	n.Use(ui.NewLogger())
	n.Use(negroni.HandlerFunc(ui.UserMiddleware(uis.UserManager)))
	n.UseHandler(router)
	graceful.Run(uis.Settings.Ui.HttpListenAddr, requestTimeout, n)
	evergreen.Logger.Logf(slogger.INFO, "UI server cleanly terminated")
}

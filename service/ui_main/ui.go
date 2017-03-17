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

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/render"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/send"
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
	grip.SetName("ui-server")

	settings := evergreen.GetSettingsOrExit()

	if settings.Ui.LogFile == "" {
		sender := send.MakeCallSiteConsoleLogger(2)
		defer sender.Close()
		grip.CatchEmergencyFatal(grip.SetSender(sender))
	} else {
		sender, err := send.MakeCallSiteFileLogger(settings.Ui.LogFile, 2)
		grip.CatchEmergencyFatal(err)
		defer sender.Close()
		grip.CatchEmergencyFatal(grip.SetSender(sender))
	}
	evergreen.SetLegacyLogger()
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)

	defer util.RecoverAndLogStackTrace()

	home := evergreen.FindEvergreenHome()
	if home == "" {
		fmt.Println("EVGHOME environment variable must be set to run UI server")
		os.Exit(1)
	}
	uis, err := service.NewUIServer(settings, home)
	if err != nil {
		fmt.Println("Failed to create ui server: %v", err)
		os.Exit(1)
	}
	router, err := uis.NewRouter()
	if err != nil {
		fmt.Println("Failed to create router:", err)
		os.Exit(1)
	}

	webHome := filepath.Join(home, "public")

	functionOptions := service.FuncOptions{webHome, settings.Ui.HelpUrl, true, router}

	functions, err := service.MakeTemplateFuncs(functionOptions, settings.SuperUsers)
	htmlFunctions := htmlTemplate.FuncMap(functions)
	textFunctions := textTemplate.FuncMap(functions)

	if err != nil {
		fmt.Println("Failed to create template function map:", err)
		os.Exit(1)
	}

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, service.WebRootPath, service.Templates),
		DisableCache: !settings.Ui.CacheTemplates,
		HtmlFuncs:    htmlFunctions,
		TextFuncs:    textFunctions,
	})
	err = uis.InitPlugins()
	if err != nil {
		grip.Warningln("problem initializing plugins:", err)
	}

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(webHome)))
	n.Use(service.NewLogger())
	n.Use(negroni.HandlerFunc(service.UserMiddleware(uis.UserManager)))
	n.UseHandler(router)
	graceful.Run(settings.Ui.HttpListenAddr, requestTimeout, n)
	grip.Info("UI server cleanly terminated")
}

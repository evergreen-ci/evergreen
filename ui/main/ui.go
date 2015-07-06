package main

import (
	"flag"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/ui"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/sessions"
	"gopkg.in/tylerb/graceful.v1"
	"net/http"
	"os"
	"path/filepath"
	"time"
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
	if settings.Ui.LogFile != "" {
		evergreen.SetLogger(settings.Ui.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	home := evergreen.FindEvergreenHome()

	userManager, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		fmt.Println("Failed to create user manager:", err)
		os.Exit(1)
	}

	cookieStore := sessions.NewCookieStore([]byte(settings.Ui.Secret))

	uis := ui.UIServer{
		nil,             // render
		settings.Ui.Url, // RootURL
		userManager,     // User Manager
		*settings,       // mci settings
		cookieStore,     // cookiestore
		nil,             // plugin panel manager
	}
	router, err := uis.NewRouter()
	if err != nil {
		fmt.Println("Failed to create router:", err)
		os.Exit(1)
	}

	webHome := filepath.Join(home, "public")

	functionOptions := ui.FuncOptions{webHome, settings.Ui.HelpUrl, true, router}

	functions, err := ui.MakeTemplateFuncs(functionOptions, settings.SuperUsers)
	if err != nil {
		fmt.Println("Failed to create template function map:", err)
		os.Exit(1)
	}

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, ui.WebRootPath, ui.Templates),
		DisableCache: !settings.Ui.CacheTemplates,
		Funcs:        functions,
	})
	uis.InitPlugins()

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(webHome)))
	n.Use(ui.NewLogger())
	n.Use(negroni.HandlerFunc(ui.UserMiddleware(userManager)))
	n.UseHandler(router)
	graceful.Run(settings.Ui.HttpListenAddr, requestTimeout, n)
	evergreen.Logger.Logf(slogger.INFO, "UI server cleanly terminated")
}

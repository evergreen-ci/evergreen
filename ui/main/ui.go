package main

import (
	"fmt"
	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/ui"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/sessions"
	"net/http"
	"os"
	"path/filepath"
)

const UIPort = ":9090"

func main() {
	mciSettings := evergreen.MustConfig()
	if mciSettings.Ui.LogFile != "" {
		evergreen.SetLogger(mciSettings.Ui.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	home, err := evergreen.FindMCIHome()
	if err != nil {
		fmt.Println("Can't find mci home", err)
		os.Exit(1)
	}

	crowdManager, err := auth.NewCrowdUserManager(
		mciSettings.Crowd.Username,
		mciSettings.Crowd.Password,
		mciSettings.Crowd.Urlroot,
	)
	if err != nil {
		fmt.Println("Failed to create user manager:", err)
		os.Exit(1)
	}

	cookieStore := sessions.NewCookieStore([]byte(mciSettings.Ui.Secret))

	uis := ui.UIServer{
		nil,                // render
		mciSettings.Ui.Url, // RootURL
		crowdManager,       // User Manager
		*mciSettings,       // mci settings
		cookieStore,        // cookiestore
		nil,                // plugin panel manager
	}
	router, err := uis.NewRouter()
	if err != nil {
		fmt.Println("Failed to create router:", err)
		os.Exit(1)
	}

	webHome := filepath.Join(home, "public")

	functionOptions := ui.FuncOptions{webHome, mciSettings.Ui.HelpUrl, true, router}

	functions, err := ui.MakeTemplateFuncs(functionOptions, mciSettings.SuperUsers)
	if err != nil {
		fmt.Println("Failed to create template function map:", err)
		os.Exit(1)
	}

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, ui.WebRootPath, ui.Templates),
		DisableCache: !mciSettings.Ui.CacheTemplates,
		Funcs:        functions,
	})
	uis.InitPlugins()

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(webHome)))
	n.Use(ui.NewLogger())
	n.Use(negroni.HandlerFunc(ui.UserMiddleware(crowdManager)))
	n.UseHandler(router)

	n.Run(mciSettings.Ui.HttpListenAddr)
}

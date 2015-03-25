package main

import (
	"10gen.com/mci"
	"10gen.com/mci/auth"
	"10gen.com/mci/db"
	_ "10gen.com/mci/plugin/config"
	"10gen.com/mci/ui"
	"fmt"
	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/sessions"
	"net/http"
	"os"
	"path/filepath"
)

const UIPort = ":9090"

func main() {
	mciSettings := mci.MustConfig()
	if mciSettings.Ui.LogFile != "" {
		mci.SetLogger(mciSettings.Ui.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	home, err := mci.FindMCIHome()
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

	funcs, err := ui.MakeTemplateFuncs(ui.FuncOptions{
		filepath.Join(home, "public"), mciSettings.Ui.HelpUrl, true, router})
	if err != nil {
		fmt.Println("Failed to create template function map:", err)
		os.Exit(1)
	}

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, ui.WebRootPath, ui.Templates),
		DisableCache: true,
		Funcs:        funcs,
	})
	uis.InitPlugins()

	staticHome := filepath.Join(home, "public")

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(staticHome)))
	n.Use(ui.NewLogger())
	n.Use(negroni.HandlerFunc(ui.UserMiddleware(crowdManager)))
	n.UseHandler(router)

	n.Run(mciSettings.Ui.HttpListenAddr)
}

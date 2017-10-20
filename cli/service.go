package cli

import (
	htmlTemplate "html/template"
	"net/http"
	"path/filepath"
	textTemplate "text/template"
	"time"

	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/render"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/negroni"
)

var (
	// requestTimeout is the duration to wait until killing
	// active requests and stopping the server.
	requestTimeout = 10 * time.Second
)

type ServiceWebCommand struct {
	ConfigPath string `long:"conf" default:"/etc/mci_settings.yml" description:"path to the service configuration file"`
	APIService bool   `long:"api" description:"run the API service (default port: 8080)"`
	UIService  bool   `long:"ui" description:"run the UI service (default port: 9090)"`
}

func (c *ServiceWebCommand) Execute(_ []string) error {
	defer recovery.LogStackTraceAndExit("evergreen service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	if err := env.Configure(ctx, c.ConfigPath); err != nil {
		return errors.Wrap(err, "problem configuring application environment")
	}

	settings := env.Settings()

	apiHandler, err := getHandlerAPI(settings)
	if err != nil {
		return errors.WithStack(err)
	}

	uiHandler, err := getHandlerUI(settings)
	if err != nil {
		return errors.WithStack(err)
	}

	pprofHandler := service.GetHandlerPprof(settings)
	sender, err := settings.GetSender()
	if err != nil {
		return errors.WithStack(err)
	}
	defer sender.Close()

	if err = grip.SetSender(sender); err != nil {
		return errors.Wrap(err, "problem setting up logger")
	}

	grip.SetName("evergreen.service")
	grip.Warning(grip.SetDefaultLevel(level.Info))
	grip.Warning(grip.SetThreshold(level.Debug))

	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	go util.SystemInfoCollector(ctx)

	catcher := grip.NewBasicCatcher()
	apiWait := make(chan struct{})
	go func() {
		catcher.Add(service.RunGracefully(settings.Api.HttpListenAddr, requestTimeout, apiHandler))
		close(apiWait)
	}()

	uiWait := make(chan struct{})
	go func() {
		catcher.Add(service.RunGracefully(settings.Ui.HttpListenAddr, requestTimeout, uiHandler))
		close(uiWait)
	}()
	pprofWait := make(chan struct{})
	if settings.PprofPort != "" {
		go func() {
			catcher.Add(service.RunGracefully(settings.PprofPort, requestTimeout, pprofHandler))
			close(pprofWait)
		}()
	} else {
		close(pprofWait)
	}

	<-apiWait
	<-uiWait
	<-pprofWait

	return catcher.Resolve()
}

func getHandlerAPI(settings *evergreen.Settings) (http.Handler, error) {
	as, err := service.NewAPIServer(settings)
	if err != nil {
		err = errors.Wrap(err, "failed to create API server")
		return nil, err
	}

	router := as.NewRouter()

	n := negroni.New()
	n.Use(service.NewLogger())
	n.Use(negroni.HandlerFunc(service.UserMiddleware(as.UserManager)))
	n.UseHandler(router)
	return n, nil
}

func getHandlerUI(settings *evergreen.Settings) (http.Handler, error) {
	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	uis, err := service.NewUIServer(settings, home)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UI server")
	}

	router, err := uis.NewRouter()
	if err != nil {
		return nil, errors.Wrap(err, "problem creating router")
	}

	webHome := filepath.Join(home, "public")

	functionOptions := service.FuncOptions{
		WebHome:  webHome,
		HelpHome: settings.Ui.HelpUrl,
		IsProd:   !settings.IsNonProd,
		Router:   router,
	}

	functions, err := service.MakeTemplateFuncs(functionOptions, settings.SuperUsers)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create template function map")
	}

	htmlFunctions := htmlTemplate.FuncMap(functions)
	textFunctions := textTemplate.FuncMap(functions)

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, service.WebRootPath, service.Templates),
		DisableCache: !settings.Ui.CacheTemplates,
		HtmlFuncs:    htmlFunctions,
		TextFuncs:    textFunctions,
	})

	if err = uis.InitPlugins(); err != nil {
		return nil, errors.Wrap(err, "problem initializing plugins")
	}

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(webHome)))
	n.Use(service.NewLogger())
	n.Use(negroni.HandlerFunc(service.UserMiddleware(uis.UserManager)))
	n.UseHandler(router)

	return n, nil
}

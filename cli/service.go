package cli

import (
	"fmt"
	htmlTemplate "html/template"
	"net/http"
	"path/filepath"
	textTemplate "text/template"
	"time"

	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
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
	ctx, cancel := context.WithCancel(context.Background())

	env := evergreen.GetEnvironment()
	grip.CatchEmergencyFatal(errors.Wrap(env.Configure(ctx, c.ConfigPath), "problem configuring application environment"))

	settings := env.Settings()
	sender, err := settings.GetSender()
	grip.CatchEmergencyFatal(err)
	grip.CatchEmergencyFatal(grip.SetSender(sender))

	defer sender.Close()
	defer recovery.LogStackTraceAndExit("evergreen service")
	defer cancel()

	grip.SetName("evergreen.service")
	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), 15*time.Second, time.Now(), true, func(queue amboy.Queue) error {
		return queue.Put(units.NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%d", time.Now().Unix())))
	})

	router := mux.NewRouter()

	apiHandler, err := getHandlerAPI(settings, router)
	if err != nil {
		return errors.WithStack(err)
	}

	uiHandler, err := getHandlerUI(settings, router)
	if err != nil {
		return errors.WithStack(err)
	}

	pprofHandler := service.GetHandlerPprof(settings)

	catcher := grip.NewBasicCatcher()
	apiWait := make(chan struct{})
	go func() {
		catcher.Add(service.RunGracefully(settings.Api.HttpListenAddr, requestTimeout, apiHandler))
		close(apiWait)
	}()

	uiWait := make(chan struct{})
	go func() {
		if settings.Ui.CsrfKey != "" {
			errorHandler := csrf.ErrorHandler(http.HandlerFunc(service.ForbiddenHandler))
			uiHandler = csrf.Protect([]byte(settings.Ui.CsrfKey), errorHandler)(uiHandler)
		}
		catcher.Add(service.RunGracefully(settings.Ui.HttpListenAddr, requestTimeout, uiHandler))
		close(uiWait)
	}()

	pprofWait := make(chan struct{})
	go func() {
		if settings.PprofPort != "" {
			catcher.Add(service.RunGracefully(settings.PprofPort, requestTimeout, pprofHandler))
		}
		close(pprofWait)
	}()

	<-apiWait
	<-uiWait
	<-pprofWait

	return catcher.Resolve()
}

func getHandlerAPI(settings *evergreen.Settings, router *mux.Router) (http.Handler, error) {
	as, err := service.NewAPIServer(settings)
	if err != nil {
		err = errors.Wrap(err, "failed to create API server")
		return nil, err
	}

	as.AttachRoutes(router)

	n := negroni.New()
	n.Use(service.NewLogger())
	n.Use(negroni.HandlerFunc(service.UserMiddleware(as.UserManager)))
	n.UseHandler(router)
	return n, nil
}

func getHandlerUI(settings *evergreen.Settings, router *mux.Router) (http.Handler, error) {
	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	uis, err := service.NewUIServer(settings, home)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UI server")
	}

	err = uis.AttachRoutes(router)
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

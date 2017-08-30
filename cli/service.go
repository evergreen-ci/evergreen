package cli

import (
	htmlTemplate "html/template"
	"net/http"
	"path/filepath"
	textTemplate "text/template"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/render"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
	if c.APIService == c.UIService {
		return errors.New("Must specify exactly one of --api or --ui")
	}
	settings, err := evergreen.NewSettings(c.ConfigPath)
	if err != nil {
		return errors.Wrap(err, "problem getting settings")
	}

	if err = settings.Validate(); err != nil {
		return errors.Wrap(err, "problem validating settings")
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	var (
		loggerName  string
		logFileName string
		handler     http.Handler
		listenAddr  string
		sender      send.Sender
	)

	if c.APIService {
		loggerName = "evg-api-server"
		logFileName = settings.Api.LogFile
		handler, err = getHandlerAPI(settings)
		listenAddr = settings.Api.HttpListenAddr
	} else if c.UIService {
		loggerName = "evg-ui-server"
		logFileName = settings.Ui.LogFile
		handler, err = getHandlerUI(settings)
		listenAddr = settings.Ui.HttpListenAddr
	}

	if err != nil {
		return errors.Wrap(err, "problem setting up service")
	}

	sender, err = settings.GetSender(logFileName)
	if err != nil {
		return err
	}
	defer sender.Close()

	if err := grip.SetSender(sender); err != nil {
		return errors.Wrap(err, "problem setting up logger")
	}

	defer util.RecoverAndLogStackTrace()

	evergreen.SetLegacyLogger()
	grip.SetName(loggerName)
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)

	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go evergreen.SystemInfoCollector(ctx)

	err = errors.Wrap(service.RunGracefully(listenAddr, requestTimeout, handler),
		"problem running service")
	grip.Alert(err)
	return err
}

func getHandlerAPI(settings *evergreen.Settings) (http.Handler, error) {
	as, err := service.NewAPIServer(settings)
	if err != nil {
		err = errors.Wrap(err, "failed to create API server")
		return nil, err
	}

	handler, err := as.Handler()
	if err != nil {
		err = errors.Wrap(err, "failed to get API route handlers")
		return nil, err
	}

	return handler, nil
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

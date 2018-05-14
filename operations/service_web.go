package operations

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/sometimes"
	nrgorilla "github.com/newrelic/go-agent/_integrations/nrgorilla/v1"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/urfave/negroni"
)

func startWebService() cli.Command {
	return cli.Command{
		Name:  "web",
		Usage: "start web services for API and UI",
		Flags: mergeFlagSlices(serviceConfigFlags(), addDbSettingsFlags()),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)
			db := parseDB(c)
			ctx, cancel := context.WithCancel(context.Background())

			env := evergreen.GetEnvironment()
			grip.CatchEmergencyFatal(errors.Wrap(env.Configure(ctx, confPath, db), "problem configuring application environment"))

			settings := env.Settings()
			sender, err := settings.GetSender(env)
			grip.CatchEmergencyFatal(err)
			grip.CatchEmergencyFatal(grip.SetSender(sender))
			queue := env.RemoteQueue()

			defer sender.Close()
			defer recovery.LogStackTraceAndExit("evergreen service")
			defer cancel()

			grip.SetName("evergreen.service")
			grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

			startWebTierBackgroundJobs(ctx, env)

			router := mux.NewRouter()

			apiHandler, err := getHandlerAPI(settings, queue, router)
			if err != nil {
				return errors.WithStack(err)
			}
			apiServer := service.GetServer(settings.Api.HttpListenAddr, apiHandler)

			uiHandler, err := getHandlerUI(settings, queue, router)
			if err != nil {
				return errors.WithStack(err)
			}
			if settings.Ui.CsrfKey != "" {
				errorHandler := csrf.ErrorHandler(http.HandlerFunc(service.ForbiddenHandler))
				uiHandler = csrf.Protect([]byte(settings.Ui.CsrfKey), errorHandler)(uiHandler)
			}
			uiServer := service.GetServer(settings.Ui.HttpListenAddr, uiHandler)

			newRelic, err := settings.NewRelic.SetUp()
			if newRelic == nil || err != nil {
				grip.Debug(message.WrapError(err, message.Fields{
					"message": "skipping new relic setup",
				}))
			} else {
				grip.Info(message.Fields{
					"message":          "successfully set up new relic",
					"application_name": settings.NewRelic.ApplicationName,
				})
				nrgorilla.InstrumentRoutes(router, newRelic)
			}

			catcher := grip.NewBasicCatcher()
			apiWait := make(chan struct{})
			go func() {
				defer recovery.LogStackTraceAndContinue("api server")
				catcher.Add(apiServer.ListenAndServe())
				close(apiWait)
			}()

			uiWait := make(chan struct{})
			go func() {
				defer recovery.LogStackTraceAndContinue("ui server")
				catcher.Add(uiServer.ListenAndServe())
				close(uiWait)
			}()

			pprofWait := make(chan struct{})
			pprofServer := service.GetServer(settings.PprofPort, service.GetHandlerPprof(settings))
			go func() {
				defer recovery.LogStackTraceAndContinue("proff server")

				if settings.PprofPort != "" {
					catcher.Add(pprofServer.ListenAndServe())
				}

				close(pprofWait)
			}()

			gracefulWait := make(chan struct{})
			go gracefulShutdownForSIGTERM(ctx, []*http.Server{pprofServer, uiServer, apiServer}, gracefulWait, catcher)

			<-apiWait
			<-uiWait
			<-pprofWait

			grip.Notice("waiting for web services to terminate gracefully")
			<-gracefulWait

			return catcher.Resolve()
		},
	}
}

func startWebTierBackgroundJobs(ctx context.Context, env evergreen.Environment) {
	startSysInfoCollectors(ctx, env, 15*time.Second, amboy.QueueOperationConfig{
		ContinueOnError: true,
		LogErrors:       false,
		DebugLogging:    false,
	})

	opts := amboy.QueueOperationConfig{
		ContinueOnError: false,
		LogErrors:       true,
		DebugLogging:    false,
	}

	const amboyStatsInterval = time.Minute

	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), amboyStatsInterval, time.Now(), opts, func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			grip.Alert(message.WrapError(err, message.Fields{
				"message":       "problem fetching service flags",
				"operation":     "background stats",
				"interval_secs": amboyStatsInterval.Seconds(),
			}))
			return err
		}

		if flags.BackgroundStatsDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "background stats collection disabled",
				"impact":  "amboy stats disabled",
				"mode":    "degraded",
			})
			return nil
		}

		return queue.Put(units.NewLocalAmboyStatsCollector(env, fmt.Sprintf("amboy-local-stats-%d", time.Now().Unix())))
	})
}

func gracefulShutdownForSIGTERM(ctx context.Context, servers []*http.Server, wait chan struct{}, catcher grip.Catcher) {
	defer recovery.LogStackTraceAndContinue("graceful shutdown")
	sigChan := make(chan os.Signal, len(servers))
	signal.Notify(sigChan, syscall.SIGTERM)

	<-sigChan
	waiters := make([]chan struct{}, 0)

	grip.Info("received SIGTERM, terminating web service")
	for _, s := range servers {
		if s == nil {
			continue
		}

		waiter := make(chan struct{})
		go func(server *http.Server) {
			defer recovery.LogStackTraceAndContinue("server shutdown")

			catcher.Add(server.Shutdown(ctx))
			close(waiter)
		}(s)
		waiters = append(waiters, waiter)
	}

	for _, waiter := range waiters {
		if waiter == nil {
			continue
		}

		<-waiter
	}

	close(wait)
}

func getHandlerAPI(settings *evergreen.Settings, queue amboy.Queue, router *mux.Router) (http.Handler, error) {
	as, err := service.NewAPIServer(settings, queue)
	if err != nil {
		err = errors.Wrap(err, "failed to create API server")
		return nil, err
	}

	as.AttachRoutes(router)

	n := negroni.New()
	n.Use(service.NewRecoveryLogger())
	n.Use(negroni.HandlerFunc(service.UserMiddleware(as.UserManager)))
	n.UseHandler(router)
	return n, nil
}

func getHandlerUI(settings *evergreen.Settings, queue amboy.Queue, router *mux.Router) (http.Handler, error) {
	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	webHome := filepath.Join(home, "public")
	functionOptions := service.TemplateFunctionOptions{
		WebHome:  webHome,
		HelpHome: settings.Ui.HelpUrl,
		IsProd:   !settings.IsNonProd,
		Router:   router,
	}

	uis, err := service.NewUIServer(settings, queue, home, functionOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UI server")
	}

	err = uis.AttachRoutes(router)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating router")
	}

	if err = uis.InitPlugins(); err != nil {
		return nil, errors.Wrap(err, "problem initializing plugins")
	}

	n := negroni.New()
	n.Use(negroni.NewStatic(http.Dir(webHome)))
	n.Use(service.NewRecoveryLogger())
	n.Use(negroni.HandlerFunc(service.UserMiddleware(uis.UserManager)))
	n.UseHandler(router)

	return n, nil
}

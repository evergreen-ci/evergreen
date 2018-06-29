package operations

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/gorilla/csrf"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
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

			defer cancel()
			defer sender.Close()
			defer recovery.LogStackTraceAndExit("evergreen service")

			grip.SetName("evergreen.service")
			grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

			startSystemCronJobs(ctx, env)

			var (
				apiServer *http.Server
				uiServer  *http.Server
			)

			pprof, err := service.GetHandlerPprof(settings)
			if err != nil {
				return errors.WithStack(err)
			}

			serviceHandler, err := getServiceRouter(settings, queue)
			if err != nil {
				return errors.WithStack(err)
			}
			apiServer = service.GetServer(settings.Api.HttpListenAddr, serviceHandler)

			if settings.Ui.CsrfKey != "" {
				errorHandler := csrf.ErrorHandler(http.HandlerFunc(service.ForbiddenHandler))
				uiHandler := csrf.Protect([]byte(settings.Ui.CsrfKey), errorHandler)(serviceHandler)
				uiServer = service.GetServer(settings.Ui.HttpListenAddr, uiHandler)
			} else {
				uiServer = service.GetServer(settings.Ui.HttpListenAddr, serviceHandler)
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

			pprofServer := service.GetServer(settings.PprofPort, pprof)
			pprofWait := make(chan struct{})
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

			grip.Notice("waiting for background tasks to finish")
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			catcher.Add(env.Close(ctx))

			return catcher.Resolve()
		},
	}
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

func getServiceRouter(settings *evergreen.Settings, queue amboy.Queue) (http.Handler, error) {
	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	functionOptions := service.TemplateFunctionOptions{
		WebHome:  filepath.Join(home, "public"),
		HelpHome: settings.Ui.HelpUrl,
		IsProd:   !settings.IsNonProd,
	}

	uis, err := service.NewUIServer(settings, queue, home, functionOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UI server")
	}

	as, err := service.NewAPIServer(settings, queue)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create API server")
	}

	return service.GetRouter(as, uis)
}

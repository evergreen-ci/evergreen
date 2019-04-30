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
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/reporting"
	"github.com/mongodb/amboy/rest"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
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

			env, err := evergreen.NewEnvironment(ctx, confPath, db)
			grip.EmergencyFatal(errors.Wrap(err, "problem configuring application environment"))
			evergreen.SetEnvironment(env)
			if c.Bool(overwriteConfFlagName) {
				grip.EmergencyFatal(errors.Wrap(env.SaveConfig(), "problem saving config"))
			}
			grip.EmergencyFatal(errors.Wrap(env.RemoteQueue().Start(ctx), "problem starting remote queue"))

			settings := env.Settings()
			sender, err := settings.GetSender(env)
			grip.EmergencyFatal(err)
			grip.EmergencyFatal(grip.SetSender(sender))
			queue := env.RemoteQueue()
			remoteQueueGroup := env.RemoteQueueGroup()

			defer cancel()
			defer sender.Close()
			defer recovery.LogStackTraceAndExit("evergreen service")

			grip.SetName("evergreen.service")
			grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

			grip.EmergencyFatal(errors.Wrap(startSystemCronJobs(ctx, env), "problem starting background work"))

			var (
				apiServer *http.Server
				uiServer  *http.Server
			)

			serviceHandler, err := getServiceRouter(settings, queue, remoteQueueGroup)
			if err != nil {
				return errors.WithStack(err)
			}
			adminHandler, err := getAdminService(ctx, env, settings)
			if err != nil {
				return errors.WithStack(err)
			}

			apiServer = service.GetServer(settings.Api.HttpListenAddr, serviceHandler)
			uiServer = service.GetServer(settings.Ui.HttpListenAddr, serviceHandler)

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

			adminServer := service.GetServer(settings.PprofPort, adminHandler)
			adminWait := make(chan struct{})
			go func() {
				defer recovery.LogStackTraceAndContinue("admin server")

				if settings.PprofPort != "" {
					catcher.Add(adminServer.ListenAndServe())
				}

				close(adminWait)
			}()

			gracefulWait := make(chan struct{})
			go gracefulShutdownForSIGTERM(ctx, []*http.Server{uiServer, apiServer, adminServer}, gracefulWait, catcher)

			<-apiWait
			<-uiWait
			<-adminWait

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

func getServiceRouter(settings *evergreen.Settings, queue amboy.Queue, remoteQueueGroup amboy.QueueGroup) (http.Handler, error) {
	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	functionOptions := service.TemplateFunctionOptions{
		WebHome:  filepath.Join(home, "public"),
		HelpHome: settings.Ui.HelpUrl,
	}

	uis, err := service.NewUIServer(settings, queue, home, functionOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UI server")
	}

	as, err := service.NewAPIServer(settings, queue, remoteQueueGroup)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create API server")
	}

	return service.GetRouter(as, uis)
}

func getAdminService(ctx context.Context, env evergreen.Environment, settings *evergreen.Settings) (http.Handler, error) {
	localPool, ok := env.LocalQueue().Runner().(amboy.AbortableRunner)
	if !ok {
		return nil, errors.New("local pool is not configured with an abortable pool")
	}
	remotePool, ok := env.RemoteQueue().Runner().(amboy.AbortableRunner)
	if !ok {
		return nil, errors.New("remote pool is not configured with an abortable pool")
	}

	opts := queue.DefaultMongoDBOptions()
	opts.URI = settings.Database.Url
	opts.DB = settings.Amboy.DB
	opts.Priority = true

	app := gimlet.NewApp()
	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	apps := []*gimlet.APIApp{app}

	localAbort := rest.NewManagementService(localPool).App()
	localAbort.SetPrefix("/amboy/local/pool")

	remoteAbort := rest.NewManagementService(remotePool).App()
	remoteAbort.SetPrefix("/amboy/remote/pool")

	localReporting := rest.NewReportingService(reporting.NewQueueReporter(env.LocalQueue())).App()
	localReporting.SetPrefix("/amboy/local/reporting")

	apps = append(apps, localAbort, remoteAbort, localReporting)
	if evergreen.EnableAmboyRemoteReporting {
		remoteReporter, err := reporting.MakeDBQueueState(ctx, settings.Amboy.Name, opts, env.Client())
		if err != nil {
			return nil, errors.Wrap(err, "problem building queue reporter")
		}

		remoteReporting := rest.NewReportingService(remoteReporter).App()
		remoteReporting.SetPrefix("/amboy/remote/reporting")
		apps = append(apps, remoteReporting)
	}

	jpm := jasper.NewManagerService(env.JasperManager())
	jpmapp := jpm.App(ctx)
	jpmapp.SetPrefix("jasper")
	jpm.SetDisableCachePruning(true)
	apps = append(apps, jpmapp, gimlet.GetPProfApp())

	handler, err := gimlet.MergeApplications(apps...)
	if err != nil {
		return nil, errors.Wrap(err, "problem assembling handler")
	}

	return handler, nil
}

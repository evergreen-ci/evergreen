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
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/management"
	"github.com/mongodb/amboy/rest"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper/remote"
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
			grip.EmergencyFatal(errors.Wrap(err, "configuring application environment"))
			evergreen.SetEnvironment(env)
			if c.Bool(overwriteConfFlagName) {
				grip.EmergencyFatal(errors.Wrap(env.SaveConfig(ctx), "saving config"))
			}
			grip.EmergencyFatal(errors.Wrap(env.RemoteQueue().Start(ctx), "starting remote queue"))

			settings := env.Settings()
			sender, err := settings.GetSender(ctx, env)
			grip.EmergencyFatal(err)
			grip.EmergencyFatal(grip.SetSender(sender))
			queue := env.RemoteQueue()
			remoteQueueGroup := env.RemoteQueueGroup()

			// Create the user manager before setting up job queues to allow
			// background reauthorization jobs to start.
			if err = setupUserManager(env, settings); err != nil {
				return errors.Wrap(err, "setting up user manager")
			}

			defer cancel()
			defer sender.Close()
			defer recovery.LogStackTraceAndExit("evergreen service")

			grip.SetName("evergreen.service")
			grip.Notice(message.Fields{
				"agent":   evergreen.AgentVersion,
				"cli":     evergreen.ClientVersion,
				"build":   evergreen.BuildRevision,
				"process": grip.Name(),
			})

			grip.EmergencyFatal(errors.Wrap(startSystemCronJobs(ctx, env), "starting background work"))

			var (
				apiServer *http.Server
				uiServer  *http.Server
			)

			serviceHandler, err := getServiceRouter(env, queue, remoteQueueGroup)
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
				defer recovery.LogStackTraceAndContinue("API server")
				catcher.Add(apiServer.ListenAndServe())
				close(apiWait)
			}()

			uiWait := make(chan struct{})
			go func() {
				defer recovery.LogStackTraceAndContinue("UI server")
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
			go gracefulShutdownForSIGTERM(ctx, []*http.Server{uiServer, apiServer, adminServer}, gracefulWait, catcher, env)

			<-apiWait
			<-uiWait
			<-adminWait

			grip.Notice("waiting for web services to terminate gracefully")
			<-gracefulWait

			grip.Notice("waiting for background tasks to finish")
			ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			catcher.Add(env.Close(ctx))

			return catcher.Resolve()
		},
	}
}

func gracefulShutdownForSIGTERM(ctx context.Context, servers []*http.Server, wait chan struct{}, catcher grip.Catcher, env evergreen.Environment) {
	defer recovery.LogStackTraceAndContinue("graceful shutdown")
	sigChan := make(chan os.Signal, len(servers))
	signal.Notify(sigChan, syscall.SIGTERM)

	<-sigChan
	// we got the signal, so modify the status endpoint and wait (EVG-12993)
	// This allows the load balancer to detect shutoffs and route traffic with no downtime
	env.SetShutdown()

	time.Sleep(time.Duration(env.Settings().ShutdownWaitSeconds) * time.Second)
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

func getServiceRouter(env evergreen.Environment, queue amboy.Queue, remoteQueueGroup amboy.QueueGroup) (http.Handler, error) {
	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	functionOptions := service.TemplateFunctionOptions{
		WebHome:  filepath.Join(home, "public"),
		HelpHome: env.Settings().Ui.HelpUrl,
	}

	uis, err := service.NewUIServer(env, queue, home, functionOptions)
	if err != nil {
		return nil, errors.Wrap(err, "creating UI server")
	}

	as, err := service.NewAPIServer(env, queue)
	if err != nil {
		return nil, errors.Wrap(err, "creating API server")
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

	app := gimlet.NewApp()
	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	apps := []*gimlet.APIApp{app}

	localAbort := rest.NewAbortablePoolManagementService(localPool).App()
	localAbort.SetPrefix("/amboy/local/pool")

	remoteAbort := rest.NewAbortablePoolManagementService(remotePool).App()
	remoteAbort.SetPrefix("/amboy/remote/pool")

	groupAbort := rest.NewManagementGroupService(env.RemoteQueueGroup()).App()
	groupAbort.SetPrefix("/amboy/group/pool")

	localManagement := rest.NewManagementService(management.NewQueueManager(env.LocalQueue())).App()
	localManagement.SetPrefix("/amboy/local/management")

	apps = append(apps, localAbort, remoteAbort, groupAbort, localManagement)

	jpm := remote.NewRESTService(env.JasperManager())
	jpmapp := jpm.App(ctx)
	jpmapp.SetPrefix("jasper")
	jpm.SetDisableCachePruning(true)
	apps = append(apps, jpmapp, gimlet.GetPProfApp())

	handler, err := gimlet.MergeApplications(apps...)
	if err != nil {
		return nil, errors.Wrap(err, "merging Gimlet applications")
	}

	return handler, nil
}

// setupUserManager sets up the global user authentication manager.
func setupUserManager(env evergreen.Environment, settings *evergreen.Settings) error {
	um, info, err := auth.LoadUserManager(settings)
	if err != nil {
		return errors.Wrap(err, "loading user manager")
	}
	env.SetUserManager(um)
	env.SetUserManagerInfo(info)
	return nil
}

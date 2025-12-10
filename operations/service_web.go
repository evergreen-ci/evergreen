package operations

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/management"
	"github.com/mongodb/amboy/rest"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/automaxprocs/maxprocs"
)

func startWebService() cli.Command {
	return cli.Command{
		Name:  "web",
		Usage: "start web services for API and UI",
		Flags: mergeFlagSlices(
			serviceConfigFlags(),
			addDbSettingsFlags(),
			[]cli.Flag{cli.StringFlag{
				Name:   traceEndpointFlagName,
				Usage:  "Endpoint to send startup traces to",
				EnvVar: evergreen.TraceEndpoint}},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())

			// When running within a container, detect the number of CPUs available to the container
			// and set GOMAXPROCS. This noops when running outside of a container.
			_, err := maxprocs.Set()
			grip.EmergencyFatal(errors.Wrap(err, "setting max procs"))

			var tp trace.TracerProvider
			sdkTracerProvider, err := startupTracerProvider(ctx, c.String(traceEndpointFlagName))
			if err != nil || sdkTracerProvider == nil {
				grip.Error(message.WrapError(err, "initializing startup tracer provider"))
				tp = noop.NewTracerProvider()
			} else {
				tp = sdkTracerProvider
			}

			tracer := tp.Tracer("github.com/evergreen-ci/evergreen/operations")
			ctx, startServiceSpan := tracer.Start(ctx, "StartService")
			// This is only in case of an error.
			defer startServiceSpan.End()

			confPath := c.String(confFlagName)
			versionID := c.String(versionIDFlagName)
			clientS3Bucket := c.String(clientS3BucketFlagName)
			db := parseDB(c)
			env, err := evergreen.NewEnvironment(ctx, confPath, versionID, clientS3Bucket, db, tp)
			grip.EmergencyFatal(errors.Wrap(err, "configuring application environment"))

			if c.Bool(testingEnvFlagName) {
				// If running in a testing environment (e.g. local Evergreen),
				// use a fake implementation of Parameter Store since testing
				// environments won't have access to a real Parameter Store
				// instance.
				fakeparameter.ExecutionEnvironmentType = "test"

				opts := parameterstore.ParameterManagerOptions{
					PathPrefix:     env.Settings().ParameterStore.Prefix,
					CachingEnabled: true,
					SSMClient:      fakeparameter.NewFakeSSMClient(),
					DB:             env.DB(),
				}
				pm, err := parameterstore.NewParameterManager(ctx, opts)
				if err != nil {
					return errors.Wrap(err, "creating parameter manager")
				}
				env.SetParameterManager(pm)
			}

			evergreen.SetEnvironment(env)
			if c.Bool(overwriteConfFlagName) {
				grip.EmergencyFatal(errors.Wrap(env.SaveConfig(ctx), "saving config"))
			}

			// Remove the span from the remoteQueueCtx since the queue caches the ctx.
			remoteQueueCtx := trace.ContextWithSpan(ctx, nil)
			grip.EmergencyFatal(errors.Wrap(env.RemoteQueue().Start(remoteQueueCtx), "starting remote queue"))

			settings := env.Settings()
			// Remove the span from the senderCtx since the sender caches the ctx.
			senderCtx := trace.ContextWithSpan(ctx, nil)
			sender, err := settings.GetSender(senderCtx, env)
			grip.EmergencyFatal(err)
			grip.EmergencyFatal(grip.SetSender(sender))
			queue := env.RemoteQueue()

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

			grip.EmergencyFatal(errors.Wrap(startSystemCronJobs(ctx, env, tracer), "starting background work"))

			var (
				apiServer *http.Server
				uiServer  *http.Server
			)

			serviceHandler, err := getServiceRouter(ctx, env, queue, tracer)
			if err != nil {
				return errors.WithStack(err)
			}
			adminHandler, err := getAdminService(ctx, env, tracer)
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

			// This end span is the correct time to end the span for the service startup.
			startServiceSpan.End()
			if sdkTracerProvider != nil {
				catcher.Add(sdkTracerProvider.Shutdown(ctx))
			}

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

func startupTracerProvider(ctx context.Context, traceEndpoint string) (*sdktrace.TracerProvider, error) {
	if traceEndpoint == "" {
		return nil, nil
	}

	resource, err := resource.New(ctx,
		resource.WithHost(),
		resource.WithAttributes(semconv.ServiceName("evergreen")),
	)
	if err != nil {
		return nil, errors.Wrap(err, "making otel resource")
	}

	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpoint(traceEndpoint))
	opts = append(opts, otlptracegrpc.WithInsecure())

	client := otlptracegrpc.NewClient(opts...)
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, errors.Wrap(err, "initializing otel exporter")
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource),
	)
	tp.RegisterSpanProcessor(utility.NewAttributeSpanProcessor())
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		grip.Error(errors.Wrap(err, "otel error"))
	}))

	return tp, nil
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

func getServiceRouter(ctx context.Context, env evergreen.Environment, queue amboy.Queue, tracer trace.Tracer) (http.Handler, error) {
	_, span := tracer.Start(ctx, "GetServiceRouter")
	defer span.End()

	home := evergreen.FindEvergreenHome()
	if home == "" {
		return nil, errors.New("EVGHOME environment variable must be set to run UI server")
	}

	uis, err := service.NewUIServer(env, queue, home)
	if err != nil {
		return nil, errors.Wrap(err, "creating UI server")
	}

	as, err := service.NewAPIServer(env, queue)
	if err != nil {
		return nil, errors.Wrap(err, "creating API server")
	}

	return service.GetRouter(ctx, as, uis)
}

func getAdminService(ctx context.Context, env evergreen.Environment, tracer trace.Tracer) (http.Handler, error) {
	ctx, span := tracer.Start(ctx, "GetAdminService")
	defer span.End()

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
	// Remove the span from the ctx since the App caches the ctx.
	ctx = trace.ContextWithSpan(ctx, nil)
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

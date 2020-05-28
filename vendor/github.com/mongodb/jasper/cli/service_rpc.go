package cli

import (
	"context"
	"fmt"
	"net"

	"github.com/evergreen-ci/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	rpcHostEnvVar  = "JASPER_RPC_HOST"
	rpcPortEnvVar  = "JASPER_RPC_PORT"
	defaultRPCPort = 2486
)

func serviceCommandRPC(cmd string, operation serviceOperation) cli.Command {
	return cli.Command{
		Name:  RPCService,
		Usage: fmt.Sprintf("%s an RPC service", cmd),
		Flags: append(serviceFlags(),
			cli.StringFlag{
				Name:   hostFlagName,
				EnvVar: rpcHostEnvVar,
				Usage:  "the host running the RPC service",
				Value:  defaultLocalHostName,
			},
			cli.IntFlag{
				Name:   portFlagName,
				EnvVar: rpcPortEnvVar,
				Usage:  "the port running the RPC service",
				Value:  defaultRPCPort,
			},
			cli.StringFlag{
				Name:  credsFilePathFlagName,
				Usage: "the path to the file containing the RPC service credentials",
			},
		),
		Before: mergeBeforeFuncs(
			validatePort(portFlagName),
			validateLogLevel(logLevelFlagName),
			validateLimits(limitNumFilesFlagName, limitNumProcsFlagName, limitLockedMemoryFlagName, limitVirtualMemoryFlagName),
		),
		Action: func(c *cli.Context) error {
			manager, err := jasper.NewSynchronizedManager(false)
			if err != nil {
				return errors.Wrap(err, "error creating RPC manager")
			}

			daemon := newRPCDaemon(c.String(hostFlagName), c.Int(portFlagName), manager, c.String(credsFilePathFlagName), makeLogger(c))

			config := serviceConfig(RPCService, c, buildRunCommand(c, RPCService))

			if err := operation(daemon, config); !c.Bool(quietFlagName) {
				return err
			}
			return nil
		},
	}
}

type rpcDaemon struct {
	Host          string
	Port          int
	CredsFilePath string
	Manager       jasper.Manager
	Logger        *options.Logger

	exit chan struct{}
}

func newRPCDaemon(host string, port int, manager jasper.Manager, credsFilePath string, logger *options.Logger) *rpcDaemon {
	return &rpcDaemon{
		Host:          host,
		Port:          port,
		CredsFilePath: credsFilePath,
		Manager:       manager,
		Logger:        logger,
	}
}

func (d *rpcDaemon) Start(s service.Service) error {
	if d.Logger != nil {
		if err := setupLogger(d.Logger); err != nil {
			return errors.Wrap(err, "")
		}
	}

	d.exit = make(chan struct{})
	if d.Manager == nil {
		var err error
		if d.Manager, err = jasper.NewSynchronizedManager(false); err != nil {
			return errors.Wrap(err, "failed to construct RPC manager")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go handleDaemonSignals(ctx, cancel, d.exit)

	go func(ctx context.Context, d *rpcDaemon) {
		defer recovery.LogStackTraceAndContinue("rpc service")
		grip.Error(errors.Wrap(d.run(ctx), "error running RPC service"))
	}(ctx, d)

	return nil
}

func (d *rpcDaemon) Stop(s service.Service) error {
	close(d.exit)
	return nil
}

func (d *rpcDaemon) run(ctx context.Context) error {
	return errors.Wrap(runServices(ctx, d.newService), "error running RPC service")
}

func (d *rpcDaemon) newService(ctx context.Context) (util.CloseFunc, error) {
	if d.Manager == nil {
		return nil, errors.New("manager is not set on RPC service")
	}

	grip.Infof("starting RPC service at '%s:%d'", d.Host, d.Port)

	return newRPCService(ctx, d.Host, d.Port, d.Manager, d.CredsFilePath)
}

// newRPCService creates an RPC service around the manager serving requests on
// the host and port.
func newRPCService(ctx context.Context, host string, port int, manager jasper.Manager, credsFilePath string) (util.CloseFunc, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve RPC address")
	}

	closeService, err := remote.StartRPCServiceWithFile(ctx, manager, addr, credsFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "error starting RPC service")
	}
	return closeService, nil
}

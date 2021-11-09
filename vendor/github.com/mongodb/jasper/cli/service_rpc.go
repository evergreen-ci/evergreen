package cli

import (
	"context"
	"fmt"
	"net"

	"github.com/evergreen-ci/baobab"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
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

			opts := daemonOptions{
				host:             c.String(hostFlagName),
				port:             c.Int(portFlagName),
				manager:          manager,
				logger:           makeLogger(c),
				preconditionCmds: c.StringSlice(preconditionCmdsFlagName),
			}
			daemon := newRPCDaemon(opts, c.String(credsFilePathFlagName))

			config := serviceConfig(RPCService, c, buildServiceRunCommand(c, RPCService))

			if err := operation(daemon, config); !c.Bool(quietFlagName) {
				return err
			}
			return nil
		},
	}
}

type rpcDaemon struct {
	baseDaemon
	credsFilePath string
}

func newRPCDaemon(opts daemonOptions, credsFilePath string) *rpcDaemon {
	return &rpcDaemon{
		baseDaemon:    newBaseDaemon(opts),
		credsFilePath: credsFilePath,
	}
}

func (d *rpcDaemon) Start(s baobab.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	if err := d.setup(ctx, cancel); err != nil {
		return errors.Wrap(err, "setup")
	}

	go func(ctx context.Context, d *rpcDaemon) {
		defer recovery.LogStackTraceAndContinue("RPC service")
		grip.Error(errors.Wrap(d.run(ctx), "error running RPC service"))
	}(ctx, d)

	return nil
}

func (d *rpcDaemon) Stop(s baobab.Service) error {
	close(d.exit)
	return nil
}

func (d *rpcDaemon) run(ctx context.Context) error {
	return errors.Wrap(runServices(ctx, d.newService), "error running RPC service")
}

func (d *rpcDaemon) newService(ctx context.Context) (util.CloseFunc, error) {
	if d.manager == nil {
		return nil, errors.New("manager is not set on RPC service")
	}

	grip.Infof("starting RPC service at '%s:%d'", d.host, d.port)

	return newRPCService(ctx, d.host, d.port, d.manager, d.credsFilePath)
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

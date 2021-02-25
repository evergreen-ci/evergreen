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
	wireHostEnvVar  = "JASPER_WIRE_HOST"
	wirePortEnvVar  = "JASPER_WIRE_PORT"
	defaultWirePort = 2488
)

func serviceCommandWire(cmd string, operation serviceOperation) cli.Command {
	return cli.Command{
		Name:  WireService,
		Usage: fmt.Sprintf("%s a MongoDB wire protocol service", cmd),
		Flags: append(serviceFlags(),
			cli.StringFlag{
				Name:   hostFlagName,
				EnvVar: wireHostEnvVar,
				Usage:  "the host running the wire service",
				Value:  defaultLocalHostName,
			},
			cli.IntFlag{
				Name:   portFlagName,
				EnvVar: wirePortEnvVar,
				Usage:  "the port running the wire service",
				Value:  defaultWirePort,
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
				return errors.Wrap(err, "error creating wire manager")
			}

			opts := daemonOptions{
				host:             c.String(hostFlagName),
				port:             c.Int(portFlagName),
				manager:          manager,
				logger:           makeLogger(c),
				preconditionCmds: c.StringSlice(preconditionCmdsFlagName),
			}
			daemon := newWireDaemon(opts)

			config := serviceConfig(WireService, c, buildServiceRunCommand(c, WireService))

			if err := operation(daemon, config); !c.Bool(quietFlagName) {
				return err
			}
			return nil
		},
	}
}

type wireDaemon struct {
	baseDaemon
}

func newWireDaemon(opts daemonOptions) *wireDaemon {
	return &wireDaemon{newBaseDaemon(opts)}
}

func (d *wireDaemon) Start(s baobab.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	if err := d.setup(ctx, cancel); err != nil {
		return errors.Wrap(err, "setup")
	}

	go func(ctx context.Context, d *wireDaemon) {
		defer recovery.LogStackTraceAndContinue("wire service")
		grip.Error(errors.Wrap(d.run(ctx), "error running wire service"))
	}(ctx, d)

	return nil
}

func (d *wireDaemon) Stop(s baobab.Service) error {
	close(d.exit)
	return nil
}

func (d *wireDaemon) run(ctx context.Context) error {
	return errors.Wrap(runServices(ctx, d.newService), "error running wire service")
}

func (d *wireDaemon) newService(ctx context.Context) (util.CloseFunc, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", d.host, d.port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve wire address")
	}

	closeService, err := remote.StartMDBService(ctx, d.manager, addr)
	if err != nil {
		return nil, errors.Wrap(err, "error starting wire service")
	}
	return closeService, nil
}

package cli

import (
	"context"
	"fmt"
	"net"

	"github.com/kardianos/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	keyFilePathFlagName = "key_path"

	rpcHostEnvVar  = "JASPER_RPC_HOST"
	rpcPortEnvVar  = "JASPER_RPC_PORT"
	defaultRPCPort = 2486
)

func serviceCommandRPC(cmd string, operation serviceOperation) cli.Command {
	return cli.Command{
		Name:  rpcService,
		Usage: fmt.Sprintf("%s an RPC service", cmd),
		Flags: []cli.Flag{
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
				Name:  keyFilePathFlagName,
				Usage: "the path to the certificate file",
			},
			cli.StringFlag{
				Name:  certFilePathFlagName,
				Usage: "the path to the key file",
			},
		},
		Before: validatePort(portFlagName),
		Action: func(c *cli.Context) error {
			manager, err := jasper.NewLocalManager(false)
			if err != nil {
				return errors.Wrap(err, "error creating RPC manager")
			}

			daemon := newRPCDaemon(c.String(hostFlagName), c.Int(portFlagName), c.String(certFilePathFlagName), c.String(keyFilePathFlagName), manager)

			config := serviceConfig(rpcService, buildRunCommand(c, rpcService))

			return operation(daemon, config)
		},
	}
}

type rpcDaemon struct {
	Host         string
	Port         int
	KeyFilePath  string
	CertFilePath string
	Manager      jasper.Manager

	exit chan struct{}
}

func newRPCDaemon(host string, port int, certFilePath, keyFilePath string, manager jasper.Manager) *rpcDaemon {
	return &rpcDaemon{
		Host:         host,
		Port:         port,
		CertFilePath: certFilePath,
		KeyFilePath:  keyFilePath,
		Manager:      manager,
	}
}

func (d *rpcDaemon) Start(s service.Service) error {
	d.exit = make(chan struct{})
	if d.Manager == nil {
		var err error
		if d.Manager, err = jasper.NewLocalManager(false); err != nil {
			return errors.Wrap(err, "failed to construct RPC manager")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go handleDaemonSignals(ctx, cancel, d.exit)

	go func(ctx context.Context, d *rpcDaemon) {
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

func (d *rpcDaemon) newService(ctx context.Context) (jasper.CloseFunc, error) {
	if d.Manager == nil {
		return nil, errors.New("manager is not set on RPC service")
	}
	grip.Infof("starting RPC service at '%s:%d'", d.Host, d.Port)
	return newRPCService(ctx, d.Host, d.Port, d.Manager, d.CertFilePath, d.KeyFilePath)
}

// newRPCService creates an RPC service around the manager serving requests on
// the host and port.
func newRPCService(ctx context.Context, host string, port int, manager jasper.Manager, certFilePath, keyFilePath string) (jasper.CloseFunc, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve RPC address")
	}

	closeService, err := rpc.StartService(ctx, manager, addr, certFilePath, keyFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "error starting RPC service")
	}

	return closeService, nil
}

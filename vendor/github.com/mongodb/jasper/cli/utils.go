package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/kardianos/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	restService     = "rest"
	rpcService      = "rpc"
	combinedService = "combined"
)

// mergeBeforeFuncs returns a cli.BeforeFunc that runs all funcs and accumulates
// the errors.
func mergeBeforeFuncs(funcs ...cli.BeforeFunc) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()
		for _, f := range funcs {
			catcher.Add(f(c))
		}
		return catcher.Resolve()
	}
}

// joinFlagNames joins multiple CLI flag names.
func joinFlagNames(names ...string) string {
	return strings.Join(names, ", ")
}

// unparseFlagSet returns all flags set in the given context in the form
// --flag=value.
func unparseFlagSet(c *cli.Context) []string {
	args := []string{}
	for _, flagName := range c.FlagNames() {
		if !c.IsSet(flagName) {
			continue
		}
		args = append(args, fmt.Sprintf("--%s=%s", flagName, c.String(flagName)))
	}
	return args
}

const (
	minPort = 1 << 10
	maxPort = math.MaxUint16 - 1
)

// clientFlags returns flags used by all client commands.
func clientFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  hostFlagName,
			Usage: "the host running the Jasper service",
			Value: defaultLocalHostName,
		},
		cli.IntFlag{
			Name:  portFlagName,
			Usage: fmt.Sprintf("the port running the Jasper service (if service is '%s', default port is %d; if service is '%s', default port is %d)", restService, defaultRESTPort, rpcService, defaultRPCPort),
		},
		cli.StringFlag{
			Name:  serviceFlagName,
			Usage: fmt.Sprintf("the type of Jasper service ('%s' or '%s')", restService, rpcService),
		},
		cli.StringFlag{
			Name:  credsFilePathFlagName,
			Usage: "the path to the file containing the credentials",
		},
	}
}

// clientBefore returns the cli.BeforeFunc used by all client commands.
func clientBefore() func(*cli.Context) error {
	return mergeBeforeFuncs(
		func(c *cli.Context) error {
			service := c.String(serviceFlagName)
			if service != restService && service != rpcService {
				return errors.Errorf("service must be '%s' or '%s'", restService, rpcService)
			}
			return nil
		},
		func(c *cli.Context) error {
			if c.Int(portFlagName) != 0 {
				return nil
			}
			switch c.String(serviceFlagName) {
			case restService:
				if err := c.Set(portFlagName, strconv.Itoa(defaultRESTPort)); err != nil {
					return err
				}
			case rpcService:
				if err := c.Set(portFlagName, strconv.Itoa(defaultRPCPort)); err != nil {
					return err
				}
			}
			return validatePort(portFlagName)(c)
		},
	)
}

// validatePort validates that the flag given by the name is a valid port value.
func validatePort(flagName string) func(*cli.Context) error {
	return func(c *cli.Context) error {
		port := c.Int(flagName)
		if port < minPort || port > maxPort {
			return errors.Errorf("port must be between %d-%d exclusive", minPort, maxPort)
		}
		return nil
	}
}

// readInput reads JSON from the input and decodes it to the output.
func readInput(input io.Reader, output interface{}) error {
	bytes, err := ioutil.ReadAll(input)
	if err != nil {
		return errors.Wrap(err, "error reading from input")
	}
	return errors.Wrap(json.Unmarshal(bytes, output), "error decoding to output")
}

// writeOutput encodes the output as JSON and writes it to w.
func writeOutput(output io.Writer, input interface{}) error {
	bytes, err := json.MarshalIndent(input, "", "    ")
	if err != nil {
		return errors.Wrap(err, "error encoding input")
	}
	if _, err := output.Write(bytes); err != nil {
		return errors.Wrap(err, "error writing to output")
	}

	return nil
}

// newRemoteClient returns a remote client that connects to the service at the
// given host and port, with the optional TLS credentials file for RPC
// communication.
func newRemoteClient(ctx context.Context, service, host string, port int, credsFilePath string) (jasper.RemoteClient, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve address")
	}

	if service == restService {
		return jasper.NewRESTClient(addr), nil
	} else if service == rpcService {
		return rpc.NewClientWithFile(ctx, addr, credsFilePath)
	}
	return nil, errors.Errorf("unrecognized service type '%s'", service)
}

// doPassthroughInputOutput passes input from standard input to the input validator,
// validates the input, runs the request, and writes the response of the request
// to standard output.
func doPassthroughInputOutput(c *cli.Context, input Validator, request func(context.Context, jasper.RemoteClient) (response interface{})) error {
	ctx, cancel := context.WithTimeout(context.Background(), clientConnectionTimeout)
	defer cancel()

	if err := readInput(os.Stdin, input); err != nil {
		return errors.Wrap(err, "error reading from standard input")
	}
	if err := input.Validate(); err != nil {
		return errors.Wrap(err, "input is invalid")
	}

	return withConnection(ctx, c, func(client jasper.RemoteClient) error {
		return errors.Wrap(writeOutput(os.Stdout, request(ctx, client)), "error writing to standard output")
	})
}

// doPassthroughOutput runs the request and writes the output of the request to
// standard output.
func doPassthroughOutput(c *cli.Context, request func(context.Context, jasper.RemoteClient) (response interface{})) error {
	ctx, cancel := context.WithTimeout(context.Background(), clientConnectionTimeout)
	defer cancel()

	return withConnection(ctx, c, func(client jasper.RemoteClient) error {
		return errors.Wrap(writeOutput(os.Stdout, request(ctx, client)), "error writing to standard output")
	})
}

// withConnection runs the operation within the scope of a remote client
// connection.
func withConnection(ctx context.Context, c *cli.Context, operation func(jasper.RemoteClient) error) error {
	host := c.String(hostFlagName)
	port := c.Int(portFlagName)
	service := c.String(serviceFlagName)
	credsFilePath := c.String(credsFilePathFlagName)

	client, err := newRemoteClient(ctx, service, host, port, credsFilePath)
	if err != nil {
		return errors.Wrap(err, "error setting up remote client")
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(operation(client))
	catcher.Add(client.CloseConnection())

	return catcher.Resolve()
}

// withService runs the operation with a new service.
func withService(daemon service.Interface, config *service.Config, operation func(service.Service) error) error {
	svc, err := service.New(daemon, config)
	if err != nil {
		return errors.Wrap(err, "error initializing new service")
	}
	return operation(svc)
}

// runServices starts the given services, waits until the context is done, and
// closes all the running services.
func runServices(ctx context.Context, makeServices ...func(context.Context) (jasper.CloseFunc, error)) error {
	closeServices := []jasper.CloseFunc{}
	closeAllServices := func(closeServices []jasper.CloseFunc) error {
		catcher := grip.NewBasicCatcher()
		for _, closeService := range closeServices {
			catcher.Add(errors.Wrap(closeService(), "error closing service"))
		}
		return catcher.Resolve()
	}

	for _, makeService := range makeServices {
		closeService, err := makeService(ctx)
		if err != nil {
			catcher := grip.NewBasicCatcher()
			catcher.Wrap(err, "failed to create service")
			catcher.Add(closeAllServices(closeServices))
			return catcher.Resolve()
		}
		closeServices = append(closeServices, closeService)
	}

	<-ctx.Done()
	return closeAllServices(closeServices)
}

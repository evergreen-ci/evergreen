package cli

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// makeRemoteClient returns a remote client that connects to the service at the
// given host and port, with the optional SSL/TLS credentials file specified at
// the given location.
func makeRemoteClient(ctx context.Context, service, host string, port int, certFilePath string) (jasper.RemoteClient, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve address")
	}

	if service == serviceREST {
		return jasper.NewRESTClient(addr), nil
	} else if service == serviceRPC {
		return rpc.NewClient(ctx, addr, certFilePath)
	}
	return nil, errors.Errorf("unrecognized service type '%s'", service)
}

// doPassthroughInputOutput passes input from stdin to the input validator,
// validates the input, runs the request, and writes the response of the request
// to stdout.
func doPassthroughInputOutput(c *cli.Context, input Validator, request func(context.Context, jasper.RemoteClient) (response interface{})) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := readInput(os.Stdin, input); err != nil {
		return errors.Wrap(err, "error reading from stdin")
	}
	if err := input.Validate(); err != nil {
		return errors.Wrap(err, "input is invalid")
	}

	return withConnection(ctx, c, func(client jasper.RemoteClient) error {
		return errors.Wrap(writeOutput(os.Stdout, request(ctx, client)), "error writing to stdout")
	})
}

// doPassthroughOutput runs the request and writes the output of the request to
// stdout.
func doPassthroughOutput(c *cli.Context, request func(context.Context, jasper.RemoteClient) (response interface{})) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return withConnection(ctx, c, func(client jasper.RemoteClient) error {
		return errors.Wrap(writeOutput(os.Stdout, request(ctx, client)), "error writing to stdout")
	})
}

// withConnection runs the operation within the scope of a remote client
// connection.
func withConnection(ctx context.Context, c *cli.Context, operation func(jasper.RemoteClient) error) error {
	host := c.GlobalString(hostFlagName)
	port := c.GlobalInt(portFlagName)
	service := c.GlobalString(serviceFlagName)
	certFilePath := c.GlobalString(certFilePathFlagName)

	client, err := makeRemoteClient(ctx, service, host, port, certFilePath)
	if err != nil {
		return errors.Wrap(err, "error setting up remote client")
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(operation(client))
	catcher.Add(client.CloseConnection())

	return catcher.Resolve()
}

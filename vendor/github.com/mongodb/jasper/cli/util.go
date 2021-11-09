package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	service "github.com/evergreen-ci/baobab"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/remote"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
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
func unparseFlagSet(c *cli.Context, serviceType string) []string {
	for i, arg := range os.Args {
		if arg == serviceType && i+1 < len(os.Args) {
			return os.Args[i+1:]
		}
	}
	return []string{}
}

func requireStringFlag(name string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if c.String(name) == "" {
			return errors.Errorf("must specify string for flag '%s'", name)
		}
		return nil
	}
}

func requireOneFlag(names ...string) cli.BeforeFunc { //nolint: deadcode
	return func(c *cli.Context) error {
		var count int
		for _, name := range names {
			if c.IsSet(name) {
				count++
			}
		}
		if count != 1 {
			return errors.Errorf("must specify exactly one flag from the following: %s", names)
		}
		return nil
	}
}

// requireRelativePath verifies that the path flag relPathFlagName is set to a
// path relative to the directory path set for dirFlagName.
//nolint: deadcode
func requireRelativePath(relPathFlagName, pathFlagName string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		relPath := util.ConsistentFilepath(c.String(relPathFlagName))
		path := util.ConsistentFilepath(c.String(pathFlagName))
		if filepath.IsAbs(relPath) {
			if strings.HasPrefix(relPath, path) {
				relPath = strings.TrimPrefix(relPath, path)
				return errors.Wrapf(c.Set(relPathFlagName, relPath), "setting flag '%s' to relative path", relPathFlagName)
			}
			return errors.Errorf("path '%s' must be relative to the path '%s'", relPath, path)
		}
		return nil
	}
}

// requireStringSliceFlag verifies that the flag name is set to a non-empty
// string slice.
//nolint: deadcode
func requireStringSliceFlag(name string) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if len(c.StringSlice(name)) == 0 {
			return errors.Errorf("must specify at least one string for flag '%s'", name)
		}
		return nil
	}
}

const (
	minPort = 1 << 10
	maxPort = math.MaxUint16 - 1
)

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

// newRemoteManager returns a remote.Manager that connects to the service at the
// given host and port, with the optional TLS credentials file for RPC
// communication.
func newRemoteManager(ctx context.Context, service, host string, port int, credsFilePath string) (remote.Manager, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve address")
	}

	if service == RESTService {
		return remote.NewRESTClient(addr), nil
	} else if service == RPCService {
		return remote.NewRPCClientWithFile(ctx, addr, credsFilePath)
	}
	return nil, errors.Errorf("unrecognized service type '%s'", service)
}

// doPassthroughInputOutput passes input from standard input to the input
// validator, validates the input, runs the request, and writes the response of
// the request to standard output.
func doPassthroughInputOutput(c *cli.Context, input Validator, request func(context.Context, remote.Manager) (response interface{})) error {
	ctx, cancel := context.WithTimeout(context.Background(), clientConnectionTimeout)
	defer cancel()

	if err := readInput(os.Stdin, input); err != nil {
		return errors.Wrap(err, "error reading from standard input")
	}
	if err := input.Validate(); err != nil {
		return errors.Wrap(err, "input is invalid")
	}

	return withConnection(ctx, c, func(client remote.Manager) error {
		return errors.Wrap(writeOutput(os.Stdout, request(ctx, client)), "error writing to standard output")
	})
}

// doPassthroughOutput runs the request and writes the output of the request to
// standard output.
func doPassthroughOutput(c *cli.Context, request func(context.Context, remote.Manager) (response interface{})) error {
	ctx, cancel := context.WithTimeout(context.Background(), clientConnectionTimeout)
	defer cancel()

	return withConnection(ctx, c, func(client remote.Manager) error {
		return errors.Wrap(writeOutput(os.Stdout, request(ctx, client)), "error writing to standard output")
	})
}

// withConnection runs the operation within the scope of a remote client
// connection.
func withConnection(ctx context.Context, c *cli.Context, operation func(remote.Manager) error) error {
	host := c.String(hostFlagName)
	port := c.Int(portFlagName)
	service := c.String(serviceFlagName)
	credsFilePath := c.String(credsFilePathFlagName)

	client, err := newRemoteManager(ctx, service, host, port, credsFilePath)
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
func runServices(ctx context.Context, makeServices ...func(context.Context) (util.CloseFunc, error)) error {
	closeServices := []util.CloseFunc{}
	closeAllServices := func(closeServices []util.CloseFunc) error {
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

func randDur(window time.Duration) time.Duration {
	return window + time.Duration(rand.Int63n(int64(window)))
}

// CappedWriter implements a buffer that stores up to MaxBytes bytes.
// Returns ErrBufferFull on overflowing writes.
type CappedWriter struct {
	Buffer   *bytes.Buffer
	MaxBytes int
}

// ErrBufferFull returns an error indicating that a CappedWriter's buffer has
// reached max capacity.
func ErrBufferFull() error {
	return errors.New("buffer is full")
}

// Write writes to the buffer. An error is returned if the buffer is full.
func (cw *CappedWriter) Write(in []byte) (int, error) {
	remaining := cw.MaxBytes - cw.Buffer.Len()
	if len(in) <= remaining {
		return cw.Buffer.Write(in)
	}
	// fill up the remaining buffer and return an error
	n, _ := cw.Buffer.Write(in[:remaining])
	return n, ErrBufferFull()
}

// IsFull indicates whether the buffer is full.
func (cw *CappedWriter) IsFull() bool {
	return cw.Buffer.Len() == cw.MaxBytes
}

// String return the contents of the buffer as a string.
func (cw *CappedWriter) String() string {
	return cw.Buffer.String()
}

// Bytes returns the contents of the buffer.
func (cw *CappedWriter) Bytes() []byte {
	return cw.Buffer.Bytes()
}

// Close is a noop method so that you can use CappedWriter as an
// io.WriteCloser.
func (cw *CappedWriter) Close() error { return nil }

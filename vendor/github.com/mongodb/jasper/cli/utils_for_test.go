package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

const testTimeout = 2 * time.Second

var nextPort <-chan int

func init() {
	nextPort = func() <-chan int {
		out := make(chan int, 25)
		go func() {
			id := 4000
			for {
				id++
				out <- id
			}
		}()
		return out
	}()
}

// noWhitespace returns the string str without whitespace.
func noWhitespace(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, str)
}

// getNextPort returns a new port.
func getNextPort() int {
	return <-nextPort
}

// buildDir gets the Jasper build directory.
func buildDir(t *testing.T) string {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(filepath.Dir(cwd), "build")
}

func trueCreateOpts() *jasper.CreateOptions {
	return &jasper.CreateOptions{Args: []string{"true"}}
}

func sleepCreateOpts(timeoutSecs int) *jasper.CreateOptions {
	return &jasper.CreateOptions{Args: []string{"sleep", strconv.Itoa(timeoutSecs)}}
}

// mockCLIContext creates a *cli.Context on localhost with the given service
// type and port.
func mockCLIContext(service string, port int) *cli.Context {
	flags := &flag.FlagSet{}
	_ = flags.String(serviceFlagName, service, "")
	_ = flags.Int(portFlagName, port, "")
	_ = flags.String(hostFlagName, "localhost", "")
	_ = flags.String(credsFilePathFlagName, "", "")
	return cli.NewContext(nil, flags, nil)
}

type mockInput struct {
	Value     string `json:"value"`
	validated bool
}

func (m *mockInput) Validate() error {
	m.validated = true
	return nil
}

type mockOutput struct {
	Value string `json:"value"`
}

// mockRequest returns a function that returns a mockOutput with the given
// value val.
func mockRequest(val string) func(context.Context, jasper.RemoteClient) interface{} {
	return func(context.Context, jasper.RemoteClient) interface{} {
		return mockOutput{val}
	}
}

// withMockStdin runs the operation with a stdin that contains the given input.
// It passes the mocked stdin as a parameter to the operation.
func withMockStdin(t *testing.T, input string, operation func(*os.File) error) error {
	stdin := os.Stdin
	defer func() {
		os.Stdin = stdin
	}()
	tmpFile, err := ioutil.TempFile(buildDir(t), "mock_stdin.txt")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, tmpFile.Close())
		assert.NoError(t, os.Remove(tmpFile.Name()))
	}()
	_, err = tmpFile.WriteString(input)
	require.NoError(t, err)
	_, err = tmpFile.Seek(0, 0)
	require.NoError(t, err)
	os.Stdin = tmpFile
	return operation(os.Stdin)
}

// withMockStdout runs the operation with a stdout that can be inspected as a
// regular file. It passes the mocked stdout to the operation.
func withMockStdout(t *testing.T, operation func(*os.File) error) error {
	stdout := os.Stdout
	defer func() {
		os.Stdout = stdout
	}()
	tmpFile, err := ioutil.TempFile(buildDir(t), "mock_stdout.txt")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, tmpFile.Close())
		assert.NoError(t, os.Remove(tmpFile.Name()))
	}()
	os.Stdout = tmpFile
	return operation(os.Stdout)
}

// waitForRESTService waits until the REST service becomes available to serve
// requests or the context times out.
func waitForRESTService(ctx context.Context, t *testing.T, url string) {
	// Block until the service comes up
	timeoutInterval := 10 * time.Millisecond
	timer := time.NewTimer(timeoutInterval)
	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "test timed out before REST service was available")
			return
		case <-timer.C:
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				timer.Reset(timeoutInterval)
				continue
			}
			req = req.WithContext(ctx)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				timer.Reset(timeoutInterval)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				timer.Reset(timeoutInterval)
				continue
			}
			return
		}
	}
}

// execCLICommandInputOutput runs the CLI command with the given input to stdin
// and writes the result from stdout to output.
func execCLICommandInputOutput(t *testing.T, c *cli.Context, cmd cli.Command, input []byte, output interface{}) error {
	return withMockStdin(t, string(input), func(*os.File) error {
		return execCLICommandOutput(t, c, cmd, output)
	})
}

// execCLICommandInputOutput runs the CLI command and writes the result from
// stdout to output.
func execCLICommandOutput(t *testing.T, c *cli.Context, cmd cli.Command, output interface{}) error {
	return withMockStdout(t, func(stdout *os.File) error {
		if err := cli.HandleAction(cmd.Action, c); err != nil {
			return err
		}
		if _, err := stdout.Seek(0, 0); err != nil {
			return err
		}
		resp, err := ioutil.ReadAll(stdout)
		if err != nil {
			return err
		}
		return json.Unmarshal(resp, output)
	})
}

// makeTestRESTService creates a REST service for testing purposes only on
// localhost.
func makeTestRESTService(ctx context.Context, t *testing.T, port int, manager jasper.Manager) jasper.CloseFunc {
	closeService, err := newRESTService(ctx, "localhost", port, manager)
	require.NoError(t, err)
	waitForRESTService(ctx, t, fmt.Sprintf("http://localhost:%d/jasper/v1", port))
	return closeService
}

// makeTestRESTServiceAndClient creates a REST service and client for testing
// purposes on localhost.
func makeTestRESTServiceAndClient(ctx context.Context, t *testing.T, port int, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
	closeService := makeTestRESTService(ctx, t, port, manager)
	client, err := newRemoteClient(ctx, RESTService, "localhost", port, "")
	require.NoError(t, err)
	return closeService, client
}

// makeTestRPCService creates an RPC service for testing purposes only on
// localhost with no credentials.
func makeTestRPCService(ctx context.Context, t *testing.T, port int, manager jasper.Manager) jasper.CloseFunc {
	closeService, err := newRPCService(ctx, "localhost", port, manager, "")
	require.NoError(t, err)
	return closeService
}

// makeTestRESTServiceAndClient creates an RPC servicen and client for testing
// purposes on localhost with no credentials.
func makeTestRPCServiceAndClient(ctx context.Context, t *testing.T, port int, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
	closeService := makeTestRPCService(ctx, t, port, manager)
	client, err := newRemoteClient(ctx, RPCService, "localhost", port, "")
	require.NoError(t, err)
	return closeService, client
}

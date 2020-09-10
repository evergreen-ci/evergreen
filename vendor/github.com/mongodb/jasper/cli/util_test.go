package cli

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/evergreen-ci/service"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/remote"
	"github.com/mongodb/jasper/testutil"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestReadInputValidJSON(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":"bar","bat":"baz","qux":[1,2,3,4,5]}`)
	output := struct {
		Foo string `json:"foo"`
		Bat string `json:"bat"`
		Qux []int  `json:"qux"`
	}{}
	require.NoError(t, readInput(input, &output))
	assert.Equal(t, "bar", output.Foo)
	assert.Equal(t, "baz", output.Bat)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, output.Qux)
}

func TestReadInputInvalidInput(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":}`)
	output := struct {
		Foo string `json:"foo"`
	}{}
	assert.Error(t, readInput(input, &output))
}

func TestReadInputInvalidOutput(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":"bar"}`)
	output := make(chan struct{})
	assert.Error(t, readInput(input, output))
}

func TestWriteOutput(t *testing.T) {
	input := struct {
		Foo string `json:"foo"`
		Bat string `json:"bat"`
		Qux []int  `json:"qux"`
	}{
		Foo: "bar",
		Bat: "baz",
		Qux: []int{1, 2, 3, 4, 5},
	}
	inputBuf := bytes.NewBufferString(`
	{
	"foo": "bar",
	"bat": "baz",
	"qux": [1 ,2, 3, 4, 5]
	}
	`)
	inputString := inputBuf.String()
	output := &bytes.Buffer{}
	require.NoError(t, writeOutput(output, input))
	assert.Equal(t, testutil.RemoveWhitespace(inputString), testutil.RemoveWhitespace(output.String()))
}

func TestWriteOutputInvalidInput(t *testing.T) {
	input := make(chan struct{})
	output := &bytes.Buffer{}
	assert.Error(t, writeOutput(output, input))
}

func TestWriteOutputInvalidOutput(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":"bar"}`)

	output, err := ioutil.TempFile(testutil.BuildDirectory(), "write_output.txt")
	require.NoError(t, err)
	defer os.RemoveAll(output.Name())
	require.NoError(t, output.Close())
	assert.Error(t, writeOutput(output, input))
}

func TestMakeRemoteManagerInvalidService(t *testing.T) {
	ctx := context.Background()
	client, err := newRemoteManager(ctx, "invalid", "localhost", testutil.GetPortNumber(), "")
	require.Error(t, err)
	require.Nil(t, client)
}

func TestMakeRemoteManager(t *testing.T) {
	for remoteType, makeServiceAndClient := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) (util.CloseFunc, remote.Manager){
		RESTService: makeTestRESTServiceAndClient,
		RPCService:  makeTestRPCServiceAndClient,
	} {
		t.Run(remoteType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
			defer cancel()
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			closeService, client := makeServiceAndClient(ctx, t, testutil.GetPortNumber(), manager)
			assert.NoError(t, closeService())
			assert.NoError(t, client.CloseConnection())
		})
	}
}

func TestCLICommon(t *testing.T) {
	for remoteType, makeServiceAndClient := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) (util.CloseFunc, remote.Manager){
		RESTService: makeTestRESTServiceAndClient,
		RPCService:  makeTestRPCServiceAndClient,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error{
				"CreateProcessWithConnection": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error {
					return withConnection(ctx, c, func(client remote.Manager) error {
						proc, err := client.CreateProcess(ctx, testutil.TrueCreateOpts())
						require.NoError(t, err)
						require.NotNil(t, proc)
						assert.NotZero(t, proc.Info(ctx).PID)
						return nil
					})
				},
				"DoPassthroughInputOutputReadsFromStdin": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error {
					return withMockStdin(t, `{"value":"foo"}`, func(stdin *os.File) error {
						return withMockStdout(t, func(*os.File) error {
							input := &mockInput{}
							require.NoError(t, doPassthroughInputOutput(c, input, mockRequest("")))
							output, err := ioutil.ReadAll(stdin)
							require.NoError(t, err)
							assert.Len(t, output, 0)
							return nil
						})
					})
				},
				"DoPassthroughInputOutputSetsAndValidatesInput": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error {
					expectedInput := "foo"
					return withMockStdin(t, fmt.Sprintf(`{"value":"%s"}`, expectedInput), func(*os.File) error {
						return withMockStdout(t, func(*os.File) error {
							input := &mockInput{}
							require.NoError(t, doPassthroughInputOutput(c, input, mockRequest("")))
							assert.Equal(t, expectedInput, input.Value)
							assert.True(t, input.validated)
							return nil
						})
					})
				},
				"DoPassthroughInputOutputWritesResponseToStdout": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error {
					return withMockStdin(t, `{"value":"foo"}`, func(*os.File) error {
						return withMockStdout(t, func(stdout *os.File) error {
							input := &mockInput{}
							outputVal := "bar"
							require.NoError(t, doPassthroughInputOutput(c, input, mockRequest(outputVal)))
							assert.Equal(t, "foo", input.Value)
							assert.True(t, input.validated)

							expectedOutput := `{"value":"bar"}`
							_, err := stdout.Seek(0, 0)
							require.NoError(t, err)
							output, err := ioutil.ReadAll(stdout)
							require.NoError(t, err)
							assert.Equal(t, testutil.RemoveWhitespace(expectedOutput), testutil.RemoveWhitespace(string(output)))
							return nil
						})
					})
				},
				"DoPassthroughOutputIgnoresStdin": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error {
					input := "foo"
					return withMockStdin(t, input, func(stdin *os.File) error {
						return withMockStdout(t, func(*os.File) error {
							require.NoError(t, doPassthroughOutput(c, mockRequest("")))
							output, err := ioutil.ReadAll(stdin)
							require.NoError(t, err)
							assert.Len(t, output, len(input))
							return nil

						})
					})
				},
				"DoPassthroughOutputWritesResponseToStdout": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) error {
					return withMockStdout(t, func(stdout *os.File) error {
						outputVal := "bar"
						require.NoError(t, doPassthroughOutput(c, mockRequest(outputVal)))

						expectedOutput := `{"value": "bar"}`
						_, err := stdout.Seek(0, 0)
						require.NoError(t, err)
						output, err := ioutil.ReadAll(stdout)
						require.NoError(t, err)
						assert.Equal(t, testutil.RemoveWhitespace(expectedOutput), testutil.RemoveWhitespace(string(output)))
						return nil
					})
				},
				// "": func(ctx context.Context, t *testing.T, c *cli.Context, client remote.Manager) err {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()
					port := testutil.GetPortNumber()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewSynchronizedManager(false)
					require.NoError(t, err)
					closeService, client := makeServiceAndClient(ctx, t, port, manager)
					defer func() {
						assert.NoError(t, client.CloseConnection())
						assert.NoError(t, closeService())
					}()

					assert.NoError(t, testCase(ctx, t, c, client))
				})
			}
		})
	}
}

func TestWithService(t *testing.T) {
	svcFuncRan := false
	svcFunc := func(svc service.Service) error {
		svcFuncRan = true
		return nil
	}
	assert.Error(t, withService(&rpcDaemon{}, &service.Config{}, svcFunc))
	assert.False(t, svcFuncRan)

	assert.NoError(t, withService(&rpcDaemon{}, &service.Config{Name: "foo"}, svcFunc))
	assert.True(t, svcFuncRan)
}

func TestRunServices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	assert.NoError(t, runServices(ctx))
	assert.Equal(t, context.DeadlineExceeded, ctx.Err())

	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	assert.Error(t, runServices(ctx, func(ctx context.Context) (util.CloseFunc, error) {
		return nil, ctx.Err()
	}))

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	assert.NoError(t, runServices(ctx, func(ctx context.Context) (util.CloseFunc, error) {
		return func() error { return nil }, nil
	}))

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	closeFuncCalled := false
	closeFunc := func() error {
		closeFuncCalled = true
		return nil
	}

	assert.Error(t, runServices(ctx, func(ctx context.Context) (util.CloseFunc, error) {
		return closeFunc, errors.New("fail to make service")
	}))
	assert.False(t, closeFuncCalled)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	closeFuncCalled = false

	assert.NoError(t, runServices(ctx, func(ctx context.Context) (util.CloseFunc, error) {
		return closeFunc, nil
	}))
	assert.True(t, closeFuncCalled)

	anotherCloseFuncCalled := false
	anotherCloseFunc := func() error {
		anotherCloseFuncCalled = true
		return nil
	}

	assert.Error(t, runServices(ctx, func(ctx context.Context) (util.CloseFunc, error) {
		return closeFunc, nil
	}, func(ctx context.Context) (util.CloseFunc, error) {
		return anotherCloseFunc, errors.New("fail to make another service")
	}))
	assert.True(t, closeFuncCalled)
	assert.False(t, anotherCloseFuncCalled)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	closeFuncCalled = false
	anotherCloseFuncCalled = false

	assert.NoError(t, runServices(ctx, func(ctx context.Context) (util.CloseFunc, error) {
		return closeFunc, nil
	}, func(ctx context.Context) (util.CloseFunc, error) {
		return anotherCloseFunc, nil
	}))
	assert.True(t, closeFuncCalled)
	assert.True(t, anotherCloseFuncCalled)
}

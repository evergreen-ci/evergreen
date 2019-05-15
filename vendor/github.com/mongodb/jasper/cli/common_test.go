package cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestRemoteClientInvalidService(t *testing.T) {
	ctx := context.Background()
	client, err := makeRemoteClient(ctx, "invalid", "localhost", getNextPort(), "")
	require.Error(t, err)
	require.Nil(t, client)
}

func TestRemoteClient(t *testing.T) {
	for remoteType, makeServiceAndClient := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient){
		serviceREST: makeRESTServiceAndClient,
		serviceRPC:  makeRPCServiceAndClient,
	} {
		t.Run(remoteType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()
			manager, err := jasper.NewLocalManager(false)
			require.NoError(t, err)
			closeService, client := makeServiceAndClient(ctx, t, getNextPort(), manager)
			assert.NoError(t, closeService())
			assert.NoError(t, client.CloseConnection())
		})
	}
}

func TestCLICommon(t *testing.T) {
	for remoteType, makeServiceAndClient := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient){
		serviceREST: makeRESTServiceAndClient,
		serviceRPC:  makeRPCServiceAndClient,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient){
				"CreateProcessWithConnection": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {
					withConnection(ctx, c, func(client jasper.RemoteClient) error {
						proc, err := client.CreateProcess(ctx, trueCreateOpts())
						require.NoError(t, err)
						require.NotNil(t, proc)
						assert.NotZero(t, proc.Info(ctx).PID)
						return nil
					})
				},
				"DoPassthroughInputOutputReadsFromStdin": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {
					withMockStdin(t, `{"value":"foo"}`, func(stdin *os.File) error {
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
				"DoPassthroughInputOutputSetsAndValidatesInput": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {
					expectedInput := "foo"
					withMockStdin(t, fmt.Sprintf(`{"value":"%s"}`, expectedInput), func(*os.File) error {
						return withMockStdout(t, func(*os.File) error {
							input := &mockInput{}
							require.NoError(t, doPassthroughInputOutput(c, input, mockRequest("")))
							assert.Equal(t, expectedInput, input.Value)
							assert.True(t, input.validated)
							return nil
						})
					})
				},
				"DoPassthroughInputOutputWritesResponseToStdout": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {
					withMockStdin(t, `{"value":"foo"}`, func(*os.File) error {
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
							assert.Equal(t, noWhitespace(expectedOutput), noWhitespace(string(output)))
							return nil
						})
					})
				},
				"DoPassthroughOutputIgnoresStdin": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {
					input := "foo"
					withMockStdin(t, input, func(stdin *os.File) error {
						return withMockStdout(t, func(*os.File) error {
							require.NoError(t, doPassthroughOutput(c, mockRequest("")))
							output, err := ioutil.ReadAll(stdin)
							require.NoError(t, err)
							assert.Len(t, output, len(input))
							return nil

						})
					})
				},
				"DoPassthroughOutputWritesResponseToStdout": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {
					withMockStdout(t, func(stdout *os.File) error {
						outputVal := "bar"
						require.NoError(t, doPassthroughOutput(c, mockRequest(outputVal)))

						expectedOutput := `{"value": "bar"}`
						_, err := stdout.Seek(0, 0)
						require.NoError(t, err)
						output, err := ioutil.ReadAll(stdout)
						require.NoError(t, err)
						assert.Equal(t, noWhitespace(expectedOutput), noWhitespace(string(output)))
						return nil
					})
				},
				// "": func(ctx context.Context, t *testing.T, c *cli.Context, client jasper.RemoteClient) {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
					defer cancel()
					port := getNextPort()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewLocalManager(false)
					require.NoError(t, err)
					closeService, client := makeServiceAndClient(ctx, t, port, manager)
					defer func() {
						assert.NoError(t, client.CloseConnection())
						assert.NoError(t, closeService())
					}()

					testCase(ctx, t, c, client)
				})
			}
		})
	}
}

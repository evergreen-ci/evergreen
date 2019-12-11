package cli

import (
	"context"
	"encoding/json"
	"syscall"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/mongodb/jasper/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestCLIRemoteWindows(t *testing.T) {
	for remoteType, makeService := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) util.CloseFunc{
		RESTService: makeTestRESTService,
		RPCService:  makeTestRPCService,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context){
				"SignalEventPasses": func(ctx context.Context, t *testing.T, c *cli.Context) {
					eventName := "event"
					utf16EventName, err := syscall.UTF16PtrFromString(eventName)
					require.NoError(t, err)

					event, err := jasper.CreateEvent(utf16EventName)
					require.NoError(t, err)
					defer jasper.CloseHandle(event)

					input, err := json.Marshal(EventInput{Name: eventName})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, remoteSignalEvent(), input, resp))
					assert.True(t, resp.Successful())
				},
			} {

				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					port := testutil.GetPortNumber()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewSynchronizedManager(false)
					require.NoError(t, err)
					closeService := makeService(ctx, t, port, manager)
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, closeService())
					}()

					testCase(ctx, t, c)
				})
			}
		})
	}
}

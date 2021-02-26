package remote

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsEvents(t *testing.T) {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	for clientName, makeClient := range map[string]func(ctx context.Context, t *testing.T) Manager{
		"RPC": func(ctx context.Context, t *testing.T) Manager {
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			client, err := makeInsecureRPCServiceAndClient(ctx, manager)
			require.NoError(t, err)
			return client
		},
		"REST": func(ctx context.Context, t *testing.T) Manager {
			mngr, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			_, client, err := makeRESTServiceAndClient(ctx, mngr, httpClient)
			require.NoError(t, err)
			return client
		},
	} {
		t.Run(clientName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, Manager){
				"SignalEventFailsWithNonexistentEvent": func(ctx context.Context, t *testing.T, client Manager) {
					assert.Error(t, client.SignalEvent(ctx, "foo"))
				},
				"SignalEventWithExistingEvent": func(ctx context.Context, t *testing.T, client Manager) {
					eventName := "ThisIsARealEvent"
					utf16EventName, err := syscall.UTF16PtrFromString(eventName)
					require.NoError(t, err)

					event, err := jasper.CreateEvent(utf16EventName)
					require.NoError(t, err)
					defer jasper.CloseHandle(event)

					require.NoError(t, client.SignalEvent(ctx, eventName))

					status, err := jasper.WaitForSingleObject(event, time.Second)
					require.NoError(t, err)
					assert.Equal(t, jasper.WAIT_OBJECT_0, status)
				},
				// "": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					client := makeClient(ctx, t)
					defer func() {
						assert.NoError(t, client.CloseConnection())
					}()

					testCase(ctx, t, client)
				})
			}
		})
	}
}

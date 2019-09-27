package rest

import (
	"context"
	"fmt"
	"syscall"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsRESTService(t *testing.T) {
	httpClient := testutil.GetHTTPClient()
	defer testutil.PutHTTPClient(httpClient)

	for testName, testCase := range map[string]func(context.Context, *testing.T, *Service, *restClient){
		"SignalEventWithNonexistentEvent": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			assert.Error(t, client.SignalEvent(ctx, "foo"))
		},
		"SignalEventWithExistingEvent": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			eventName := "ThisIsARealEvent"
			utf16EventName, err := syscall.UTF16PtrFromString(eventName)
			require.NoError(t, err)

			event, err := jasper.CreateEvent(utf16EventName)
			require.NoError(t, err)
			defer jasper.CloseHandle(event)

			assert.NoError(t, client.SignalEvent(ctx, eventName))
		},
		// "": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.LongTestTimeout)
			defer cancel()

			srv, port, err := startRESTService(ctx, httpClient)
			require.NoError(t, err)
			require.NotNil(t, srv)

			client := &restClient{
				prefix: fmt.Sprintf("http://localhost:%d/jasper/v1", port),
				client: httpClient,
			}

			testCase(ctx, t, srv, client)
		})
	}
}

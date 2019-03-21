package rpc

import (
	"context"
	"syscall"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpc "google.golang.org/grpc"
)

func TestWindowsRPCService(t *testing.T) {
	for managerName, makeManager := range map[string]func(trackProcs bool) (jasper.Manager, error){
		"Basic":    jasper.NewLocalManager,
		"Blocking": jasper.NewLocalManagerBlockingProcesses,
	} {
		t.Run(managerName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, internal.JasperProcessManagerClient){
				"SignalEventFailsWithNonexistentEvent": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					outcome, err := client.SignalEvent(ctx, &internal.EventName{Value: "foo"})
					require.Nil(t, err)
					assert.False(t, outcome.Success)
				},
				"SignalEventWithExistingEvent": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					eventName := "ThisIsARealEvent"
					utf16EventName, err := syscall.UTF16PtrFromString(eventName)
					require.NoError(t, err)

					event, err := jasper.CreateEvent(utf16EventName)
					require.NoError(t, err)
					defer jasper.CloseHandle(event)

					outcome, err := client.SignalEvent(ctx, &internal.EventName{Value: eventName})
					require.Nil(t, err)
					assert.True(t, outcome.Success)
				},
				// "": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
					defer cancel()

					manager, err := makeManager(false)
					require.NoError(t, err)
					addr, err := startRPC(ctx, manager)
					require.NoError(t, err)

					conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithBlock())
					require.NoError(t, err)
					client := internal.NewJasperProcessManagerClient(conn)

					go func() {
						<-ctx.Done()
						conn.Close()
					}()

					testCase(ctx, t, client)
				})
			}
		})
	}
}

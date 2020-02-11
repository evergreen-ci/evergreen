package remote

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/remote/internal"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestRPCService(t *testing.T) {
	for managerName, makeManager := range map[string]func() (jasper.Manager, error){
		"Basic": func() (jasper.Manager, error) { return jasper.NewSynchronizedManager(false) },
	} {
		t.Run(managerName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, internal.JasperProcessManagerClient){
				"RegisterSignalTriggerIDChecksForExistingProcess": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					outcome, err := client.RegisterSignalTriggerID(ctx, internal.ConvertSignalTriggerParams("foo", jasper.CleanTerminationSignalTrigger))
					require.NoError(t, err)
					assert.False(t, outcome.Success)
				},
				//"": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					manager, err := makeManager()
					require.NoError(t, err)
					addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", testutil.GetPortNumber()))
					require.NoError(t, err)
					require.NoError(t, startTestRPCService(ctx, manager, addr, nil))

					conn, err := grpc.DialContext(ctx, addr.String(), grpc.WithInsecure(), grpc.WithBlock())
					require.NoError(t, err)
					client := internal.NewJasperProcessManagerClient(conn)

					go func() {
						<-ctx.Done()
						assert.NoError(t, conn.Close())
					}()

					testCase(ctx, t, client)
				})
			}
		})
	}
}

package remote

import (
	"context"
	"fmt"
	"net"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
)

func makeTestMDBServiceAndClient(ctx context.Context, mngr jasper.Manager) (Manager, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", testutil.GetPortNumber()))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	closeService, err := StartMDBService(ctx, mngr, addr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	go func() {
		<-ctx.Done()
		grip.Notice(closeService())
	}()
	if err = testutil.WaitForWireService(ctx, addr); err != nil {
		return nil, errors.WithStack(err)
	}

	client, err := NewMDBClient(ctx, addr, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	go func() {
		<-ctx.Done()
		grip.Notice(client.CloseConnection())
	}()
	return client, nil
}

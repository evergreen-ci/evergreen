package wire

import (
	"context"
	"fmt"
	"net"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
)

func makeTestServiceAndClient(ctx context.Context, mngr jasper.Manager) (jasper.RemoteClient, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", testutil.GetPortNumber()))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	closeService, err := StartService(ctx, mngr, addr)
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

	client, err := NewClient(ctx, addr, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	go func() {
		<-ctx.Done()
		grip.Notice(client.CloseConnection())
	}()
	return client, nil
}

func createProcs(ctx context.Context, opts *options.Create, manager jasper.Manager, num int) ([]jasper.Process, error) {
	catcher := grip.NewBasicCatcher()
	out := []jasper.Process{}
	for i := 0; i < num; i++ {
		optsCopy := *opts

		proc, err := manager.CreateProcess(ctx, &optsCopy)
		catcher.Add(err)
		if proc != nil {
			out = append(out, proc)
		}
	}

	return out, catcher.Resolve()
}

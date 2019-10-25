package rpc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/certdepot"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// startTestService creates a server for testing purposes that terminates when
// the context is done.
func startTestService(ctx context.Context, mngr jasper.Manager, addr net.Addr, creds *certdepot.Credentials) error {
	closeService, err := StartService(ctx, mngr, addr, creds)
	if err != nil {
		return errors.Wrap(err, "could not start server")
	}

	go func() {
		<-ctx.Done()
		grip.Error(closeService())
	}()

	return nil
}

// newTestClient establishes a client for testing purposes that closes when
// the context is done.
func newTestClient(ctx context.Context, addr net.Addr, creds *certdepot.Credentials) (jasper.RemoteClient, error) {
	client, err := NewClient(ctx, addr, creds)
	if err != nil {
		return nil, errors.Wrap(err, "could not get client")
	}

	go func() {
		<-ctx.Done()
		grip.Notice(client.CloseConnection())
	}()

	return client, nil
}

// buildDir gets the Jasper build directory.
func buildDir(t *testing.T) string {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(filepath.Dir(cwd), "build")
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

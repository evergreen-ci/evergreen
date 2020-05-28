package remote

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/certdepot"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func makeInsecureRPCServiceAndClient(ctx context.Context, mngr jasper.Manager) (Manager, error) {
	addr, err := tryStartRPCService(ctx, func(ctx context.Context, addr net.Addr) error {
		return startTestRPCService(ctx, mngr, addr, nil)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to start RPC service")
	}
	return newTestRPCClient(ctx, addr, nil)
}

func tryStartRPCService(ctx context.Context, startService func(context.Context, net.Addr) error) (net.Addr, error) {
	var addr net.Addr
	var err error
tryPort:
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break tryPort
		default:
			addr, err = net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", testutil.GetPortNumber()))
			if err != nil {
				continue
			}

			if err = startService(ctx, addr); err != nil {
				continue
			}

			break tryPort
		}
	}
	return addr, err
}

func makeTLSRPCServiceAndClient(ctx context.Context, mngr jasper.Manager) (Manager, error) {
	caCertFile := filepath.Join("testdata", "ca.crt")

	serverCertFile := filepath.Join("testdata", "server.crt")
	serverKeyFile := filepath.Join("testdata", "server.key")

	clientCertFile := filepath.Join("testdata", "client.crt")
	clientKeyFile := filepath.Join("testdata", "client.key")

	// Make CA credentials
	caCert, err := ioutil.ReadFile(caCertFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cert file")
	}

	// Make server credentials
	serverCert, err := ioutil.ReadFile(serverCertFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cert file")
	}
	serverKey, err := ioutil.ReadFile(serverKeyFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read key file")
	}
	serverCreds, err := certdepot.NewCredentials(caCert, serverCert, serverKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize test server credentials")
	}

	addr, err := tryStartRPCService(ctx, func(ctx context.Context, addr net.Addr) error {
		return startTestRPCService(ctx, mngr, addr, serverCreds)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to start RPC service")
	}

	clientCert, err := ioutil.ReadFile(clientCertFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cert file")
	}
	clientKey, err := ioutil.ReadFile(clientKeyFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read key file")
	}
	clientCreds, err := certdepot.NewCredentials(caCert, clientCert, clientKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize test client credentials")
	}

	return newTestRPCClient(ctx, addr, clientCreds)
}

// startTestService creates a server for testing purposes that terminates when
// the context is done.
func startTestRPCService(ctx context.Context, mngr jasper.Manager, addr net.Addr, creds *certdepot.Credentials) error {
	closeService, err := StartRPCService(ctx, mngr, addr, creds)
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
func newTestRPCClient(ctx context.Context, addr net.Addr, creds *certdepot.Credentials) (Manager, error) {
	client, err := NewRPCClient(ctx, addr, creds)
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

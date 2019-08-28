package rpc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var intSource <-chan int

func init() {
	intSource = func() <-chan int {
		out := make(chan int, 25)
		go func() {
			id := 5000
			for {
				id++
				out <- id
			}
		}()
		return out
	}()
}

const (
	taskTimeout = 30 * time.Second
)

func getPortNumber() int {
	return <-intSource
}

func trueCreateOpts() *jasper.CreateOptions {
	return &jasper.CreateOptions{
		Args: []string{"true"},
	}
}

func falseCreateOpts() *jasper.CreateOptions {
	return &jasper.CreateOptions{
		Args: []string{"false"},
	}
}

func sleepCreateOpts(num int) *jasper.CreateOptions {
	return &jasper.CreateOptions{
		Args: []string{"sleep", fmt.Sprint(num)},
	}
}

func createProcs(ctx context.Context, opts *jasper.CreateOptions, manager jasper.Manager, num int) ([]jasper.Process, error) {
	catcher := grip.NewBasicCatcher()
	out := []jasper.Process{}
	for i := 0; i < num; i++ {
		proc, err := manager.CreateProcess(ctx, opts)
		catcher.Add(err)
		if proc != nil {
			out = append(out, proc)
		}
	}

	return out, catcher.Resolve()
}

// startTestService creates a server for testing purposes that terminates when
// the context is done.
func startTestService(ctx context.Context, mngr jasper.Manager, addr net.Addr, creds *Credentials) error {
	closeService, err := StartService(ctx, mngr, addr, creds)
	if err != nil {
		return errors.Wrap(err, "could not start server")
	}

	go func() {
		<-ctx.Done()
		closeService()
	}()

	return nil
}

// newTestClient establishes a client for testing purposes that closes when
// the context is done.
func newTestClient(ctx context.Context, addr net.Addr, creds *Credentials) (jasper.RemoteClient, error) {
	client, err := NewClient(ctx, addr, creds)
	if err != nil {
		return nil, errors.Wrap(err, "could not get client")
	}

	go func() {
		<-ctx.Done()
		client.CloseConnection()
	}()

	return client, nil
}

// buildDir gets the Jasper build directory.
func buildDir(t *testing.T) string {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(filepath.Dir(cwd), "build")
}

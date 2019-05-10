package rpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	grpc "google.golang.org/grpc"
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
	taskTimeout = 20 * time.Second
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

func startRPC(ctx context.Context, mngr jasper.Manager) (string, error) {
	addr := fmt.Sprintf("localhost:%d", getPortNumber())
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return "", errors.WithStack(err)
	}

	rpcSrv := grpc.NewServer()

	AttachService(mngr, rpcSrv)
	go rpcSrv.Serve(lis)

	go func() {
		<-ctx.Done()
		rpcSrv.Stop()
	}()

	return addr, nil
}

func getClient(ctx context.Context, addr string) (jasper.Manager, error) {
	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	return NewRPCManager(conn), nil

}

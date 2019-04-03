package rpc

import (
	"context"
	"net"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc/internal"
	"github.com/pkg/errors"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// AttachService attaches the jasper GRPC server to the given manager. After
// this function successfully returns, calls to Manager functions will be sent
// over GRPC to the Jasper GRPC server.
func AttachService(manager jasper.Manager, s *grpc.Server) error {
	return errors.WithStack(internal.AttachService(manager, s))
}

// CloseFunc is a function used to close the service or client.
type CloseFunc func() error

// StartServer starts an RPC server with the specified address. If certFile
// and keyFile are non-empty, the credentials will be read from the files to
// start a TLS service; otherwise it will start an insecure service. The caller
// is responsible for closing the connection using the returned CloseFunc.
func StartServer(ctx context.Context, manager jasper.Manager, address net.Addr, certFile string, keyFile string) (CloseFunc, error) {
	lis, err := net.Listen(address.Network(), address.String())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var service *grpc.Server
	if certFile != "" && keyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get server credentials from cert file '%s' and key file '%s'", certFile, keyFile)
		}
		service = grpc.NewServer(grpc.Creds(creds))
	} else {
		service = grpc.NewServer()
	}

	if err := AttachService(manager, service); err != nil {
		return nil, errors.Wrap(err, "could not attach manager to service")
	}
	go service.Serve(lis)

	return func() error { service.Stop(); return nil }, nil
}

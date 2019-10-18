package rpc

import (
	"context"
	"net"

	"github.com/evergreen-ci/aviation"
	"github.com/evergreen-ci/certdepot"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
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

// StartService starts an RPC server with the specified address addr around the
// given manager. If creds is non-nil, the credentials will be used to establish
// a secure TLS connection with clients; otherwise, it will start an insecure
// service. The caller is responsible for closing the connection using the
// return jasper.CloseFunc.
func StartService(ctx context.Context, manager jasper.Manager, addr net.Addr, creds *certdepot.Credentials) (jasper.CloseFunc, error) {
	lis, err := net.Listen(addr.Network(), addr.String())
	if err != nil {
		return nil, errors.Wrapf(err, "error listening on %s", addr.String())
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(aviation.MakeGripUnaryInterceptor(logging.MakeGrip(grip.GetSender()))),
		grpc.StreamInterceptor(aviation.MakeGripStreamInterceptor(logging.MakeGrip(grip.GetSender()))),
	}
	if creds != nil {
		tlsConf, err := creds.Resolve()
		if err != nil {
			return nil, errors.Wrap(err, "error generating TLS config from server credentials")
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConf)))
	}

	service := grpc.NewServer(opts...)

	if err := AttachService(manager, service); err != nil {
		return nil, errors.Wrap(err, "could not attach manager to service")
	}
	go service.Serve(lis)

	return func() error { service.Stop(); return nil }, nil
}

// StartServiceWithFile is the same as StartService, but the credentials will be
// read from the file given by filePath if the filePath is non-empty. The
// credentials file should contain the JSON-encoded bytes from
// (*Credentials).Export().
func StartServiceWithFile(ctx context.Context, manager jasper.Manager, addr net.Addr, filePath string) (jasper.CloseFunc, error) {
	var creds *certdepot.Credentials
	if filePath != "" {
		var err error
		creds, err = certdepot.NewCredentialsFromFile(filePath)
		if err != nil {
			return nil, errors.Wrap(err, "error getting credentials from file")
		}
	}

	return StartService(ctx, manager, addr, creds)
}

package rpc

import (
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc/internal"
	"github.com/pkg/errors"
	grpc "google.golang.org/grpc"
)

// AttachService attaches the jasper GRPC server to the given manager. After
// this function successfully returns, calls to Manager functions will be sent
// over GRPC to the Jasper GRPC server.
func AttachService(manager jasper.Manager, s *grpc.Server) error {
	return errors.WithStack(internal.AttachService(manager, s))
}

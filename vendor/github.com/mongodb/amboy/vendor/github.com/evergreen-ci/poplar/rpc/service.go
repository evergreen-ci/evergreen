package rpc

import (
	"github.com/evergreen-ci/poplar"
	"github.com/evergreen-ci/poplar/rpc/internal"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func AttachService(registry *poplar.RecorderRegistry, s *grpc.Server) error {
	return errors.WithStack(internal.AttachService(registry, s))
}

package aviation

import (
	"context"

	"github.com/evergreen-ci/gimlet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// Return a gRPC UnaryServerInterceptor which checks the certifcate user's
// validity against the given user manager.
func MakeCertificateUserValidationUnaryInterceptor(um gimlet.UserManager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		p, ok := peer.FromContext(ctx)
		if !ok {
			return nil, grpc.Errorf(codes.Unauthenticated, "missing peer info from context")
		}

		tlsAuth, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return nil, grpc.Errorf(codes.Unauthenticated, "unexpected peer transport credentials")
		}

		username := tlsAuth.State.VerifiedChains[0][0].Subject.CommonName

		usr, err := um.GetUserByID(username)
		if err != nil {
			return nil, grpc.Errorf(codes.Unauthenticated, "problem finding user: %+v", err)
		}

		if usr == nil {
			return nil, grpc.Errorf(codes.Unauthenticated, "user associated with certificate no longer valid!")
		}

		return handler(ctx, req)
	}
}

// Return a gRPC UnaryStreamInterceptor which checks the certifcate user's
// validity against the given user manager.
func MakeCertificateUserValidationStreamInterceptor(um gimlet.UserManager) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		ctx := stream.Context()

		p, ok := peer.FromContext(ctx)
		if !ok {
			return grpc.Errorf(codes.Unauthenticated, "missing peer info from context")
		}

		tlsAuth, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return grpc.Errorf(codes.Unauthenticated, "unexpected peer transport credentials")
		}
		username := tlsAuth.State.VerifiedChains[0][0].Subject.CommonName

		usr, err := um.GetUserByID(username)
		if err != nil {
			return grpc.Errorf(codes.Unauthenticated, "problem finding user: %+v", err)
		}

		if usr == nil {
			return grpc.Errorf(codes.Unauthenticated, "user associated with certificate no longer valid!")
		}

		return handler(srv, stream)
	}
}

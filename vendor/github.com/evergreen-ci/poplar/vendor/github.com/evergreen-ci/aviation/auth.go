package aviation

import (
	"context"
	"strings"

	"github.com/evergreen-ci/gimlet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

// TODO: add interceptors to provide "role required" and "group
// member" using gimlet.Authenticator

func MakeAuthenticationRequiredUnaryInterceptor(um gimlet.UserManager, conf gimlet.UserMiddlewareConfiguration, ignore ...string) grpc.UnaryServerInterceptor {
	ignoreMap := getIgnoreMap(ignore)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info == nil || !ignoreMap[info.FullMethod] {
			if err := checkUser(ctx, um, conf); err != nil {
				return nil, err
			}
		}

		return handler(ctx, req)
	}
}

func MakeAuthenticationRequiredStreamInterceptor(um gimlet.UserManager, conf gimlet.UserMiddlewareConfiguration, ignore ...string) grpc.StreamServerInterceptor {
	ignoreMap := getIgnoreMap(ignore)
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info == nil || !ignoreMap[info.FullMethod] {
			if err := checkUser(stream.Context(), um, conf); err != nil {
				return err
			}
		}

		return handler(srv, stream)
	}
}

func getIgnoreMap(ignore []string) map[string]bool {
	ignoreMap := map[string]bool{}
	for _, method := range ignore {
		ignoreMap[method] = true
	}

	return ignoreMap
}

func checkUser(ctx context.Context, um gimlet.UserManager, conf gimlet.UserMiddlewareConfiguration) error {
	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return grpc.Errorf(codes.Unauthenticated, "missing metadata from context")
	}

	var (
		authDataAPIKey string
		authDataName   string
	)

	// We need to set these to lower case since grpc sets all of
	// the context metadata keys to lowercase.
	headerKeyName := strings.ToLower(conf.HeaderKeyName)
	headerUserName := strings.ToLower(conf.HeaderUserName)

	// Grab API auth details from header.
	if len(meta[headerKeyName]) > 0 {
		authDataAPIKey = meta[headerKeyName][0]
	}
	if len(meta[headerUserName]) > 0 {
		authDataName = meta[headerUserName][0]
	}

	if len(authDataAPIKey) == 0 {
		return grpc.Errorf(codes.Unauthenticated, "user key not provided")
	}

	usr, err := um.GetUserByID(authDataName)
	if err != nil {
		return grpc.Errorf(codes.Unauthenticated, "finding user: %+v", err)
	}

	if usr == nil {
		return grpc.Errorf(codes.Unauthenticated, "user not found")
	}

	if usr.GetAPIKey() != authDataAPIKey {
		return grpc.Errorf(codes.Unauthenticated, "incorrect credentials")
	}

	return nil
}

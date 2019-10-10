package timber

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/aviation/services"
	"google.golang.org/grpc"
)

// DialCedarOptions describes the options for the DialCedar function. The base
// address defaults to `cedar.mongodb.com` and the RPC port to 7070. If a base
// address is provided the RPC port must also be provided. Username and
// password must always be provided. This aliases the same type in aviation in
// order to avoid users having to vendor aviation.
type DialCedarOptions services.DialCedarOptions

// DialCedar is a convenience function for creating a RPC client connection
// with cedar via gRPC. This wraps the same function in aviation in order to
// avoid users having to vendor aviation.
func DialCedar(ctx context.Context, client *http.Client, opts DialCedarOptions) (*grpc.ClientConn, error) {
	serviceOpts := services.DialCedarOptions(opts)
	return services.DialCedar(ctx, client, &serviceOpts)
}

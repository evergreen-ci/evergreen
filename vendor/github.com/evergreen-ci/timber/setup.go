package timber

import (
	"context"
	"errors"
	"net/http"

	"github.com/evergreen-ci/aviation/services"
	"google.golang.org/grpc"
)

// DialCedarOptions describes the options for the DialCedar function. The base
// address defaults to `cedar.mongodb.com` and the RPC port to 7070. If a base
// address is provided the RPC port must also be provided. Username and
// either password or API key must always be provided. This aliases the same
// type in aviation in order to avoid users having to vendor aviation.
type DialCedarOptions services.DialCedarOptions

// DialCedar is a convenience function for creating a RPC client connection
// with cedar via gRPC. This wraps the same function in aviation in order to
// avoid users having to vendor aviation.
func DialCedar(ctx context.Context, client *http.Client, opts DialCedarOptions) (*grpc.ClientConn, error) {
	serviceOpts := services.DialCedarOptions(opts)
	return services.DialCedar(ctx, client, &serviceOpts)
}

// ConnectionOptions contains the options needed to create a gRPC connection
// with cedar.
type ConnectionOptions struct {
	DialOpts DialCedarOptions
	Client   http.Client
}

func (opts ConnectionOptions) Validate() error {
	if (opts.DialOpts.BaseAddress == "" && opts.DialOpts.RPCPort != "") ||
		(opts.DialOpts.BaseAddress != "" && opts.DialOpts.RPCPort == "") {
		return errors.New("must provide both base address and rpc port or neither")
	}
	hasAuth := opts.DialOpts.Username != "" && (opts.DialOpts.APIKey != "" || opts.DialOpts.Password != "")
	if !hasAuth && opts.DialOpts.BaseAddress == "" {
		return errors.New("must specify username and api key/password, or address and port for an insecure connection")
	}
	return nil
}

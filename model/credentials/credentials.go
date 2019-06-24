package credentials

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/jasper/rpc"
)

// ForJasperClient returns the app server's credentials to authenticate with
// hosts running Jasper.
func ForJasperClient(ctx context.Context, env evergreen.Environment) (*rpc.Credentials, error) {
	return FindByID(ctx, env, serviceName)
}

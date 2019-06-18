package credentials

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/jasper/rpc"
)

// JasperClient returns the app server's credentials to authenticate with hosts
// running Jasper.
func JasperClient(ctx context.Context, env evergreen.Environment) (*rpc.Credentials, error) {
	if err := validateBootstrapped(); err != nil {
		return nil, err
	}
	return FindByID(ctx, env, serviceName)
}

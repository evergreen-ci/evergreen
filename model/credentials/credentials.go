package credentials

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
)

// ForJasperClient returns the app server's credentials to authenticate with
// hosts running Jasper.
func ForJasperClient(ctx context.Context, env evergreen.Environment) (*rpc.Credentials, error) {
	creds, err := FindByID(ctx, env, serviceName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return creds, nil
}

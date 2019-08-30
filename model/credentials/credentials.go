package credentials

import (
	"context"

	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
)

// ForJasperClient returns the app server's credentials to authenticate with
// hosts running Jasper.
func ForJasperClient(ctx context.Context) (*rpc.Credentials, error) {
	creds, err := FindByID(ctx, serviceName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return creds, nil
}

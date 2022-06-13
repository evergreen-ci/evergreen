package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// DisableQuery will return SERVICE_UNAVAILABLE for any query
// with an operation name listed in config.DisabledGQLQueries
type DisableQuery struct{}

func (DisableQuery) ExtensionName() string {
	return "DisableQuery"
}

func (DisableQuery) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (DisableQuery) MutateOperationContext(ctx context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	settings, err := evergreen.GetConfig()
	if err != nil {
		grip.Error(errors.Wrap(err, "getting Evergreen admin settings"))
	} else if utility.StringSliceContains(settings.DisabledGQLQueries, rc.Operation.Name) {
		return gqlError.ServiceUnavailable.Send(ctx, "Query is disabled by admin")
	}
	return nil
}

package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// SplunkTracing is a graphql extension that adds splunk logging to graphql.
// It is used to log the duration of a query and the user that made the request.
// It does this by hooking into lifecycle events that gqlgen uses.
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
	}
	if utility.StringSliceContains(settings.DisabledGQLQueries, rc.Operation.Name) {
		return &gqlerror.Error{
			Message: "Query is disabled by admin",
			Extensions: map[string]interface{}{
				"code": ServiceUnavailable,
			},
		}
	}
	return nil
}

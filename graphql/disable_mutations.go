package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// DisableMutations will return SERVICE_UNAVAILABLE for any mutation
type DisableMutations struct{}

func (DisableMutations) ExtensionName() string {
	return "DisableMutations"
}

func (DisableMutations) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (DisableMutations) MutateOperationContext(ctx context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	if rc.Operation.Operation == ast.Mutation {
		return ServiceUnavailable.Send(ctx, "Mutations are disabled")
	}
	return nil
}

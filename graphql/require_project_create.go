package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
)

// RequireProjectCreate restricts project creation to superusers and users with project create permission.
func RequireProjectCreate(ctx context.Context, _ any, next graphql.Resolver) (any, error) {
	// Allow if user is superuser.
	user := mustHaveUser(ctx)
	if userHasSuperuserProjectPermission(ctx, user) {
		return next(ctx)
	}

	canCreate, err := user.HasProjectCreatePermission(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("checking user permissions: %s", err.Error()))
	}
	if canCreate {
		return next(ctx)
	}

	return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to create projects", user.Username()))
}

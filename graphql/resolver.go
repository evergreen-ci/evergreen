package graphql

// This file will always be generated when running gqlgen.
// werrors imports in the resolvers are due to https://github.com/99designs/gqlgen/issues/1171.

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
)

type Resolver struct{}

func NewConfig() Config {
	c := Config{
		Resolvers: &Resolver{},
	}
	setComplexityFuncs(&c)
	c.Directives.RequirePatchOwner = RequirePatchOwner
	c.Directives.RequirePatchSubmitAccess = RequirePatchSubmitAccess
	c.Directives.RequireHostAccess = RequireHostAccess
	c.Directives.RequireDistroAccess = RequireDistroAccess
	c.Directives.RequireProjectCreate = RequireProjectCreate
	c.Directives.RequireRepoAccess = RequireRepoAccess
	c.Directives.RequireProjectAccess = RequireProjectAccess
	c.Directives.RequireProjectSettingsAccess = RequireProjectSettingsAccess
	c.Directives.RedactSecrets = func(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
		return next(ctx)
	}
	c.Directives.RequireAdmin = RequireAdmin
	c.Directives.RequireVolumeAccess = RequireVolumeAccess

	return c
}

package graphql

import (
	"context"
)

type Resolver struct{}

func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}

type queryResolver struct{ *Resolver }

func (r *queryResolver) Test(ctx context.Context) (string, error) {
	return "Hello World", nil
}

func New() Config {
	return Config{
		Resolvers: &Resolver{},
	}
}

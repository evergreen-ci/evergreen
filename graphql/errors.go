package graphql

import (
	"context"
	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/gqlerror"
)

type gqlError string

const (
	// InternalServerError conveys that the server errored out when trying to perform an action
	InternalServerError = "INTERNAL_SERVER_ERROR"
	// Forbidden conveys that user does not required permissions to access resource
	Forbidden = "FORBIDDEN"
	// ResourceNotFound conveys the requested resource does not exist
	ResourceNotFound = "RESOURCE_NOT_FOUND"
)

// Send sends a gql error to the client formatted with
func (err gqlError) Send(ctx context.Context, message string) *gqlerror.Error {
	switch err {
	case InternalServerError:
		return formError(ctx, message, InternalServerError)
	case Forbidden:
		return formError(ctx, message, Forbidden)
	case ResourceNotFound:
		return formError(ctx, message, ResourceNotFound)
	default:
		return gqlerror.ErrorPathf(graphql.GetResolverContext(ctx).Path(), message)
	}
}

func formError(ctx context.Context, message string, code gqlError) *gqlerror.Error {
	return gqlerror.ErrorPathf(graphql.GetResolverContext(ctx).Path(), message)
}

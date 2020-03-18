package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// GqlError represents the error codes send alongside gql errors
type GqlError string

const (
	// InternalServerError conveys that the server errored out when trying to perform an action
	InternalServerError GqlError = "INTERNAL_SERVER_ERROR"
	// Forbidden conveys that user does not required permissions to access resource
	Forbidden GqlError = "FORBIDDEN"
	// ResourceNotFound conveys the requested resource does not exist
	ResourceNotFound GqlError = "RESOURCE_NOT_FOUND"
)

// Send sends a gql error to the client formatted with
func (err GqlError) Send(ctx context.Context, message string) *gqlerror.Error {
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

func formError(ctx context.Context, message string, code GqlError) *gqlerror.Error {
	return &gqlerror.Error{
		Message: message,
		Extensions: map[string]interface{}{
			"code": code,
		},
	}
}

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
	// InputValidationError conveys that the given input is not formatted properly
	InputValidationError GqlError = "INPUT_VALIDATION_ERROR"
	ServiceUnavailable   GqlError = "SERVICE_UNAVAILABLE"
	// PartialError conveys that the request succeeded, but there were nonfatal errors that may be communicated to users
	PartialError GqlError = "PARTIAL_ERROR"
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
	case InputValidationError:
		return formError(ctx, message, InputValidationError)
	case ServiceUnavailable:
		return formError(ctx, message, ServiceUnavailable)
	case PartialError:
		return formError(ctx, message, PartialError)
	default:
		return gqlerror.ErrorPathf(graphql.GetFieldContext(ctx).Path(), message)
	}
}

func formError(_ context.Context, msg string, code GqlError) *gqlerror.Error {
	return &gqlerror.Error{
		Message: msg,
		Extensions: map[string]interface{}{
			"code": code,
		},
	}
}

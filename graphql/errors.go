package graphql

import (
	"context"
	"errors"

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

// Send sends a gql error to the client. If one or more causes are provided
// they are attached as the returned *gqlerror.Error's Unwrap target so
// errors.As / errors.Is inspection can reach the original error. In
// particular, the GraphQL error presenter uses loaders.IsBatchError (an
// errors.As check) to detect dataloader batch errors and skip re-logging them
// for every field that shares the failed batch; without a cause populated, a
// single batch failure is reported once per field that tries to read from the
// batch, producing N+1 duplicate logs.
func (err GqlError) Send(ctx context.Context, message string, causes ...error) *gqlerror.Error {
	var gqlErr *gqlerror.Error
	switch err {
	case InternalServerError:
		gqlErr = formError(ctx, message, InternalServerError)
	case Forbidden:
		gqlErr = formError(ctx, message, Forbidden)
	case ResourceNotFound:
		gqlErr = formError(ctx, message, ResourceNotFound)
	case InputValidationError:
		gqlErr = formError(ctx, message, InputValidationError)
	case ServiceUnavailable:
		gqlErr = formError(ctx, message, ServiceUnavailable)
	case PartialError:
		gqlErr = formError(ctx, message, PartialError)
	default:
		// The %s formatter is necessary in Go because formatting functions
		// cannot be passed a non-constant format string, and gqlgen doesn't
		// have a non-formatting variant of this function.
		// See https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf#hdr-Examples
		gqlErr = gqlerror.ErrorPathf(graphql.GetFieldContext(ctx).Path(), "%s", message)
	}
	switch len(causes) {
	case 0:
	case 1:
		gqlErr.Err = causes[0]
	default:
		gqlErr.Err = errors.Join(causes...)
	}
	return gqlErr
}

func formError(_ context.Context, msg string, code GqlError) *gqlerror.Error {
	return &gqlerror.Error{
		Message: msg,
		Extensions: map[string]any{
			"code": code,
		},
	}
}

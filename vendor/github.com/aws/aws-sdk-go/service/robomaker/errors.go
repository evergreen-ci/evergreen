// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package robomaker

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeConcurrentDeploymentException for service response error code
	// "ConcurrentDeploymentException".
	//
	// The failure percentage threshold percentage was met.
	ErrCodeConcurrentDeploymentException = "ConcurrentDeploymentException"

	// ErrCodeIdempotentParameterMismatchException for service response error code
	// "IdempotentParameterMismatchException".
	//
	// The request uses the same client token as a previous, but non-identical request.
	// Do not reuse a client token with different requests, unless the requests
	// are identical.
	ErrCodeIdempotentParameterMismatchException = "IdempotentParameterMismatchException"

	// ErrCodeInternalServerException for service response error code
	// "InternalServerException".
	//
	// AWS RoboMaker experienced a service issue. Try your call again.
	ErrCodeInternalServerException = "InternalServerException"

	// ErrCodeInvalidParameterException for service response error code
	// "InvalidParameterException".
	//
	// A parameter specified in a request is not valid, is unsupported, or cannot
	// be used. The returned message provides an explanation of the error value.
	ErrCodeInvalidParameterException = "InvalidParameterException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// The requested resource exceeds the maximum number allowed, or the number
	// of concurrent stream requests exceeds the maximum number allowed.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeResourceAlreadyExistsException for service response error code
	// "ResourceAlreadyExistsException".
	//
	// The specified resource already exists.
	ErrCodeResourceAlreadyExistsException = "ResourceAlreadyExistsException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// The specified resource does not exist.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeServiceUnavailableException for service response error code
	// "ServiceUnavailableException".
	//
	// The request has failed due to a temporary failure of the server.
	ErrCodeServiceUnavailableException = "ServiceUnavailableException"

	// ErrCodeThrottlingException for service response error code
	// "ThrottlingException".
	//
	// AWS RoboMaker is temporarily unable to process the request. Try your call
	// again.
	ErrCodeThrottlingException = "ThrottlingException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"ConcurrentDeploymentException":        newErrorConcurrentDeploymentException,
	"IdempotentParameterMismatchException": newErrorIdempotentParameterMismatchException,
	"InternalServerException":              newErrorInternalServerException,
	"InvalidParameterException":            newErrorInvalidParameterException,
	"LimitExceededException":               newErrorLimitExceededException,
	"ResourceAlreadyExistsException":       newErrorResourceAlreadyExistsException,
	"ResourceNotFoundException":            newErrorResourceNotFoundException,
	"ServiceUnavailableException":          newErrorServiceUnavailableException,
	"ThrottlingException":                  newErrorThrottlingException,
}

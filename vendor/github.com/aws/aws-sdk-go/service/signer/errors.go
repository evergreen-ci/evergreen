// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package signer

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeAccessDeniedException for service response error code
	// "AccessDeniedException".
	//
	// You do not have sufficient access to perform this action.
	ErrCodeAccessDeniedException = "AccessDeniedException"

	// ErrCodeBadRequestException for service response error code
	// "BadRequestException".
	//
	// The request contains invalid parameters for the ARN or tags. This exception
	// also occurs when you call a tagging API on a cancelled signing profile.
	ErrCodeBadRequestException = "BadRequestException"

	// ErrCodeConflictException for service response error code
	// "ConflictException".
	//
	// The resource encountered a conflicting state.
	ErrCodeConflictException = "ConflictException"

	// ErrCodeInternalServiceErrorException for service response error code
	// "InternalServiceErrorException".
	//
	// An internal error occurred.
	ErrCodeInternalServiceErrorException = "InternalServiceErrorException"

	// ErrCodeNotFoundException for service response error code
	// "NotFoundException".
	//
	// The signing profile was not found.
	ErrCodeNotFoundException = "NotFoundException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// A specified resource could not be found.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeServiceLimitExceededException for service response error code
	// "ServiceLimitExceededException".
	//
	// The client is making a request that exceeds service limits.
	ErrCodeServiceLimitExceededException = "ServiceLimitExceededException"

	// ErrCodeThrottlingException for service response error code
	// "ThrottlingException".
	//
	// The request was denied due to request throttling.
	//
	// Instead of this error, TooManyRequestsException should be used.
	ErrCodeThrottlingException = "ThrottlingException"

	// ErrCodeTooManyRequestsException for service response error code
	// "TooManyRequestsException".
	//
	// The allowed number of job-signing requests has been exceeded.
	//
	// This error supersedes the error ThrottlingException.
	ErrCodeTooManyRequestsException = "TooManyRequestsException"

	// ErrCodeValidationException for service response error code
	// "ValidationException".
	//
	// You signing certificate could not be validated.
	ErrCodeValidationException = "ValidationException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"AccessDeniedException":         newErrorAccessDeniedException,
	"BadRequestException":           newErrorBadRequestException,
	"ConflictException":             newErrorConflictException,
	"InternalServiceErrorException": newErrorInternalServiceErrorException,
	"NotFoundException":             newErrorNotFoundException,
	"ResourceNotFoundException":     newErrorResourceNotFoundException,
	"ServiceLimitExceededException": newErrorServiceLimitExceededException,
	"ThrottlingException":           newErrorThrottlingException,
	"TooManyRequestsException":      newErrorTooManyRequestsException,
	"ValidationException":           newErrorValidationException,
}

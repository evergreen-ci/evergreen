// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package lexmodelsv2

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeConflictException for service response error code
	// "ConflictException".
	ErrCodeConflictException = "ConflictException"

	// ErrCodeInternalServerException for service response error code
	// "InternalServerException".
	ErrCodeInternalServerException = "InternalServerException"

	// ErrCodePreconditionFailedException for service response error code
	// "PreconditionFailedException".
	ErrCodePreconditionFailedException = "PreconditionFailedException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeServiceQuotaExceededException for service response error code
	// "ServiceQuotaExceededException".
	ErrCodeServiceQuotaExceededException = "ServiceQuotaExceededException"

	// ErrCodeThrottlingException for service response error code
	// "ThrottlingException".
	ErrCodeThrottlingException = "ThrottlingException"

	// ErrCodeValidationException for service response error code
	// "ValidationException".
	ErrCodeValidationException = "ValidationException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"ConflictException":             newErrorConflictException,
	"InternalServerException":       newErrorInternalServerException,
	"PreconditionFailedException":   newErrorPreconditionFailedException,
	"ResourceNotFoundException":     newErrorResourceNotFoundException,
	"ServiceQuotaExceededException": newErrorServiceQuotaExceededException,
	"ThrottlingException":           newErrorThrottlingException,
	"ValidationException":           newErrorValidationException,
}

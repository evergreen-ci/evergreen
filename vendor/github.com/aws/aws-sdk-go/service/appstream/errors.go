// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package appstream

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeConcurrentModificationException for service response error code
	// "ConcurrentModificationException".
	//
	// An API error occurred. Wait a few minutes and try again.
	ErrCodeConcurrentModificationException = "ConcurrentModificationException"

	// ErrCodeIncompatibleImageException for service response error code
	// "IncompatibleImageException".
	//
	// The image can't be updated because it's not compatible for updates.
	ErrCodeIncompatibleImageException = "IncompatibleImageException"

	// ErrCodeInvalidAccountStatusException for service response error code
	// "InvalidAccountStatusException".
	//
	// The resource cannot be created because your AWS account is suspended. For
	// assistance, contact AWS Support.
	ErrCodeInvalidAccountStatusException = "InvalidAccountStatusException"

	// ErrCodeInvalidParameterCombinationException for service response error code
	// "InvalidParameterCombinationException".
	//
	// Indicates an incorrect combination of parameters, or a missing parameter.
	ErrCodeInvalidParameterCombinationException = "InvalidParameterCombinationException"

	// ErrCodeInvalidRoleException for service response error code
	// "InvalidRoleException".
	//
	// The specified role is invalid.
	ErrCodeInvalidRoleException = "InvalidRoleException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// The requested limit exceeds the permitted limit for an account.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeOperationNotPermittedException for service response error code
	// "OperationNotPermittedException".
	//
	// The attempted operation is not permitted.
	ErrCodeOperationNotPermittedException = "OperationNotPermittedException"

	// ErrCodeRequestLimitExceededException for service response error code
	// "RequestLimitExceededException".
	//
	// AppStream 2.0 can’t process the request right now because the Describe
	// calls from your AWS account are being throttled by Amazon EC2. Try again
	// later.
	ErrCodeRequestLimitExceededException = "RequestLimitExceededException"

	// ErrCodeResourceAlreadyExistsException for service response error code
	// "ResourceAlreadyExistsException".
	//
	// The specified resource already exists.
	ErrCodeResourceAlreadyExistsException = "ResourceAlreadyExistsException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// The specified resource is in use.
	ErrCodeResourceInUseException = "ResourceInUseException"

	// ErrCodeResourceNotAvailableException for service response error code
	// "ResourceNotAvailableException".
	//
	// The specified resource exists and is not in use, but isn't available.
	ErrCodeResourceNotAvailableException = "ResourceNotAvailableException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// The specified resource was not found.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"ConcurrentModificationException":      newErrorConcurrentModificationException,
	"IncompatibleImageException":           newErrorIncompatibleImageException,
	"InvalidAccountStatusException":        newErrorInvalidAccountStatusException,
	"InvalidParameterCombinationException": newErrorInvalidParameterCombinationException,
	"InvalidRoleException":                 newErrorInvalidRoleException,
	"LimitExceededException":               newErrorLimitExceededException,
	"OperationNotPermittedException":       newErrorOperationNotPermittedException,
	"RequestLimitExceededException":        newErrorRequestLimitExceededException,
	"ResourceAlreadyExistsException":       newErrorResourceAlreadyExistsException,
	"ResourceInUseException":               newErrorResourceInUseException,
	"ResourceNotAvailableException":        newErrorResourceNotAvailableException,
	"ResourceNotFoundException":            newErrorResourceNotFoundException,
}

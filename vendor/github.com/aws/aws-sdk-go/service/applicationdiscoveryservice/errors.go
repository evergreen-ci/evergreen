// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package applicationdiscoveryservice

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeAuthorizationErrorException for service response error code
	// "AuthorizationErrorException".
	//
	// The AWS user account does not have permission to perform the action. Check
	// the IAM policy associated with this account.
	ErrCodeAuthorizationErrorException = "AuthorizationErrorException"

	// ErrCodeConflictErrorException for service response error code
	// "ConflictErrorException".
	ErrCodeConflictErrorException = "ConflictErrorException"

	// ErrCodeHomeRegionNotSetException for service response error code
	// "HomeRegionNotSetException".
	//
	// The home region is not set. Set the home region to continue.
	ErrCodeHomeRegionNotSetException = "HomeRegionNotSetException"

	// ErrCodeInvalidParameterException for service response error code
	// "InvalidParameterException".
	//
	// One or more parameters are not valid. Verify the parameters and try again.
	ErrCodeInvalidParameterException = "InvalidParameterException"

	// ErrCodeInvalidParameterValueException for service response error code
	// "InvalidParameterValueException".
	//
	// The value of one or more parameters are either invalid or out of range. Verify
	// the parameter values and try again.
	ErrCodeInvalidParameterValueException = "InvalidParameterValueException"

	// ErrCodeOperationNotPermittedException for service response error code
	// "OperationNotPermittedException".
	//
	// This operation is not permitted.
	ErrCodeOperationNotPermittedException = "OperationNotPermittedException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// This issue occurs when the same clientRequestToken is used with the StartImportTask
	// action, but with different parameters. For example, you use the same request
	// token but have two different import URLs, you can encounter this issue. If
	// the import tasks are meant to be different, use a different clientRequestToken,
	// and try again.
	ErrCodeResourceInUseException = "ResourceInUseException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// The specified configuration ID was not located. Verify the configuration
	// ID and try again.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeServerInternalErrorException for service response error code
	// "ServerInternalErrorException".
	//
	// The server experienced an internal error. Try again.
	ErrCodeServerInternalErrorException = "ServerInternalErrorException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"AuthorizationErrorException":    newErrorAuthorizationErrorException,
	"ConflictErrorException":         newErrorConflictErrorException,
	"HomeRegionNotSetException":      newErrorHomeRegionNotSetException,
	"InvalidParameterException":      newErrorInvalidParameterException,
	"InvalidParameterValueException": newErrorInvalidParameterValueException,
	"OperationNotPermittedException": newErrorOperationNotPermittedException,
	"ResourceInUseException":         newErrorResourceInUseException,
	"ResourceNotFoundException":      newErrorResourceNotFoundException,
	"ServerInternalErrorException":   newErrorServerInternalErrorException,
}

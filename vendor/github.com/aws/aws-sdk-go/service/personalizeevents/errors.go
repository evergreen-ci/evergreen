// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package personalizeevents

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeInvalidInputException for service response error code
	// "InvalidInputException".
	//
	// Provide a valid value for the field or parameter.
	ErrCodeInvalidInputException = "InvalidInputException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// The specified resource is in use.
	ErrCodeResourceInUseException = "ResourceInUseException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// Could not find the specified resource.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"InvalidInputException":     newErrorInvalidInputException,
	"ResourceInUseException":    newErrorResourceInUseException,
	"ResourceNotFoundException": newErrorResourceNotFoundException,
}

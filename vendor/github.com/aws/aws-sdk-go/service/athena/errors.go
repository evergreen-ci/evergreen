// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package athena

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeInternalServerException for service response error code
	// "InternalServerException".
	//
	// Indicates a platform issue, which may be due to a transient condition or
	// outage.
	ErrCodeInternalServerException = "InternalServerException"

	// ErrCodeInvalidRequestException for service response error code
	// "InvalidRequestException".
	//
	// Indicates that something is wrong with the input to the request. For example,
	// a required parameter may be missing or out of range.
	ErrCodeInvalidRequestException = "InvalidRequestException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// A resource, such as a workgroup, was not found.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeTooManyRequestsException for service response error code
	// "TooManyRequestsException".
	//
	// Indicates that the request was throttled.
	ErrCodeTooManyRequestsException = "TooManyRequestsException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"InternalServerException":   newErrorInternalServerException,
	"InvalidRequestException":   newErrorInvalidRequestException,
	"ResourceNotFoundException": newErrorResourceNotFoundException,
	"TooManyRequestsException":  newErrorTooManyRequestsException,
}

// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package lexmodelbuildingservice

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeBadRequestException for service response error code
	// "BadRequestException".
	//
	// The request is not well formed. For example, a value is invalid or a required
	// field is missing. Check the field values, and try again.
	ErrCodeBadRequestException = "BadRequestException"

	// ErrCodeConflictException for service response error code
	// "ConflictException".
	//
	// There was a conflict processing the request. Try your request again.
	ErrCodeConflictException = "ConflictException"

	// ErrCodeInternalFailureException for service response error code
	// "InternalFailureException".
	//
	// An internal Amazon Lex error occurred. Try your request again.
	ErrCodeInternalFailureException = "InternalFailureException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// The request exceeded a limit. Try your request again.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeNotFoundException for service response error code
	// "NotFoundException".
	//
	// The resource specified in the request was not found. Check the resource and
	// try again.
	ErrCodeNotFoundException = "NotFoundException"

	// ErrCodePreconditionFailedException for service response error code
	// "PreconditionFailedException".
	//
	// The checksum of the resource that you are trying to change does not match
	// the checksum in the request. Check the resource's checksum and try again.
	ErrCodePreconditionFailedException = "PreconditionFailedException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// The resource that you are attempting to delete is referred to by another
	// resource. Use this information to remove references to the resource that
	// you are trying to delete.
	//
	// The body of the exception contains a JSON object that describes the resource.
	//
	// { "resourceType": BOT | BOTALIAS | BOTCHANNEL | INTENT,
	//
	// "resourceReference": {
	//
	// "name": string, "version": string } }
	ErrCodeResourceInUseException = "ResourceInUseException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"BadRequestException":         newErrorBadRequestException,
	"ConflictException":           newErrorConflictException,
	"InternalFailureException":    newErrorInternalFailureException,
	"LimitExceededException":      newErrorLimitExceededException,
	"NotFoundException":           newErrorNotFoundException,
	"PreconditionFailedException": newErrorPreconditionFailedException,
	"ResourceInUseException":      newErrorResourceInUseException,
}

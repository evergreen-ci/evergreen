// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package mediapackage

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeForbiddenException for service response error code
	// "ForbiddenException".
	ErrCodeForbiddenException = "ForbiddenException"

	// ErrCodeInternalServerErrorException for service response error code
	// "InternalServerErrorException".
	ErrCodeInternalServerErrorException = "InternalServerErrorException"

	// ErrCodeNotFoundException for service response error code
	// "NotFoundException".
	ErrCodeNotFoundException = "NotFoundException"

	// ErrCodeServiceUnavailableException for service response error code
	// "ServiceUnavailableException".
	ErrCodeServiceUnavailableException = "ServiceUnavailableException"

	// ErrCodeTooManyRequestsException for service response error code
	// "TooManyRequestsException".
	ErrCodeTooManyRequestsException = "TooManyRequestsException"

	// ErrCodeUnprocessableEntityException for service response error code
	// "UnprocessableEntityException".
	ErrCodeUnprocessableEntityException = "UnprocessableEntityException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"ForbiddenException":           newErrorForbiddenException,
	"InternalServerErrorException": newErrorInternalServerErrorException,
	"NotFoundException":            newErrorNotFoundException,
	"ServiceUnavailableException":  newErrorServiceUnavailableException,
	"TooManyRequestsException":     newErrorTooManyRequestsException,
	"UnprocessableEntityException": newErrorUnprocessableEntityException,
}

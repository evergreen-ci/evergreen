// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package schemas

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeBadRequestException for service response error code
	// "BadRequestException".
	ErrCodeBadRequestException = "BadRequestException"

	// ErrCodeConflictException for service response error code
	// "ConflictException".
	ErrCodeConflictException = "ConflictException"

	// ErrCodeForbiddenException for service response error code
	// "ForbiddenException".
	ErrCodeForbiddenException = "ForbiddenException"

	// ErrCodeGoneException for service response error code
	// "GoneException".
	ErrCodeGoneException = "GoneException"

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

	// ErrCodeUnauthorizedException for service response error code
	// "UnauthorizedException".
	ErrCodeUnauthorizedException = "UnauthorizedException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"BadRequestException":          newErrorBadRequestException,
	"ConflictException":            newErrorConflictException,
	"ForbiddenException":           newErrorForbiddenException,
	"GoneException":                newErrorGoneException,
	"InternalServerErrorException": newErrorInternalServerErrorException,
	"NotFoundException":            newErrorNotFoundException,
	"ServiceUnavailableException":  newErrorServiceUnavailableException,
	"TooManyRequestsException":     newErrorTooManyRequestsException,
	"UnauthorizedException":        newErrorUnauthorizedException,
}

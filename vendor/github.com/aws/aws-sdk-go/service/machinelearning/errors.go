// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package machinelearning

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeIdempotentParameterMismatchException for service response error code
	// "IdempotentParameterMismatchException".
	//
	// A second request to use or change an object was not allowed. This can result
	// from retrying a request using a parameter that was not present in the original
	// request.
	ErrCodeIdempotentParameterMismatchException = "IdempotentParameterMismatchException"

	// ErrCodeInternalServerException for service response error code
	// "InternalServerException".
	//
	// An error on the server occurred when trying to process a request.
	ErrCodeInternalServerException = "InternalServerException"

	// ErrCodeInvalidInputException for service response error code
	// "InvalidInputException".
	//
	// An error on the client occurred. Typically, the cause is an invalid input
	// value.
	ErrCodeInvalidInputException = "InvalidInputException"

	// ErrCodeInvalidTagException for service response error code
	// "InvalidTagException".
	ErrCodeInvalidTagException = "InvalidTagException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// The subscriber exceeded the maximum number of operations. This exception
	// can occur when listing objects such as DataSource.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodePredictorNotMountedException for service response error code
	// "PredictorNotMountedException".
	//
	// The exception is thrown when a predict request is made to an unmounted MLModel.
	ErrCodePredictorNotMountedException = "PredictorNotMountedException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// A specified resource cannot be located.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeTagLimitExceededException for service response error code
	// "TagLimitExceededException".
	ErrCodeTagLimitExceededException = "TagLimitExceededException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"IdempotentParameterMismatchException": newErrorIdempotentParameterMismatchException,
	"InternalServerException":              newErrorInternalServerException,
	"InvalidInputException":                newErrorInvalidInputException,
	"InvalidTagException":                  newErrorInvalidTagException,
	"LimitExceededException":               newErrorLimitExceededException,
	"PredictorNotMountedException":         newErrorPredictorNotMountedException,
	"ResourceNotFoundException":            newErrorResourceNotFoundException,
	"TagLimitExceededException":            newErrorTagLimitExceededException,
}

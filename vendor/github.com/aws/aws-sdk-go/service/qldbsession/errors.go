// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package qldbsession

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeBadRequestException for service response error code
	// "BadRequestException".
	//
	// Returned if the request is malformed or contains an error such as an invalid
	// parameter value or a missing required parameter.
	ErrCodeBadRequestException = "BadRequestException"

	// ErrCodeCapacityExceededException for service response error code
	// "CapacityExceededException".
	//
	// Returned when the request exceeds the processing capacity of the ledger.
	ErrCodeCapacityExceededException = "CapacityExceededException"

	// ErrCodeInvalidSessionException for service response error code
	// "InvalidSessionException".
	//
	// Returned if the session doesn't exist anymore because it timed out or expired.
	ErrCodeInvalidSessionException = "InvalidSessionException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// Returned if a resource limit such as number of active sessions is exceeded.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeOccConflictException for service response error code
	// "OccConflictException".
	//
	// Returned when a transaction cannot be written to the journal due to a failure
	// in the verification phase of optimistic concurrency control (OCC).
	ErrCodeOccConflictException = "OccConflictException"

	// ErrCodeRateExceededException for service response error code
	// "RateExceededException".
	//
	// Returned when the rate of requests exceeds the allowed throughput.
	ErrCodeRateExceededException = "RateExceededException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"BadRequestException":       newErrorBadRequestException,
	"CapacityExceededException": newErrorCapacityExceededException,
	"InvalidSessionException":   newErrorInvalidSessionException,
	"LimitExceededException":    newErrorLimitExceededException,
	"OccConflictException":      newErrorOccConflictException,
	"RateExceededException":     newErrorRateExceededException,
}

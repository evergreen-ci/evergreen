// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package timestreamwrite

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeAccessDeniedException for service response error code
	// "AccessDeniedException".
	//
	// You are not authorized to perform this action.
	ErrCodeAccessDeniedException = "AccessDeniedException"

	// ErrCodeConflictException for service response error code
	// "ConflictException".
	//
	// Timestream was unable to process this request because it contains resource
	// that already exists.
	ErrCodeConflictException = "ConflictException"

	// ErrCodeInternalServerException for service response error code
	// "InternalServerException".
	//
	// Timestream was unable to fully process this request because of an internal
	// server error.
	ErrCodeInternalServerException = "InternalServerException"

	// ErrCodeInvalidEndpointException for service response error code
	// "InvalidEndpointException".
	//
	// The requested endpoint was invalid.
	ErrCodeInvalidEndpointException = "InvalidEndpointException"

	// ErrCodeRejectedRecordsException for service response error code
	// "RejectedRecordsException".
	//
	// WriteRecords would throw this exception in the following cases:
	//
	//    * Records with duplicate data where there are multiple records with the
	//    same dimensions, timestamps, and measure names but different measure values.
	//
	//    * Records with timestamps that lie outside the retention duration of the
	//    memory store
	//
	//    * Records with dimensions or measures that exceed the Timestream defined
	//    limits.
	//
	// For more information, see Access Management (https://docs.aws.amazon.com/timestream/latest/developerguide/ts-limits.html)
	// in the Timestream Developer Guide.
	ErrCodeRejectedRecordsException = "RejectedRecordsException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// The operation tried to access a nonexistent resource. The resource might
	// not be specified correctly, or its status might not be ACTIVE.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeServiceQuotaExceededException for service response error code
	// "ServiceQuotaExceededException".
	//
	// Instance quota of resource exceeded for this account.
	ErrCodeServiceQuotaExceededException = "ServiceQuotaExceededException"

	// ErrCodeThrottlingException for service response error code
	// "ThrottlingException".
	//
	// Too many requests were made by a user exceeding service quotas. The request
	// was throttled.
	ErrCodeThrottlingException = "ThrottlingException"

	// ErrCodeValidationException for service response error code
	// "ValidationException".
	//
	// Invalid or malformed request.
	ErrCodeValidationException = "ValidationException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"AccessDeniedException":         newErrorAccessDeniedException,
	"ConflictException":             newErrorConflictException,
	"InternalServerException":       newErrorInternalServerException,
	"InvalidEndpointException":      newErrorInvalidEndpointException,
	"RejectedRecordsException":      newErrorRejectedRecordsException,
	"ResourceNotFoundException":     newErrorResourceNotFoundException,
	"ServiceQuotaExceededException": newErrorServiceQuotaExceededException,
	"ThrottlingException":           newErrorThrottlingException,
	"ValidationException":           newErrorValidationException,
}

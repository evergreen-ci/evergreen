// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package kinesisanalytics

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeCodeValidationException for service response error code
	// "CodeValidationException".
	//
	// User-provided application code (query) is invalid. This can be a simple syntax
	// error.
	ErrCodeCodeValidationException = "CodeValidationException"

	// ErrCodeConcurrentModificationException for service response error code
	// "ConcurrentModificationException".
	//
	// Exception thrown as a result of concurrent modification to an application.
	// For example, two individuals attempting to edit the same application at the
	// same time.
	ErrCodeConcurrentModificationException = "ConcurrentModificationException"

	// ErrCodeInvalidApplicationConfigurationException for service response error code
	// "InvalidApplicationConfigurationException".
	//
	// User-provided application configuration is not valid.
	ErrCodeInvalidApplicationConfigurationException = "InvalidApplicationConfigurationException"

	// ErrCodeInvalidArgumentException for service response error code
	// "InvalidArgumentException".
	//
	// Specified input parameter value is invalid.
	ErrCodeInvalidArgumentException = "InvalidArgumentException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// Exceeded the number of applications allowed.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// Application is not available for this operation.
	ErrCodeResourceInUseException = "ResourceInUseException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// Specified application can't be found.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeResourceProvisionedThroughputExceededException for service response error code
	// "ResourceProvisionedThroughputExceededException".
	//
	// Discovery failed to get a record from the streaming source because of the
	// Amazon Kinesis Streams ProvisionedThroughputExceededException. For more information,
	// see GetRecords (https://docs.aws.amazon.com/kinesis/latest/APIReference/API_GetRecords.html)
	// in the Amazon Kinesis Streams API Reference.
	ErrCodeResourceProvisionedThroughputExceededException = "ResourceProvisionedThroughputExceededException"

	// ErrCodeServiceUnavailableException for service response error code
	// "ServiceUnavailableException".
	//
	// The service is unavailable. Back off and retry the operation.
	ErrCodeServiceUnavailableException = "ServiceUnavailableException"

	// ErrCodeTooManyTagsException for service response error code
	// "TooManyTagsException".
	//
	// Application created with too many tags, or too many tags added to an application.
	// Note that the maximum number of application tags includes system tags. The
	// maximum number of user-defined application tags is 50.
	ErrCodeTooManyTagsException = "TooManyTagsException"

	// ErrCodeUnableToDetectSchemaException for service response error code
	// "UnableToDetectSchemaException".
	//
	// Data format is not valid. Amazon Kinesis Analytics is not able to detect
	// schema for the given streaming source.
	ErrCodeUnableToDetectSchemaException = "UnableToDetectSchemaException"

	// ErrCodeUnsupportedOperationException for service response error code
	// "UnsupportedOperationException".
	//
	// The request was rejected because a specified parameter is not supported or
	// a specified resource is not valid for this operation.
	ErrCodeUnsupportedOperationException = "UnsupportedOperationException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"CodeValidationException":                        newErrorCodeValidationException,
	"ConcurrentModificationException":                newErrorConcurrentModificationException,
	"InvalidApplicationConfigurationException":       newErrorInvalidApplicationConfigurationException,
	"InvalidArgumentException":                       newErrorInvalidArgumentException,
	"LimitExceededException":                         newErrorLimitExceededException,
	"ResourceInUseException":                         newErrorResourceInUseException,
	"ResourceNotFoundException":                      newErrorResourceNotFoundException,
	"ResourceProvisionedThroughputExceededException": newErrorResourceProvisionedThroughputExceededException,
	"ServiceUnavailableException":                    newErrorServiceUnavailableException,
	"TooManyTagsException":                           newErrorTooManyTagsException,
	"UnableToDetectSchemaException":                  newErrorUnableToDetectSchemaException,
	"UnsupportedOperationException":                  newErrorUnsupportedOperationException,
}

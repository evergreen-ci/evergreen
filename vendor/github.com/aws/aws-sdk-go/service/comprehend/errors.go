// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package comprehend

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeBatchSizeLimitExceededException for service response error code
	// "BatchSizeLimitExceededException".
	//
	// The number of documents in the request exceeds the limit of 25. Try your
	// request again with fewer documents.
	ErrCodeBatchSizeLimitExceededException = "BatchSizeLimitExceededException"

	// ErrCodeConcurrentModificationException for service response error code
	// "ConcurrentModificationException".
	//
	// Concurrent modification of the tags associated with an Amazon Comprehend
	// resource is not supported.
	ErrCodeConcurrentModificationException = "ConcurrentModificationException"

	// ErrCodeInternalServerException for service response error code
	// "InternalServerException".
	//
	// An internal server error occurred. Retry your request.
	ErrCodeInternalServerException = "InternalServerException"

	// ErrCodeInvalidFilterException for service response error code
	// "InvalidFilterException".
	//
	// The filter specified for the operation is invalid. Specify a different filter.
	ErrCodeInvalidFilterException = "InvalidFilterException"

	// ErrCodeInvalidRequestException for service response error code
	// "InvalidRequestException".
	//
	// The request is invalid.
	ErrCodeInvalidRequestException = "InvalidRequestException"

	// ErrCodeJobNotFoundException for service response error code
	// "JobNotFoundException".
	//
	// The specified job was not found. Check the job ID and try again.
	ErrCodeJobNotFoundException = "JobNotFoundException"

	// ErrCodeKmsKeyValidationException for service response error code
	// "KmsKeyValidationException".
	//
	// The KMS customer managed key (CMK) entered cannot be validated. Verify the
	// key and re-enter it.
	ErrCodeKmsKeyValidationException = "KmsKeyValidationException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// The specified resource name is already in use. Use a different name and try
	// your request again.
	ErrCodeResourceInUseException = "ResourceInUseException"

	// ErrCodeResourceLimitExceededException for service response error code
	// "ResourceLimitExceededException".
	//
	// The maximum number of resources per account has been exceeded. Review the
	// resources, and then try your request again.
	ErrCodeResourceLimitExceededException = "ResourceLimitExceededException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// The specified resource ARN was not found. Check the ARN and try your request
	// again.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeResourceUnavailableException for service response error code
	// "ResourceUnavailableException".
	//
	// The specified resource is not available. Check the resource and try your
	// request again.
	ErrCodeResourceUnavailableException = "ResourceUnavailableException"

	// ErrCodeTextSizeLimitExceededException for service response error code
	// "TextSizeLimitExceededException".
	//
	// The size of the input text exceeds the limit. Use a smaller document.
	ErrCodeTextSizeLimitExceededException = "TextSizeLimitExceededException"

	// ErrCodeTooManyRequestsException for service response error code
	// "TooManyRequestsException".
	//
	// The number of requests exceeds the limit. Resubmit your request later.
	ErrCodeTooManyRequestsException = "TooManyRequestsException"

	// ErrCodeTooManyTagKeysException for service response error code
	// "TooManyTagKeysException".
	//
	// The request contains more tag keys than can be associated with a resource
	// (50 tag keys per resource).
	ErrCodeTooManyTagKeysException = "TooManyTagKeysException"

	// ErrCodeTooManyTagsException for service response error code
	// "TooManyTagsException".
	//
	// The request contains more tags than can be associated with a resource (50
	// tags per resource). The maximum number of tags includes both existing tags
	// and those included in your current request.
	ErrCodeTooManyTagsException = "TooManyTagsException"

	// ErrCodeUnsupportedLanguageException for service response error code
	// "UnsupportedLanguageException".
	//
	// Amazon Comprehend can't process the language of the input text. For custom
	// entity recognition APIs, only English, Spanish, French, Italian, German,
	// or Portuguese are accepted. For a list of supported languages, see supported-languages.
	ErrCodeUnsupportedLanguageException = "UnsupportedLanguageException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"BatchSizeLimitExceededException": newErrorBatchSizeLimitExceededException,
	"ConcurrentModificationException": newErrorConcurrentModificationException,
	"InternalServerException":         newErrorInternalServerException,
	"InvalidFilterException":          newErrorInvalidFilterException,
	"InvalidRequestException":         newErrorInvalidRequestException,
	"JobNotFoundException":            newErrorJobNotFoundException,
	"KmsKeyValidationException":       newErrorKmsKeyValidationException,
	"ResourceInUseException":          newErrorResourceInUseException,
	"ResourceLimitExceededException":  newErrorResourceLimitExceededException,
	"ResourceNotFoundException":       newErrorResourceNotFoundException,
	"ResourceUnavailableException":    newErrorResourceUnavailableException,
	"TextSizeLimitExceededException":  newErrorTextSizeLimitExceededException,
	"TooManyRequestsException":        newErrorTooManyRequestsException,
	"TooManyTagKeysException":         newErrorTooManyTagKeysException,
	"TooManyTagsException":            newErrorTooManyTagsException,
	"UnsupportedLanguageException":    newErrorUnsupportedLanguageException,
}

// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package dynamodbstreams

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeExpiredIteratorException for service response error code
	// "ExpiredIteratorException".
	//
	// The shard iterator has expired and can no longer be used to retrieve stream
	// records. A shard iterator expires 15 minutes after it is retrieved using
	// the GetShardIterator action.
	ErrCodeExpiredIteratorException = "ExpiredIteratorException"

	// ErrCodeInternalServerError for service response error code
	// "InternalServerError".
	//
	// An error occurred on the server side.
	ErrCodeInternalServerError = "InternalServerError"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// Your request rate is too high. The AWS SDKs for DynamoDB automatically retry
	// requests that receive this exception. Your request is eventually successful,
	// unless your retry queue is too large to finish. Reduce the frequency of requests
	// and use exponential backoff. For more information, go to Error Retries and
	// Exponential Backoff (http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/ErrorHandling.html#APIRetries)
	// in the Amazon DynamoDB Developer Guide.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// The operation tried to access a nonexistent stream.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeTrimmedDataAccessException for service response error code
	// "TrimmedDataAccessException".
	//
	// The operation attempted to read past the oldest stream record in a shard.
	//
	// In DynamoDB Streams, there is a 24 hour limit on data retention. Stream records
	// whose age exceeds this limit are subject to removal (trimming) from the stream.
	// You might receive a TrimmedDataAccessException if:
	//
	//    * You request a shard iterator with a sequence number older than the trim
	//    point (24 hours).
	//
	//    * You obtain a shard iterator, but before you use the iterator in a GetRecords
	//    request, a stream record in the shard exceeds the 24 hour period and is
	//    trimmed. This causes the iterator to access a record that no longer exists.
	ErrCodeTrimmedDataAccessException = "TrimmedDataAccessException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"ExpiredIteratorException":   newErrorExpiredIteratorException,
	"InternalServerError":        newErrorInternalServerError,
	"LimitExceededException":     newErrorLimitExceededException,
	"ResourceNotFoundException":  newErrorResourceNotFoundException,
	"TrimmedDataAccessException": newErrorTrimmedDataAccessException,
}

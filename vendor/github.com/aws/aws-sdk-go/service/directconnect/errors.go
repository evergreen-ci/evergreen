// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package directconnect

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeClientException for service response error code
	// "DirectConnectClientException".
	//
	// One or more parameters are not valid.
	ErrCodeClientException = "DirectConnectClientException"

	// ErrCodeDuplicateTagKeysException for service response error code
	// "DuplicateTagKeysException".
	//
	// A tag key was specified more than once.
	ErrCodeDuplicateTagKeysException = "DuplicateTagKeysException"

	// ErrCodeServerException for service response error code
	// "DirectConnectServerException".
	//
	// A server-side error occurred.
	ErrCodeServerException = "DirectConnectServerException"

	// ErrCodeTooManyTagsException for service response error code
	// "TooManyTagsException".
	//
	// You have reached the limit on the number of tags that can be assigned.
	ErrCodeTooManyTagsException = "TooManyTagsException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"DirectConnectClientException": newErrorClientException,
	"DuplicateTagKeysException":    newErrorDuplicateTagKeysException,
	"DirectConnectServerException": newErrorServerException,
	"TooManyTagsException":         newErrorTooManyTagsException,
}

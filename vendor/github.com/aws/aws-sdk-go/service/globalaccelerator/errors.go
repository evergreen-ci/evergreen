// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package globalaccelerator

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeAcceleratorNotDisabledException for service response error code
	// "AcceleratorNotDisabledException".
	//
	// The accelerator that you specified could not be disabled.
	ErrCodeAcceleratorNotDisabledException = "AcceleratorNotDisabledException"

	// ErrCodeAcceleratorNotFoundException for service response error code
	// "AcceleratorNotFoundException".
	//
	// The accelerator that you specified doesn't exist.
	ErrCodeAcceleratorNotFoundException = "AcceleratorNotFoundException"

	// ErrCodeAccessDeniedException for service response error code
	// "AccessDeniedException".
	//
	// You don't have access permission.
	ErrCodeAccessDeniedException = "AccessDeniedException"

	// ErrCodeAssociatedEndpointGroupFoundException for service response error code
	// "AssociatedEndpointGroupFoundException".
	//
	// The listener that you specified has an endpoint group associated with it.
	// You must remove all dependent resources from a listener before you can delete
	// it.
	ErrCodeAssociatedEndpointGroupFoundException = "AssociatedEndpointGroupFoundException"

	// ErrCodeAssociatedListenerFoundException for service response error code
	// "AssociatedListenerFoundException".
	//
	// The accelerator that you specified has a listener associated with it. You
	// must remove all dependent resources from an accelerator before you can delete
	// it.
	ErrCodeAssociatedListenerFoundException = "AssociatedListenerFoundException"

	// ErrCodeEndpointGroupAlreadyExistsException for service response error code
	// "EndpointGroupAlreadyExistsException".
	//
	// The endpoint group that you specified already exists.
	ErrCodeEndpointGroupAlreadyExistsException = "EndpointGroupAlreadyExistsException"

	// ErrCodeEndpointGroupNotFoundException for service response error code
	// "EndpointGroupNotFoundException".
	//
	// The endpoint group that you specified doesn't exist.
	ErrCodeEndpointGroupNotFoundException = "EndpointGroupNotFoundException"

	// ErrCodeInternalServiceErrorException for service response error code
	// "InternalServiceErrorException".
	//
	// There was an internal error for AWS Global Accelerator.
	ErrCodeInternalServiceErrorException = "InternalServiceErrorException"

	// ErrCodeInvalidArgumentException for service response error code
	// "InvalidArgumentException".
	//
	// An argument that you specified is invalid.
	ErrCodeInvalidArgumentException = "InvalidArgumentException"

	// ErrCodeInvalidNextTokenException for service response error code
	// "InvalidNextTokenException".
	//
	// There isn't another item to return.
	ErrCodeInvalidNextTokenException = "InvalidNextTokenException"

	// ErrCodeInvalidPortRangeException for service response error code
	// "InvalidPortRangeException".
	//
	// The port numbers that you specified are not valid numbers or are not unique
	// for this accelerator.
	ErrCodeInvalidPortRangeException = "InvalidPortRangeException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// Processing your request would cause you to exceed an AWS Global Accelerator
	// limit.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeListenerNotFoundException for service response error code
	// "ListenerNotFoundException".
	//
	// The listener that you specified doesn't exist.
	ErrCodeListenerNotFoundException = "ListenerNotFoundException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"AcceleratorNotDisabledException":       newErrorAcceleratorNotDisabledException,
	"AcceleratorNotFoundException":          newErrorAcceleratorNotFoundException,
	"AccessDeniedException":                 newErrorAccessDeniedException,
	"AssociatedEndpointGroupFoundException": newErrorAssociatedEndpointGroupFoundException,
	"AssociatedListenerFoundException":      newErrorAssociatedListenerFoundException,
	"EndpointGroupAlreadyExistsException":   newErrorEndpointGroupAlreadyExistsException,
	"EndpointGroupNotFoundException":        newErrorEndpointGroupNotFoundException,
	"InternalServiceErrorException":         newErrorInternalServiceErrorException,
	"InvalidArgumentException":              newErrorInvalidArgumentException,
	"InvalidNextTokenException":             newErrorInvalidNextTokenException,
	"InvalidPortRangeException":             newErrorInvalidPortRangeException,
	"LimitExceededException":                newErrorLimitExceededException,
	"ListenerNotFoundException":             newErrorListenerNotFoundException,
}

// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package ecrpublic

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeEmptyUploadException for service response error code
	// "EmptyUploadException".
	//
	// The specified layer upload does not contain any layer parts.
	ErrCodeEmptyUploadException = "EmptyUploadException"

	// ErrCodeImageAlreadyExistsException for service response error code
	// "ImageAlreadyExistsException".
	//
	// The specified image has already been pushed, and there were no changes to
	// the manifest or image tag after the last push.
	ErrCodeImageAlreadyExistsException = "ImageAlreadyExistsException"

	// ErrCodeImageDigestDoesNotMatchException for service response error code
	// "ImageDigestDoesNotMatchException".
	//
	// The specified image digest does not match the digest that Amazon ECR calculated
	// for the image.
	ErrCodeImageDigestDoesNotMatchException = "ImageDigestDoesNotMatchException"

	// ErrCodeImageNotFoundException for service response error code
	// "ImageNotFoundException".
	//
	// The image requested does not exist in the specified repository.
	ErrCodeImageNotFoundException = "ImageNotFoundException"

	// ErrCodeImageTagAlreadyExistsException for service response error code
	// "ImageTagAlreadyExistsException".
	//
	// The specified image is tagged with a tag that already exists. The repository
	// is configured for tag immutability.
	ErrCodeImageTagAlreadyExistsException = "ImageTagAlreadyExistsException"

	// ErrCodeInvalidLayerException for service response error code
	// "InvalidLayerException".
	//
	// The layer digest calculation performed by Amazon ECR upon receipt of the
	// image layer does not match the digest specified.
	ErrCodeInvalidLayerException = "InvalidLayerException"

	// ErrCodeInvalidLayerPartException for service response error code
	// "InvalidLayerPartException".
	//
	// The layer part size is not valid, or the first byte specified is not consecutive
	// to the last byte of a previous layer part upload.
	ErrCodeInvalidLayerPartException = "InvalidLayerPartException"

	// ErrCodeInvalidParameterException for service response error code
	// "InvalidParameterException".
	//
	// The specified parameter is invalid. Review the available parameters for the
	// API request.
	ErrCodeInvalidParameterException = "InvalidParameterException"

	// ErrCodeInvalidTagParameterException for service response error code
	// "InvalidTagParameterException".
	//
	// An invalid parameter has been specified. Tag keys can have a maximum character
	// length of 128 characters, and tag values can have a maximum length of 256
	// characters.
	ErrCodeInvalidTagParameterException = "InvalidTagParameterException"

	// ErrCodeLayerAlreadyExistsException for service response error code
	// "LayerAlreadyExistsException".
	//
	// The image layer already exists in the associated repository.
	ErrCodeLayerAlreadyExistsException = "LayerAlreadyExistsException"

	// ErrCodeLayerPartTooSmallException for service response error code
	// "LayerPartTooSmallException".
	//
	// Layer parts must be at least 5 MiB in size.
	ErrCodeLayerPartTooSmallException = "LayerPartTooSmallException"

	// ErrCodeLayersNotFoundException for service response error code
	// "LayersNotFoundException".
	//
	// The specified layers could not be found, or the specified layer is not valid
	// for this repository.
	ErrCodeLayersNotFoundException = "LayersNotFoundException"

	// ErrCodeLimitExceededException for service response error code
	// "LimitExceededException".
	//
	// The operation did not succeed because it would have exceeded a service limit
	// for your account. For more information, see Amazon ECR Service Quotas (https://docs.aws.amazon.com/AmazonECR/latest/userguide/service-quotas.html)
	// in the Amazon Elastic Container Registry User Guide.
	ErrCodeLimitExceededException = "LimitExceededException"

	// ErrCodeReferencedImagesNotFoundException for service response error code
	// "ReferencedImagesNotFoundException".
	//
	// The manifest list is referencing an image that does not exist.
	ErrCodeReferencedImagesNotFoundException = "ReferencedImagesNotFoundException"

	// ErrCodeRegistryNotFoundException for service response error code
	// "RegistryNotFoundException".
	//
	// The registry does not exist.
	ErrCodeRegistryNotFoundException = "RegistryNotFoundException"

	// ErrCodeRepositoryAlreadyExistsException for service response error code
	// "RepositoryAlreadyExistsException".
	//
	// The specified repository already exists in the specified registry.
	ErrCodeRepositoryAlreadyExistsException = "RepositoryAlreadyExistsException"

	// ErrCodeRepositoryNotEmptyException for service response error code
	// "RepositoryNotEmptyException".
	//
	// The specified repository contains images. To delete a repository that contains
	// images, you must force the deletion with the force parameter.
	ErrCodeRepositoryNotEmptyException = "RepositoryNotEmptyException"

	// ErrCodeRepositoryNotFoundException for service response error code
	// "RepositoryNotFoundException".
	//
	// The specified repository could not be found. Check the spelling of the specified
	// repository and ensure that you are performing operations on the correct registry.
	ErrCodeRepositoryNotFoundException = "RepositoryNotFoundException"

	// ErrCodeRepositoryPolicyNotFoundException for service response error code
	// "RepositoryPolicyNotFoundException".
	//
	// The specified repository and registry combination does not have an associated
	// repository policy.
	ErrCodeRepositoryPolicyNotFoundException = "RepositoryPolicyNotFoundException"

	// ErrCodeServerException for service response error code
	// "ServerException".
	//
	// These errors are usually caused by a server-side issue.
	ErrCodeServerException = "ServerException"

	// ErrCodeTooManyTagsException for service response error code
	// "TooManyTagsException".
	//
	// The list of tags on the repository is over the limit. The maximum number
	// of tags that can be applied to a repository is 50.
	ErrCodeTooManyTagsException = "TooManyTagsException"

	// ErrCodeUnsupportedCommandException for service response error code
	// "UnsupportedCommandException".
	//
	// The action is not supported in this Region.
	ErrCodeUnsupportedCommandException = "UnsupportedCommandException"

	// ErrCodeUploadNotFoundException for service response error code
	// "UploadNotFoundException".
	//
	// The upload could not be found, or the specified upload ID is not valid for
	// this repository.
	ErrCodeUploadNotFoundException = "UploadNotFoundException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"EmptyUploadException":              newErrorEmptyUploadException,
	"ImageAlreadyExistsException":       newErrorImageAlreadyExistsException,
	"ImageDigestDoesNotMatchException":  newErrorImageDigestDoesNotMatchException,
	"ImageNotFoundException":            newErrorImageNotFoundException,
	"ImageTagAlreadyExistsException":    newErrorImageTagAlreadyExistsException,
	"InvalidLayerException":             newErrorInvalidLayerException,
	"InvalidLayerPartException":         newErrorInvalidLayerPartException,
	"InvalidParameterException":         newErrorInvalidParameterException,
	"InvalidTagParameterException":      newErrorInvalidTagParameterException,
	"LayerAlreadyExistsException":       newErrorLayerAlreadyExistsException,
	"LayerPartTooSmallException":        newErrorLayerPartTooSmallException,
	"LayersNotFoundException":           newErrorLayersNotFoundException,
	"LimitExceededException":            newErrorLimitExceededException,
	"ReferencedImagesNotFoundException": newErrorReferencedImagesNotFoundException,
	"RegistryNotFoundException":         newErrorRegistryNotFoundException,
	"RepositoryAlreadyExistsException":  newErrorRepositoryAlreadyExistsException,
	"RepositoryNotEmptyException":       newErrorRepositoryNotEmptyException,
	"RepositoryNotFoundException":       newErrorRepositoryNotFoundException,
	"RepositoryPolicyNotFoundException": newErrorRepositoryPolicyNotFoundException,
	"ServerException":                   newErrorServerException,
	"TooManyTagsException":              newErrorTooManyTagsException,
	"UnsupportedCommandException":       newErrorUnsupportedCommandException,
	"UploadNotFoundException":           newErrorUploadNotFoundException,
}

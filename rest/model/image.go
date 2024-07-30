package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
)

////////////////////////////////////////////////////////////////////////////////
//
// APIPackage is the model to be returned by the API whenever packages are fetched.

type APIPackage struct {
	Name    *string `json:"name"`
	Manager *string `json:"manager"`
	Version *string `json:"version"`
}

// BuildFromService converts from service level thirdparty.Package to an APIPackage.
func (apiPackage *APIPackage) BuildFromService(pkg thirdparty.Package) {
	apiPackage.Name = utility.ToStringPtr(pkg.Name)
	apiPackage.Manager = utility.ToStringPtr(pkg.Manager)
	apiPackage.Version = utility.ToStringPtr(pkg.Version)
}

// ToService returns a service layer package using the data from APIPackage.
func (apiPackage *APIPackage) ToService() *thirdparty.Package {
	pkg := thirdparty.Package{}
	pkg.Name = utility.FromStringPtr(apiPackage.Name)
	pkg.Manager = utility.FromStringPtr(apiPackage.Manager)
	pkg.Version = utility.FromStringPtr(apiPackage.Version)
	return &pkg
}

////////////////////////////////////////////////////////////////////////////////
//
// APIToolchain is the model to be returned by the API whenever toolchains are fetched.

type APIToolchain struct {
	Name    *string `json:"name"`
	Path    *string `json:"path"`
	Version *string `json:"version"`
}

// BuildFromService converts from service level thirdparty.Toolchain to an APIToolchain.
func (apiToolchain *APIToolchain) BuildFromService(toolchain thirdparty.Toolchain) {
	apiToolchain.Name = utility.ToStringPtr(toolchain.Name)
	apiToolchain.Path = utility.ToStringPtr(toolchain.Manager)
	apiToolchain.Version = utility.ToStringPtr(toolchain.Version)
}

// ToService returns a service layer toolchain using the data from APIToolchain.
func (apiToolchain *APIToolchain) ToService() *thirdparty.Toolchain {
	toolchain := thirdparty.Toolchain{}
	toolchain.Name = utility.FromStringPtr(apiToolchain.Name)
	toolchain.Manager = utility.FromStringPtr(apiToolchain.Path)
	toolchain.Version = utility.FromStringPtr(apiToolchain.Version)
	return &toolchain
}

////////////////////////////////////////////////////////////////////////////////
//
// APIImageEventEntry is the model to be returned by the API whenever image event entries are fetched.

type APIImageEventEntry struct {
	Name   *string                          `json:"name"`
	Before *string                          `json:"before"`
	After  *string                          `json:"after"`
	Type   thirdparty.ImageEventType        `json:"image_event_type"`
	Action thirdparty.ImageEventEntryAction `json:"action"`
}

// BuildFromService converts from service level thirdparty.ImageEventEntry to an APIImageEventEntry.
func (apiImageEventEntry *APIImageEventEntry) BuildFromService(imageEventEntry thirdparty.ImageEventEntry) {
	apiImageEventEntry.Name = utility.ToStringPtr(imageEventEntry.Name)
	apiImageEventEntry.Before = utility.ToStringPtr(imageEventEntry.Before)
	apiImageEventEntry.After = utility.ToStringPtr(imageEventEntry.After)
	apiImageEventEntry.Type = imageEventEntry.Type
	apiImageEventEntry.Action = imageEventEntry.Action
}

// ToService returns a service layer image event entry using the data from APIImageEventEntry.
func (apiImageEventEntry *APIImageEventEntry) ToService() *thirdparty.ImageEventEntry {
	imageEventEntry := thirdparty.ImageEventEntry{
		Name:   utility.FromStringPtr(apiImageEventEntry.Name),
		Before: utility.FromStringPtr(apiImageEventEntry.Before),
		After:  utility.FromStringPtr(apiImageEventEntry.After),
		Type:   apiImageEventEntry.Type,
		Action: apiImageEventEntry.Action,
	}
	return &imageEventEntry
}

////////////////////////////////////////////////////////////////////////////////
//
// APIImageEvent is the model to be returned by the API whenever image events are fetched.

type APIImageEvent struct {
	Entries   []APIImageEventEntry `json:"entries"`
	Timestamp *time.Time           `json:"timestamp"`
	AMIBefore *string              `json:"ami_before"`
	AMIAfter  *string              `json:"ami_after"`
}

// BuildFromService converts from service level thirdparty.ImageEvent to an APIImageEvent.
func (apiImageEvent *APIImageEvent) BuildFromService(imageEvent thirdparty.ImageEvent) {
	for _, imageEventEntry := range imageEvent.Entries {
		apiImageEventEntry := APIImageEventEntry{}
		apiImageEventEntry.BuildFromService(imageEventEntry)
		apiImageEvent.Entries = append(apiImageEvent.Entries, apiImageEventEntry)
	}
	apiImageEvent.Timestamp = utility.ToTimePtr(imageEvent.Timestamp)
	apiImageEvent.AMIBefore = utility.ToStringPtr(imageEvent.AMIBefore)
	apiImageEvent.AMIAfter = utility.ToStringPtr(imageEvent.AMIAfter)
}

// ToService returns a service layer image event using the data from APIImageEvent.
func (apiImage *APIImageEvent) ToService() *thirdparty.ImageEvent {
	imageEvent := thirdparty.ImageEvent{
		Timestamp: utility.FromTimePtr(apiImage.Timestamp),
		AMIBefore: utility.FromStringPtr(apiImage.AMIBefore),
		AMIAfter:  utility.FromStringPtr(apiImage.AMIAfter),
	}
	for _, apiImageEventEntry := range apiImage.Entries {
		imageEventEntry := apiImageEventEntry.ToService()
		imageEvent.Entries = append(imageEvent.Entries, *imageEventEntry)
	}
	return &imageEvent
}

////////////////////////////////////////////////////////////////////////////////
//
// APIImage is the model to be returned by the API whenever images are fetched.

type APIImage struct {
	ID           *string    `json:"id"`
	AMI          *string    `json:"ami"`
	Kernel       *string    `json:"kernel"`
	LastDeployed *time.Time `json:"last_deployed"`
	Name         *string    `json:"name"`
	VersionID    *string    `json:"version_id"`
}

// BuildFromService converts from service level thirdparty.Image to an APIImage.
func (apiImage *APIImage) BuildFromService(image thirdparty.Image) {
	apiImage.ID = utility.ToStringPtr(image.ID)
	apiImage.AMI = utility.ToStringPtr(image.AMI)
	apiImage.Kernel = utility.ToStringPtr(image.Kernel)
	apiImage.LastDeployed = utility.ToTimePtr(image.LastDeployed)
	apiImage.Name = utility.ToStringPtr(image.Name)
	apiImage.VersionID = utility.ToStringPtr(image.VersionID)
}

// ToService returns a service layer image using the data from APIImage.
func (apiImage *APIImage) ToService() *thirdparty.Image {
	image := thirdparty.Image{}
	image.ID = utility.FromStringPtr(apiImage.ID)
	image.AMI = utility.FromStringPtr(apiImage.AMI)
	image.Kernel = utility.FromStringPtr(apiImage.Kernel)
	image.LastDeployed = utility.FromTimePtr(apiImage.LastDeployed)
	image.Name = utility.FromStringPtr(apiImage.Name)
	image.VersionID = utility.FromStringPtr(apiImage.VersionID)
	return &image
}

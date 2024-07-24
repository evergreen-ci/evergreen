package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
)

////////////////////////////////////////////////////////////////////////////////
//
// APIPackage is the model to be returned by the API whenever packages are fetched

type APIPackage struct {
	Name    *string `json:"name"`
	Manager *string `json:"manager"`
	Version *string `json:"version"`
}

////////////////////////////////////////////////////////////////////////////////
//
// APIImage is the model to be returned by the API whenever images are fetched

type APIImage struct {
	ID           *string      `json:"id"`
	AMI          *string      `json:"ami"`
	Distros      []APIDistro  `json:"distros"`
	Kernel       *string      `json:"kernel"`
	LastDeployed time.Time    `json:"last_deployed"`
	Name         *string      `json:"name"`
	Packages     []APIPackage `json:"packages"`
	VersionID    *string      `json:"version_id"`
}

// BuildFromService converts from service level thirdparty.Image to an APIImage
func (apiImage *APIImage) BuildFromService(image thirdparty.Image) {
	apiImage.ID = utility.ToStringPtr(image.ID)
	apiImage.AMI = utility.ToStringPtr(image.AMI)
	apiImage.Kernel = utility.ToStringPtr(image.Kernel)
	apiImage.LastDeployed = image.LastDeployed
	apiImage.Name = &image.Name
	apiImage.VersionID = &image.VersionID
}

// ToService returns a service layer image using the data from APIImage
func (apiImage *APIImage) ToService() *thirdparty.Image {
	image := thirdparty.Image{}
	return &image
}

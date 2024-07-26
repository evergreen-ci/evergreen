package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
)

////////////////////////////////////////////////////////////////////////////////
//
// APIPackageOpts is the model to be returned by the API whenever package opts are fetched

type APIPackageOpts struct {
	Name    *string `json:"name"`
	Manager *string `json:"manager"`
	Limit   int     `json:"limit"`
	Page    int     `json:"page"`
}

// BuildFromService converts from service level thirdparty.PackageFilterOptions to an APIPackageOpts
func (apiPackageOpts *APIPackageOpts) BuildFromService(pkg thirdparty.PackageFilterOptions) {
	apiPackageOpts.Name = utility.ToStringPtr(pkg.Name)
	apiPackageOpts.Manager = utility.ToStringPtr(pkg.Manager)
	apiPackageOpts.Limit = pkg.Limit
	apiPackageOpts.Page = pkg.Page
}

// ToService returns a service layer package options using the data from APIPackageOpts
func (apiPackageOpts *APIPackageOpts) ToService() *thirdparty.PackageFilterOptions {
	pkgOpts := thirdparty.PackageFilterOptions{}
	pkgOpts.Name = utility.FromStringPtr(apiPackageOpts.Name)
	pkgOpts.Manager = utility.FromStringPtr(apiPackageOpts.Manager)
	pkgOpts.Limit = apiPackageOpts.Limit
	pkgOpts.Page = apiPackageOpts.Page
	return &pkgOpts
}

////////////////////////////////////////////////////////////////////////////////
//
// APIPackage is the model to be returned by the API whenever packages are fetched

type APIPackage struct {
	Name    *string `json:"name"`
	Manager *string `json:"manager"`
	Version *string `json:"version"`
}

// BuildFromService converts from service level thirdparty.Package to an APIPackage
func (apiPackage *APIPackage) BuildFromService(pkg thirdparty.Package) {
	apiPackage.Name = utility.ToStringPtr(pkg.Name)
	apiPackage.Manager = utility.ToStringPtr(pkg.Manager)
	apiPackage.Version = utility.ToStringPtr(pkg.Version)
}

// ToService returns a service layer package using the data from APIPackage
func (apiPackage *APIPackage) ToService() *thirdparty.Package {
	pkg := thirdparty.Package{}
	pkg.Name = utility.FromStringPtr(apiPackage.Name)
	pkg.Manager = utility.FromStringPtr(apiPackage.Manager)
	pkg.Version = utility.FromStringPtr(apiPackage.Version)
	return &pkg
}

////////////////////////////////////////////////////////////////////////////////
//
// APIImage is the model to be returned by the API whenever images are fetched

type APIImage struct {
	ID           *string   `json:"id"`
	AMI          *string   `json:"ami"`
	Kernel       *string   `json:"kernel"`
	LastDeployed time.Time `json:"last_deployed"`
	Name         *string   `json:"name"`
	VersionID    *string   `json:"version_id"`
}

// BuildFromService converts from service level thirdparty.Image to an APIImage
func (apiImage *APIImage) BuildFromService(image thirdparty.Image) {
	apiImage.ID = utility.ToStringPtr(image.ID)
	apiImage.AMI = utility.ToStringPtr(image.AMI)
	apiImage.Kernel = utility.ToStringPtr(image.Kernel)
	apiImage.LastDeployed = image.LastDeployed
	apiImage.Name = utility.ToStringPtr(image.Name)
	apiImage.VersionID = utility.ToStringPtr(image.VersionID)
}

// ToService returns a service layer image using the data from APIImage
func (apiImage *APIImage) ToService() *thirdparty.Image {
	image := thirdparty.Image{}
	image.ID = utility.FromStringPtr(apiImage.ID)
	image.AMI = utility.FromStringPtr(apiImage.AMI)
	image.Kernel = utility.FromStringPtr(apiImage.Kernel)
	image.LastDeployed = apiImage.LastDeployed
	image.Name = utility.FromStringPtr(apiImage.Name)
	image.VersionID = utility.FromStringPtr(apiImage.VersionID)
	return &image
}

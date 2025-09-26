package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
)

type APICLIUpdate struct {
	ClientConfig APIClientConfig `json:"client_config"`
	IgnoreUpdate bool            `json:"ignore_update"`
}

func (a *APICLIUpdate) BuildFromService(c evergreen.ClientConfig) {
	a.ClientConfig.BuildFromService(c)
}

type APIClientConfig struct {
	ClientBinaries          []APIClientBinary `json:"client_binaries,omitempty"`
	S3ClientBinaries        []APIClientBinary `json:"s3_client_binaries,omitempty"`
	LatestRevision          *string           `json:"latest_revision"`
	OldestAllowedCLIVersion *string           `json:"oldest_allowed_cli_version"`
}

func (a *APIClientConfig) BuildFromService(c evergreen.ClientConfig) {
	a.ClientBinaries = make([]APIClientBinary, len(c.ClientBinaries))
	for i := range a.ClientBinaries {
		a.ClientBinaries[i].BuildFromService(c.ClientBinaries[i])
	}
	a.OldestAllowedCLIVersion = utility.ToStringPtr(c.OldestAllowedCLIVersion)
	a.LatestRevision = utility.ToStringPtr(c.LatestRevision)
}

func (a *APIClientConfig) ToService() evergreen.ClientConfig {
	c := evergreen.ClientConfig{}
	c.LatestRevision = utility.FromStringPtr(a.LatestRevision)
	c.OldestAllowedCLIVersion = utility.FromStringPtr(a.OldestAllowedCLIVersion)
	c.ClientBinaries = make([]evergreen.ClientBinary, len(a.ClientBinaries))
	for i := range c.ClientBinaries {
		c.ClientBinaries[i] = a.ClientBinaries[i].ToService()
	}

	return c
}

type APIClientBinary struct {
	Arch        *string `json:"arch"`
	OS          *string `json:"os"`
	URL         *string `json:"url"`
	DisplayName *string `json:"display_name"`
}

func (a *APIClientBinary) BuildFromService(b evergreen.ClientBinary) {
	a.Arch = utility.ToStringPtr(b.Arch)
	a.OS = utility.ToStringPtr(b.OS)
	a.URL = utility.ToStringPtr(b.URL)
	a.DisplayName = utility.ToStringPtr(b.DisplayName)
}

func (a *APIClientBinary) ToService() evergreen.ClientBinary {
	b := evergreen.ClientBinary{}
	b.Arch = utility.FromStringPtr(a.Arch)
	b.OS = utility.FromStringPtr(a.OS)
	b.URL = utility.FromStringPtr(a.URL)
	b.DisplayName = utility.FromStringPtr(a.DisplayName)
	return b
}

package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type APICLIUpdate struct {
	ClientConfig APIClientConfig `json:"client_config"`
	IgnoreUpdate bool            `json:"ignore_update"`
}

func (a *APICLIUpdate) BuildFromService(h interface{}) error {
	c, ok := h.(evergreen.ClientConfig)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting client config")
	}
	return a.ClientConfig.BuildFromService(c)
}

func (a *APICLIUpdate) ToService() (interface{}, error) {
	return nil, errors.New("(*APICLIUpdate) not implemented for read-only route")
}

type APIClientConfig struct {
	ClientBinaries   []APIClientBinary `json:"client_binaries,omitempty"`
	S3ClientBinaries []APIClientBinary `json:"s3_client_binaries,omitempty"`
	LatestRevision   *string           `json:"latest_revision"`
}

func (a *APIClientConfig) BuildFromService(h interface{}) error {
	c, ok := h.(evergreen.ClientConfig)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting client config")
	}

	catcher := grip.NewBasicCatcher()
	a.ClientBinaries = make([]APIClientBinary, len(c.ClientBinaries))
	for i := range a.ClientBinaries {
		catcher.Add(a.ClientBinaries[i].BuildFromService(c.ClientBinaries[i]))
	}
	a.S3ClientBinaries = make([]APIClientBinary, len(c.S3ClientBinaries))
	for i := range a.S3ClientBinaries {
		catcher.Add(a.S3ClientBinaries[i].BuildFromService(c.S3ClientBinaries[i]))
	}

	a.LatestRevision = utility.ToStringPtr(c.LatestRevision)

	return catcher.Resolve()
}

func (a *APIClientConfig) ToService() (interface{}, error) {
	c := evergreen.ClientConfig{}
	c.LatestRevision = utility.FromStringPtr(a.LatestRevision)
	c.ClientBinaries = make([]evergreen.ClientBinary, len(a.ClientBinaries))

	catcher := grip.NewBasicCatcher()
	for i := range c.ClientBinaries {
		var err error
		bin, err := a.ClientBinaries[i].ToService()
		if err != nil {
			catcher.Add(err)
			continue
		}
		var ok bool
		c.ClientBinaries[i], ok = bin.(evergreen.ClientBinary)
		if !ok {
			catcher.Errorf("programmatic error: incorrect type %T returned from service-level conversion", bin)
		}
	}

	c.S3ClientBinaries = make([]evergreen.ClientBinary, len(a.S3ClientBinaries))
	for i := range a.S3ClientBinaries {
		var err error
		bin, err := a.S3ClientBinaries[i].ToService()
		if err != nil {
			catcher.Add(err)
			continue
		}
		var ok bool
		c.S3ClientBinaries[i], ok = bin.(evergreen.ClientBinary)
		if !ok {
			catcher.Errorf("programmatic error: incorrect type %T returned from service-level conversion", bin)
		}
	}

	return c, catcher.Resolve()
}

type APIClientBinary struct {
	Arch        *string `json:"arch"`
	OS          *string `json:"os"`
	URL         *string `json:"url"`
	DisplayName *string `json:"display_name"`
}

func (a *APIClientBinary) BuildFromService(h interface{}) error {
	b, ok := h.(evergreen.ClientBinary)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting client binary")
	}
	a.Arch = utility.ToStringPtr(b.Arch)
	a.OS = utility.ToStringPtr(b.OS)
	a.URL = utility.ToStringPtr(b.URL)
	a.DisplayName = utility.ToStringPtr(b.DisplayName)
	return nil
}

func (a *APIClientBinary) ToService() (interface{}, error) {
	b := evergreen.ClientBinary{}
	b.Arch = utility.FromStringPtr(a.Arch)
	b.OS = utility.FromStringPtr(a.OS)
	b.URL = utility.FromStringPtr(a.URL)
	b.DisplayName = utility.FromStringPtr(a.DisplayName)
	return b, nil
}

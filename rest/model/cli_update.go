package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
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
	a.ClientConfig.BuildFromService(c)
	return nil
}

func (a *APICLIUpdate) ToService() (interface{}, error) {
	return nil, errors.New("(*APICLIUpdate) not implemented for read-only route")
}

type APIClientConfig struct {
	ClientBinaries []APIClientBinary `json:"client_binaries,omitempty"`
	LatestRevision APIString         `json:"latest_revision"`
}

func (a *APIClientConfig) BuildFromService(h interface{}) error {
	c, ok := h.(evergreen.ClientConfig)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting client config")
	}
	a.ClientBinaries = make([]APIClientBinary, len(c.ClientBinaries))
	catcher := grip.NewSimpleCatcher()
	for i := range a.ClientBinaries {
		catcher.Add(a.ClientBinaries[i].BuildFromService(c.ClientBinaries[i]))
	}
	a.LatestRevision = APIString(c.LatestRevision)
	return catcher.Resolve()
}

func (a *APIClientConfig) ToService() (interface{}, error) {
	return nil, errors.New("(*APIClientConfig) not implemented for read-only route")
}

type APIClientBinary struct {
	Arch APIString `json:"arch"`
	OS   APIString `json:"os"`
	URL  APIString `json:"url"`
}

func (a *APIClientBinary) BuildFromService(h interface{}) error {
	b, ok := h.(evergreen.ClientBinary)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting client binary")
	}
	a.Arch = APIString(b.Arch)
	a.OS = APIString(b.OS)
	a.URL = APIString(b.URL)
	return nil
}

func (a *APIClientBinary) ToService() (interface{}, error) {
	return nil, errors.New("(*APIClientBinary) not implemented for read-only route")
}

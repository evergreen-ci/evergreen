package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest"
)

// DBDistroConnector is a struct that implements the Distro related methods
// from the Connector through interactions with the backing database.
type DBDistroConnector struct{}

// FindAllDistros queries the database to find all distros.
func (dc *DBDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	distros, err := distro.Find(distro.All)
	if err != nil {
		return nil, err
	}
	if distros == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no distros found"),
		}
	}
	return distros, nil
}

// MockDistroConnector is a struct that implements Distro-related methods
// for testing.
type MockDistroConnector struct {
	Distros []distro.Distro
}

// FindAllDistros is a mock implementation for testing.
func (dc *MockDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	return dc.Distros, nil
}

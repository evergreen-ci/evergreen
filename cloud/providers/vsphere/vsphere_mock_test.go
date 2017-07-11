// +build go1.7

package vsphere

import (
	"github.com/pkg/errors"
)

type clientMock struct {
	// API call options
	failInit bool
}

func (c *clientMock) Init(_ *authOptions) error {
	if c.failInit {
		return errors.New("failed to initialize instance")
	}

	return nil
}

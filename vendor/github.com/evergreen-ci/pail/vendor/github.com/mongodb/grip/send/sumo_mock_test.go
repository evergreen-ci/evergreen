package send

import (
	"errors"
)

type sumoClientMock struct {
	failSend bool

	numSent int
}

func (c *sumoClientMock) Create(_ string) {}

func (c *sumoClientMock) Send(_ []byte, _ string) error {
	if c.failSend {
		return errors.New("sending failed")
	}

	c.numSent++

	return nil
}

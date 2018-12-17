package send

import (
	"errors"

	"github.com/fuyufjh/splunk-hec-go"
)

type splunkClientMock struct {
	failCreate bool
	failSend   bool

	numSent  int
	httpSent int
}

func (c *splunkClientMock) Create(string, string, string) error {
	if c.failCreate {
		return errors.New("creation failed")
	}

	return nil
}

func (c *splunkClientMock) WriteEvent(*hec.Event) error {
	if c.failSend {
		return errors.New("write failed")
	}

	c.numSent++
	c.httpSent++

	return nil
}

func (c *splunkClientMock) WriteBatch(b []*hec.Event) error {
	if c.failSend {
		return errors.New("write failed")
	}

	c.numSent += len(b)
	c.httpSent++

	return nil
}

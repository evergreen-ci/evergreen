package send

import (
	"errors"

	xmpp "github.com/mattn/go-xmpp"
)

type xmppClientMock struct {
	failCreate bool
	failSend   bool

	numCloses int
	numSent   int
}

func (c *xmppClientMock) Create(_ XMPPConnectionInfo) error {
	if c.failCreate {
		return errors.New("creation failed")
	}

	return nil
}

func (c *xmppClientMock) Send(_ xmpp.Chat) (int, error) {
	if c.failSend {
		return 0, errors.New("sending failed")
	}

	c.numSent++

	return 0, nil
}

func (c *xmppClientMock) Close() error {
	c.numCloses++
	return nil
}

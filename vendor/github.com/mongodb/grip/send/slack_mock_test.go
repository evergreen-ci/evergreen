package send

import (
	"errors"

	"github.com/bluele/slack"
)

// implements the slackClient interface for use in tests.
type slackClientMock struct {
	failAuthTest       bool
	failSendingMessage bool
	numSent            int
}

func (c *slackClientMock) Create(_ string) {}
func (c *slackClientMock) AuthTest() (*slack.AuthTestApiResponse, error) {
	if c.failAuthTest {
		return nil, errors.New("mock failed auth test")
	}
	return nil, nil
}

func (c *slackClientMock) ChatPostMessage(_, _ string, _ *slack.ChatPostMessageOpt) error {
	if c.failSendingMessage {
		return errors.New("mock failed auth test")
	}

	c.numSent++

	return nil
}

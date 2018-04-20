package notification

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/suite"
)

type payloadSuite struct {
	suite.Suite
}

func TestPayloads(t *testing.T) {
	suite.Run(t, &payloadSuite{})
}

func (s *payloadSuite) TestEmail() {
	url := "https://example.com/patch/1234"
	status := "failed"

	m, err := emailPayload("1234", "patch", url, status, []event.Selector{
		{
			Type: "test",
			Data: "something",
		},
	})
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal(m.Subject, "Evergreen patch has failed!")
	s.Contains(m.Body, "your Evergreen patch <")
	s.Contains(m.Body, "> has failed.")
	s.Contains(m.Body, `href="`+url+`"`)
	s.Contains(m.Body, "X-Evergreen-test:something")
}

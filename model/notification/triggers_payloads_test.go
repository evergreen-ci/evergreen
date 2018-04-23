package notification

import (
	"net/http"
	"testing"

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

	headers := http.Header{
		"X-Evergreen-test": []string{"something"},
	}

	t := commonTemplateData{
		Object:          "patch",
		ID:              "1234",
		URL:             url,
		PastTenseStatus: status,
		Headers:         headers,
	}

	m, err := emailPayload(t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal(m.Subject, "Evergreen patch has failed!")
	s.Contains(m.Body, "Your Evergreen patch <")
	s.Contains(m.Body, "> has failed.")
	s.Contains(m.Body, `href="`+url+`"`)
	s.Contains(m.Body, "X-Evergreen-test:something")
}

package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TestSpawnHostHandlerSuite struct {
	rm *RouteManager
	sc *data.MockConnector

	suite.Suite
}

func TestSpawnHostHandler(t *testing.T) {
	s := &TestSpawnHostHandlerSuite{}
	suite.Run(t, s)
}

func (s *TestSpawnHostHandlerSuite) SetupTest() {
	s.rm = getSpawnHostsRouteManager("", 2)

	s.sc = &data.MockConnector{
		MockHostConnector: data.MockHostConnector{
			CachedHosts: []host.Host{
				{
					Id:             "host1",
					Host:           "host1",
					Status:         evergreen.HostTerminated,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host2",
					Host:           "host2",
					Status:         evergreen.HostRunning,
					ExpirationTime: time.Now().Add(time.Hour),
				},
				{
					Id:             "host3",
					Host:           "host3",
					Status:         evergreen.HostUninitialized,
					ExpirationTime: time.Now().Add(time.Hour),
				},
			},
		}}
}

func (s *TestSpawnHostHandlerSuite) TestExecuteWithInvalidHost() {
	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostTerminate
	h.hostID = "host-that-doesn't-exist"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Error(err)
}

func (s *TestSpawnHostHandlerSuite) TestParseAndValidateWithInvalidAction() {
	mod := model.APISpawnHostModify{
		Action:   "fake",
		HostID:   "host1",
		RDPPwd:   "hunter2",
		AddHours: "0",
	}
	err := s.tryParseAndValidate(mod)
	s.Error(err)
	s.IsType(new(rest.APIError), err)
	apiErr := err.(*rest.APIError)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	s.Equal("invalid action", apiErr.Message)
}

// Tests for Termination actions

func (s *TestSpawnHostHandlerSuite) TestExecuteTerminateWithTerminatedHost() {
	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostTerminate
	h.hostID = "host1"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.NotNil(err)
	s.IsType(new(rest.APIError), err)
	apiErr := err.(*rest.APIError)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[0].Status)

}

func (s *TestSpawnHostHandlerSuite) TestExecuteTerminateWithUninitializedHost() {
	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostTerminate
	h.hostID = "host3"

	s.Equal(evergreen.HostUninitialized, s.sc.CachedHosts[2].Status)
	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostTerminated, s.sc.CachedHosts[2].Status)

}

func (s *TestSpawnHostHandlerSuite) TestExecuteTerminateWithRunningHost() {
	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostTerminate
	h.hostID = "host2"

	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)
	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Nil(err)
	s.Equal(evergreen.HostRunning, s.sc.CachedHosts[1].Status)

}

// Tests for change password action

func (s *TestSpawnHostHandlerSuite) TestParseAndValidateRejectsInvalidPasswords() {
	invalidPasswords := []model.APIString{"", "weak", "stilltooweak1", "火a11"}
	for _, password := range invalidPasswords {
		mod := model.APISpawnHostModify{
			Action:   HostPasswordUpdate,
			HostID:   "host1",
			RDPPwd:   password,
			AddHours: "0",
		}
		err := s.tryParseAndValidate(mod)

		s.Error(err)
		s.IsType(new(rest.APIError), err)
		apiErr := err.(*rest.APIError)
		s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	}
}

func (s *TestSpawnHostHandlerSuite) TestExecuteChangePassword() {
	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostPasswordUpdate
	h.hostID = "host2"
	h.rdpPassword = "Hunter2!"

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Nil(err)
}

// Tests for extend expiration action

func (s *TestSpawnHostHandlerSuite) TestParseAndValidateRejectsInvalidExpirations() {
	invalidExpirations := []model.APIString{"not a number", "0", "9223372036854775807"}
	for _, extendBy := range invalidExpirations {
		mod := model.APISpawnHostModify{
			Action:   HostExpirationExtension,
			HostID:   "host1",
			RDPPwd:   "",
			AddHours: extendBy,
		}

		err := s.tryParseAndValidate(mod)
		s.Error(err)
		s.IsType(new(rest.APIError), err)
		apiErr := err.(*rest.APIError)
		s.Equal(http.StatusBadRequest, apiErr.StatusCode)
	}
}

func (s *TestSpawnHostHandlerSuite) TestExecuteExtendExpirationWithLargeExpirationFails() {
	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostExpirationExtension
	h.hostID = "host2"
	h.addHours = time.Duration(9001 * time.Hour)

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.Error(err)
	s.IsType(new(rest.APIError), err)
	apiErr := err.(*rest.APIError)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
}

func (s *TestSpawnHostHandlerSuite) TestExecuteExtendExpiration() {
	dur := time.Duration(8 * time.Hour)
	expectedTime := s.sc.CachedHosts[1].ExpirationTime.Add(dur)

	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)
	h.action = HostExpirationExtension
	h.hostID = "host2"
	h.addHours = dur

	data, err := h.Execute(context.TODO(), s.sc)
	s.Empty(data.Result)
	s.NoError(err)
	s.Equal(expectedTime, s.sc.CachedHosts[1].ExpirationTime)

}
func (s *TestSpawnHostHandlerSuite) tryParseAndValidate(mod model.APISpawnHostModify) error {
	data, err := json.Marshal(mod)
	s.NoError(err)
	s.NotEmpty(data)

	var r *http.Request
	r, err = http.NewRequest("POST", "http://example.com/spawn", bytes.NewReader(data))
	s.NoError(err)
	s.NotNil(r)

	h := s.rm.Methods[0].Handler().(*spawnHostModifyHandler)

	return h.ParseAndValidate(context.TODO(), r)
}

func TestRDPPasswordValidation(t *testing.T) {
	assert := assert.New(t)

	goodPasswords := []string{"地火風水心1!", "V3ryStr0ng!", "Aaaaa\\"}
	badPasswords := []string{"", "weak", "stilltooweak1", "火火火1"}

	for _, password := range goodPasswords {
		assert.True(validateRDPPassword(password))
	}

	for _, password := range badPasswords {
		assert.False(validateRDPPassword(password))
	}
}

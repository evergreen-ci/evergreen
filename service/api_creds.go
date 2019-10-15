package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type Credentials struct {
	Username string "username"
	Password string "password"
}

func (as *APIServer) BuildloggerV3Credentials(w http.ResponseWriter, r *http.Request) {
	conf, err := evergreen.GetConfig()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "Error getting evergreen LDAP credentials for buildloggerv3"))
		return
	}

	creds := &Credentials{
		Username: conf.LoggerConfig.BuildloggerV3User,
		Password: conf.LoggerConfig.BuildloggerV3Password,
	}
	gimlet.WriteJSON(w, creds)
}

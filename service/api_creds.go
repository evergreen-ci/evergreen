package service

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (as *APIServer) BuildloggerV3Credentials(w http.ResponseWriter, r *http.Request) {
	creds := &Credentials{
		Username: as.Settings.LoggerConfig.BuildloggerV3User,
		Password: as.Settings.LoggerConfig.BuildloggerV3Password,
	}
	gimlet.WriteJSON(w, creds)
}

package service

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

type buildlogger struct {
	BaseURL  string `json:"base_url"`
	RPCPort  string `json:"rpc_port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (as *APIServer) Buildlogger(w http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(w, &buildlogger{
		BaseURL:  as.Settings.LoggerConfig.BuildloggerBaseURL,
		RPCPort:  as.Settings.LoggerConfig.BuildloggerRPCPort,
		Username: as.Settings.LoggerConfig.BuildloggerUser,
		Password: as.Settings.LoggerConfig.BuildloggerPassword,
	})
}

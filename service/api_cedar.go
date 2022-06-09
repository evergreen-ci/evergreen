package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
)

func (as *APIServer) Cedar(w http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(w, &apimodels.CedarConfig{
		BaseURL:  as.Settings.Cedar.BaseURL,
		RPCPort:  as.Settings.Cedar.RPCPort,
		Username: as.Settings.Cedar.User,
		APIKey:   as.Settings.Cedar.APIKey,
	})
}

package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

func (as *APIServer) keyValPluginInc(w http.ResponseWriter, r *http.Request) {
	key := ""
	err := util.ReadJSONInto(r.Body, &key)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "could not get key"))
		return
	}

	keyVal := &model.KeyVal{Key: key}
	if err = keyVal.Inc(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "problem doing findAndModify on key %s", key))
		return
	}

	as.WriteJSON(w, http.StatusOK, keyVal)
}

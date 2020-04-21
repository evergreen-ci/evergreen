package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func (as *APIServer) keyValPluginInc(w http.ResponseWriter, r *http.Request) {
	key := ""
	err := utility.ReadJSON(r.Body, &key)
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

	gimlet.WriteJSON(w, keyVal)
}

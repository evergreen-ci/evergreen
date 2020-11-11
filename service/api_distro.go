package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
)

func (as *APIServer) GetDistroView(w http.ResponseWriter, r *http.Request) {
	h := MustHaveHost(r)

	dv := apimodels.DistroView{
		CloneMethod:         h.Distro.CloneMethod,
		DisableShallowClone: h.Distro.DisableShallowClone,
		WorkDir:             h.Distro.WorkDir,
	}
	gimlet.WriteJSON(w, dv)
}

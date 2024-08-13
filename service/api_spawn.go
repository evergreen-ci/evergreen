package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type spawnResponse struct {
	Hosts    []host.Host `json:"hosts,omitempty"`
	HostInfo host.Host   `json:"host_info,omitempty"`
	Distros  []string    `json:"distros,omitempty"`

	// empty if the request succeeded
	ErrorMessage string `json:"error_message,omitempty"`
}

func (as *APIServer) listDistros(w http.ResponseWriter, r *http.Request) {
	distros, err := distro.Find(r.Context(), distro.BySpawnAllowed())
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	distroList := []string{}
	for _, d := range distros {
		distroList = append(distroList, d.Id)
	}
	gimlet.WriteJSON(w, spawnResponse{Distros: distroList})
}

func (as *APIServer) requestHost(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	hostRequest := struct {
		Distro    string `json:"distro"`
		PublicKey string `json:"public_key"`
		Region    string `json:"region"`
	}{}
	err := utility.ReadJSON(utility.NewRequestReader(r), &hostRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if hostRequest.Distro == "" {
		http.Error(w, "distro may not be blank", http.StatusBadRequest)
		return
	}
	if hostRequest.PublicKey == "" {
		http.Error(w, "public key may not be blank", http.StatusBadRequest)
		return
	}

	options := &model.HostRequestOptions{
		DistroID:     hostRequest.Distro,
		KeyName:      hostRequest.PublicKey,
		Region:       hostRequest.Region,
		TaskID:       "",
		UserData:     "",
		InstanceTags: nil,
		InstanceType: "",
	}
	ctx, cancel := as.env.Context()
	defer cancel()
	spawnHost, err := data.NewIntentHost(ctx, options, user, as.env)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if spawnHost == nil {
		http.Error(w, "spawned host is nil", http.StatusBadRequest)
		return
	}

	gimlet.WriteJSON(w, "")
}

// returns info on the host specified
func (as *APIServer) hostInfo(w http.ResponseWriter, r *http.Request) {
	instanceId := gimlet.GetVars(r)["instance_id"]

	h, err := host.FindOne(r.Context(), host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	gimlet.WriteJSON(w, spawnResponse{HostInfo: *h})
}

// returns info on all of the hosts spawned by a user
func (as *APIServer) hostsInfoForUser(w http.ResponseWriter, r *http.Request) {
	user := gimlet.GetVars(r)["user"]

	hosts, err := host.Find(r.Context(), host.ByUserWithUnterminatedStatus(user))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, spawnResponse{Hosts: hosts})
}

func (as *APIServer) modifyHost(w http.ResponseWriter, r *http.Request) {
	instanceId := gimlet.GetVars(r)["instance_id"]
	hostAction := r.FormValue("action")

	h, err := host.FindOne(r.Context(), host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if h == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	user := gimlet.GetUser(ctx)
	if user == nil || user.Username() != h.StartedBy {
		message := fmt.Sprintf("Only %v is authorized to terminate this host", h.StartedBy)
		http.Error(w, message, http.StatusUnauthorized)
		return
	}

	switch hostAction {
	case "terminate":
		if h.Status == evergreen.HostTerminated {
			message := fmt.Sprintf("Host %v is already terminated", h.Id)
			http.Error(w, message, http.StatusBadRequest)
			return
		}

		cloudHost, err := cloud.GetCloudHost(ctx, h, as.env)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if err = cloudHost.TerminateInstance(ctx, user.Username(), fmt.Sprintf("terminated via API by %s", user.Username())); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Failed to terminate spawn host"))
			return
		}
		gimlet.WriteJSON(w, spawnResponse{HostInfo: *h})
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action %v", hostAction), http.StatusBadRequest)
	}

}

// returns dockerfle as text
func getDockerfile(w http.ResponseWriter, r *http.Request) {
	parts := []string{
		"ARG BASE_IMAGE",
		"FROM $BASE_IMAGE",
		"ARG URL",
		"ARG EXECUTABLE_SUB_PATH",
		"ARG BINARY_NAME",
		"ADD ${URL}/clients/${EXECUTABLE_SUB_PATH} /",
		"RUN chmod 0777 /${BINARY_NAME}",
	}

	gimlet.WriteText(w, strings.Join(parts, "\n"))
}

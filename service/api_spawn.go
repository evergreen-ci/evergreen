package service

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
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
	distros, err := distro.Find(distro.BySpawnAllowed())
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	distroList := []string{}
	for _, d := range distros {
		distroList = append(distroList, d.Id)
	}
	as.WriteJSON(w, http.StatusOK, spawnResponse{Distros: distroList})
}

func (as *APIServer) requestHost(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	hostRequest := struct {
		Distro    string `json:"distro"`
		PublicKey string `json:"public_key"`
		UserData  string `json:"userdata"`
	}{}
	err := util.ReadJSONInto(util.NewRequestReader(r), &hostRequest)
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

	hc := &data.DBHostConnector{}
	spawnHost, err := hc.NewIntentHost(hostRequest.Distro, hostRequest.PublicKey, "", hostRequest.UserData, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if spawnHost == nil {
		http.Error(w, "spawned host is nil", http.StatusBadRequest)
		return
	}

	as.WriteJSON(w, http.StatusOK, "")
}

func (as *APIServer) spawnHostReady(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceId := vars["instance_id"]
	status := vars["status"]

	// mark the host itself as provisioned
	h, err := host.FindOne(host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}

	if status == evergreen.HostStatusSuccess {
		if err = h.SetRunning(); err != nil {
			grip.Errorf("Error marking host id %s as %s: %+v",
				instanceId, evergreen.HostStatusSuccess, err)
		}
	} else {
		grip.Warning(errors.WithStack(alerts.RunHostProvisionFailTriggers(h)))
		if err = h.SetDecommissioned(); err != nil {
			grip.Errorf("Error marking host %s for user %s as decommissioned: %+v",
				h.Host, h.StartedBy, err)
		}
		grip.Infof("Decommissioned %s for user %s because provisioning failed",
			h.Host, h.StartedBy)

		// send notification to the Evergreen team about this provisioning failure
		subject := fmt.Sprintf("%v Spawn provisioning failure on %v", notify.ProvisionFailurePreface, h.Distro.Id)
		message := fmt.Sprintf("Provisioning failed on %v host %v for user %v", h.Distro.Id, h.Host, h.StartedBy)
		if err = notify.NotifyAdmins(subject, message, &as.Settings); err != nil {
			grip.Errorln("issue sending email:", err)
		}

		// get/store setup logs
		body := util.NewRequestReader(r)
		defer body.Close()
		setupLog, err := ioutil.ReadAll(body)
		if err != nil {
			grip.Errorln("problem reading request:", err)
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		event.LogProvisionFailed(instanceId, string(setupLog))
	}

	message := fmt.Sprintf(`
		Host with id %v spawned.
		The host's dns name is %v.
		To ssh in: ssh -i <your private key> %v@%v`,
		h.Id, h.Host, h.User, h.Host)

	if status == evergreen.HostStatusFailed {
		message += fmt.Sprintf("\nUnfortunately, the host's setup script did not run fully - check the setup.log " +
			"file in the machine's home directory to see more details")
	}
	err = notify.TrySendNotificationToUser(h.StartedBy, "Your host is ready", message, notify.ConstructMailer(as.Settings.Notify))
	grip.ErrorWhenln(err != nil, "Error sending email", err)

	as.WriteJSON(w, http.StatusOK, spawnResponse{HostInfo: *h})
}

// returns info on the host specified
func (as *APIServer) hostInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceId := vars["instance_id"]

	h, err := host.FindOne(host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	as.WriteJSON(w, http.StatusOK, spawnResponse{HostInfo: *h})
}

// returns info on all of the hosts spawned by a user
func (as *APIServer) hostsInfoForUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	user := vars["user"]

	hosts, err := host.Find(host.ByUserWithUnterminatedStatus(user))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, spawnResponse{Hosts: hosts})
}

func (as *APIServer) modifyHost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceId := vars["instance_id"]
	hostAction := r.FormValue("action")

	h, err := host.FindOne(host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if h == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	user := GetUser(r)
	if user == nil || user.Id != h.StartedBy {
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

		cloudHost, err := cloud.GetCloudHost(h, &as.Settings)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if err = cloudHost.TerminateInstance(); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Failed to terminate spawn host"))
			return
		}
		as.WriteJSON(w, http.StatusOK, spawnResponse{HostInfo: *h})
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action %v", hostAction), http.StatusBadRequest)
	}

}

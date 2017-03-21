package service

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/spawn"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
)

type spawnRequest struct {
	Id     string `bson:"_id" json:"id"`
	User   string `bson:"user" json:"user"`
	Distro string `bson:"distro" json:"distro"`
	Status string `bson:"status" json:"status"`
	Host   string `bson:"host" json:"host"` // foreign key
}

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
	err := util.ReadJSONInto(r.Body, &hostRequest)
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

	opts := spawn.Options{
		Distro:    hostRequest.Distro,
		UserName:  user.Id,
		PublicKey: hostRequest.PublicKey,
		UserData:  hostRequest.UserData,
	}

	spawner := spawn.New(&as.Settings)
	err = spawner.Validate(opts)
	if err != nil {
		errCode := http.StatusBadRequest
		if _, ok := err.(spawn.BadOptionsErr); !ok {
			errCode = http.StatusInternalServerError
		}
		as.LoggedError(w, r, errCode, fmt.Errorf("Spawn request failed validation: %v", err))
		return
	}

	err = spawner.CreateHost(opts, user)
	if err != nil {
		grip.Error(err)
		mailErr := notify.TrySendNotificationToUser(opts.UserName, "Spawning failed", err.Error(),
			notify.ConstructMailer(as.Settings.Notify))
		if mailErr != nil {
			grip.Errorln("Failed to send notification:", mailErr)
		}
		return
	}

	as.WriteJSON(w, http.StatusOK, "")
}

func (as *APIServer) spawnHostReady(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceId := vars["instance_id"]
	status := vars["status"]

	// mark the host itself as provisioned
	host, err := host.FindOne(host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if host == nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}

	if status == evergreen.HostStatusSuccess {
		if err := host.SetRunning(); err != nil {
			grip.Errorf("Error marking host id %s as %s: %+v",
				instanceId, evergreen.HostStatusSuccess, err)
		}
	} else {
		alerts.RunHostProvisionFailTriggers(host)
		if err = host.SetDecommissioned(); err != nil {
			grip.Errorf("Error marking host %s for user %s as decommissioned: %+v",
				host.Host, host.StartedBy, err)
		}
		grip.Infof("Decommissioned %s for user %s because provisioning failed",
			host.Host, host.StartedBy)

		// send notification to the Evergreen team about this provisioning failure
		subject := fmt.Sprintf("%v Spawn provisioning failure on %v", notify.ProvisionFailurePreface, host.Distro.Id)
		message := fmt.Sprintf("Provisioning failed on %v host %v for user %v", host.Distro.Id, host.Host, host.StartedBy)
		if err = notify.NotifyAdmins(subject, message, &as.Settings); err != nil {
			grip.Errorln("issue sending email:", err)
		}

		// get/store setup logs
		setupLog, err := ioutil.ReadAll(r.Body)
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
		host.Id, host.Host, host.User, host.Host)

	if status == evergreen.HostStatusFailed {
		message += fmt.Sprintf("\nUnfortunately, the host's setup script did not run fully - check the setup.log " +
			"file in the machine's home directory to see more details")
	}
	err = notify.TrySendNotificationToUser(host.StartedBy, "Your host is ready", message, notify.ConstructMailer(as.Settings.Notify))
	grip.ErrorWhenln(err != nil, "Error sending email", err)

	as.WriteJSON(w, http.StatusOK, spawnResponse{HostInfo: *host})
}

// returns info on the host specified
func (as *APIServer) hostInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instanceId := vars["instance_id"]

	host, err := host.FindOne(host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if host == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	as.WriteJSON(w, http.StatusOK, spawnResponse{HostInfo: *host})
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

	host, err := host.FindOne(host.ById(instanceId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if host == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	user := GetUser(r)
	if user == nil || user.Id != host.StartedBy {
		message := fmt.Sprintf("Only %v is authorized to terminate this host", host.StartedBy)
		http.Error(w, message, http.StatusUnauthorized)
		return
	}

	switch hostAction {
	case "terminate":
		if host.Status == evergreen.HostTerminated {
			message := fmt.Sprintf("Host %v is already terminated", host.Id)
			http.Error(w, message, http.StatusBadRequest)
			return
		}

		cloudHost, err := providers.GetCloudHost(host, &as.Settings)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if err = cloudHost.TerminateInstance(); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Failed to terminate spawn host: %v", err))
			return
		}
		as.WriteJSON(w, http.StatusOK, spawnResponse{HostInfo: *host})
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action %v", hostAction), http.StatusBadRequest)
	}

}

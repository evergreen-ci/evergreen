package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/spawn"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const (
	HostPasswordUpdate         = "updateRDPPassword"
	HostExpirationExtension    = "extendHostExpiration"
	HostTerminate              = "terminate"
	MaxExpirationDurationHours = 24 * 7 // 7 days
)

func (uis *UIServer) spawnPage(w http.ResponseWriter, r *http.Request) {

	var spawnDistro *distro.Distro
	var spawnTask *task.Task
	var err error
	if len(r.FormValue("distro_id")) > 0 {
		spawnDistro, err = distro.FindOne(distro.ById(r.FormValue("distro_id")))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error finding distro %v", r.FormValue("distro_id")))
			return
		}
	}
	if len(r.FormValue("task_id")) > 0 {
		spawnTask, err = task.FindOne(task.ById(r.FormValue("task_id")))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error finding task %v", r.FormValue("task_id")))
			return
		}
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		Distro          *distro.Distro
		Task            *task.Task
		MaxHostsPerUser int
		ViewData
	}{spawnDistro, spawnTask, spawn.MaxPerUser, uis.GetCommonViewData(w, r, false, true)}, "base", "spawned_hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) getSpawnedHosts(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	hosts, err := host.Find(host.ByUserWithRunningStatus(user.Username()))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "Error finding running hosts for user %v", user.Username()))
		return
	}

	uis.WriteJSON(w, http.StatusOK, hosts)
}

func (uis *UIServer) getUserPublicKeys(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	uis.WriteJSON(w, http.StatusOK, user.PublicKeys())
}

func (uis *UIServer) listSpawnableDistros(w http.ResponseWriter, r *http.Request) {
	// load in the distros
	distros, err := distro.Find(distro.All)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error loading distros"))
		return
	}

	distroList := []map[string]interface{}{}

	for _, d := range distros {
		if d.SpawnAllowed {
			distroList = append(distroList, map[string]interface{}{
				"name": d.Id,
			})
		}
	}
	uis.WriteJSON(w, http.StatusOK, distroList)
}

func (uis *UIServer) requestNewHost(w http.ResponseWriter, r *http.Request) {
	authedUser := MustHaveUser(r)

	putParams := struct {
		Task      string `json:"task_id"`
		Distro    string `json:"distro"`
		KeyName   string `json:"key_name"`
		PublicKey string `json:"public_key"`
		SaveKey   bool   `json:"save_key"`
		UserData  string `json:"userdata"`
	}{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &putParams); err != nil {
		http.Error(w, fmt.Sprintf("Bad json in request: %v", err), http.StatusBadRequest)
		return
	}

	// save the supplied public key if needed
	if putParams.SaveKey {
		if err := authedUser.AddPublicKey(putParams.KeyName, putParams.PublicKey); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error saving public key"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Public key successfully saved."))
	}
	hc := &data.DBHostConnector{}
	spawnHost, err := hc.NewIntentHost(putParams.Distro, putParams.PublicKey, putParams.Task, authedUser)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error spawning host"))
		return
	}
	if spawnHost == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Spawned host is nil"))
		return
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host spawned"))
	uis.WriteJSON(w, http.StatusOK, "Host successfully spawned")
}

func (uis *UIServer) modifySpawnHost(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	updateParams := restModel.APISpawnHostModify{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &updateParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hostId := string(updateParams.HostID)
	h, err := host.FindOne(host.ById(hostId))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error finding host with id %v", hostId))
		return
	}
	if h == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("No host with id %v found", hostId))
		return
	}

	if u.Username() != h.StartedBy {
		if !auth.IsSuperUser(uis.Settings.SuperUsers, u) {
			uis.LoggedError(w, r, http.StatusUnauthorized, errors.New("not authorized to modify this host"))
			return
		}
	}

	// determine what action needs to be taken
	switch updateParams.Action {
	case HostTerminate:
		if h.Status == evergreen.HostTerminated {
			uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Host %v is already terminated", h.Id))
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		if err := spawn.TerminateHost(ctx, h, evergreen.GetEnvironment().Settings(), u.Id); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		uis.WriteJSON(w, http.StatusOK, "host terminated")
		return

	case HostPasswordUpdate:
		pwd := string(updateParams.RDPPwd)
		if !h.Distro.IsWindows() {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("rdp password can only be set on Windows hosts"))
			return
		}
		if !spawn.ValidateRDPPassword(pwd) {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Invalid password"))
			return
		}
		if err := spawn.SetHostRDPPassword(context.TODO(), h, pwd); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host RDP password successfully updated."))
		uis.WriteJSON(w, http.StatusOK, "Successfully updated host password")
		return

	case HostExpirationExtension:
		addtHours, err := strconv.Atoi(string(updateParams.AddHours))
		if err != nil {
			http.Error(w, "bad hours param", http.StatusBadRequest)
			return
		}
		var futureExpiration time.Time
		futureExpiration, err = spawn.MakeExtendedHostExpiration(h, time.Duration(addtHours)*time.Hour)
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if err := h.SetExpirationTime(futureExpiration); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error extending host expiration time"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Host expiration "+
			"extension successful; %v will expire on %v", hostId,
			futureExpiration.Format(time.RFC850))))
		uis.WriteJSON(w, http.StatusOK, "Successfully extended host expiration time")
		return

	default:
		http.Error(w, fmt.Sprintf("Unrecognized action: %v", updateParams.Action), http.StatusBadRequest)
		return
	}
}

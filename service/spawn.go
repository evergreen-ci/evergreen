package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var (
	HostPasswordUpdate         = "updateRDPPassword"
	HostExpirationExtension    = "extendHostExpiration"
	HostTerminate              = "terminate"
	MaxExpirationDurationHours = 24 * 7 // 7 days
)

func (uis *UIServer) spawnPage(w http.ResponseWriter, r *http.Request) {

	var spawnDistro distro.Distro
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

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Distro          distro.Distro
		Task            *task.Task
		MaxHostsPerUser int
		ViewData
	}{spawnDistro, spawnTask, cloud.MaxSpawnHostsPerUser, uis.GetCommonViewData(w, r, false, true)}, "base", "spawned_hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) getSpawnedHosts(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	hosts, err := host.Find(host.ByUserWithRunningStatus(user.Username()))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "Error finding running hosts for user %v", user.Username()))
		return
	}

	gimlet.WriteJSON(w, hosts)
}

func (uis *UIServer) getUserPublicKeys(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	gimlet.WriteJSON(w, user.PublicKeys())
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
	gimlet.WriteJSON(w, distroList)
}

func (uis *UIServer) requestNewHost(w http.ResponseWriter, r *http.Request) {
	authedUser := MustHaveUser(r)

	putParams := struct {
		Task          string     `json:"task_id"`
		Distro        string     `json:"distro"`
		KeyName       string     `json:"key_name"`
		PublicKey     string     `json:"public_key"`
		SaveKey       bool       `json:"save_key"`
		UserData      string     `json:"userdata"`
		UseTaskConfig bool       `json:"use_task_config"`
		InstanceTags  []host.Tag `json:"instance_tags"`
	}{}

	err := util.ReadJSONInto(util.NewRequestReader(r), &putParams)
	if err != nil {
		http.Error(w, fmt.Sprintf("Bad json in request: %v", err), http.StatusBadRequest)
		return
	}

	// save the supplied public key if needed
	if putParams.SaveKey {
		if err = authedUser.AddPublicKey(putParams.KeyName, putParams.PublicKey); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error saving public key"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Public key successfully saved."))
	}
	hc := &data.DBConnector{}
	options := &restModel.HostRequestOptions{
		DistroID:     putParams.Distro,
		KeyName:      putParams.PublicKey,
		TaskID:       putParams.Task,
		UserData:     putParams.UserData,
		InstanceTags: putParams.InstanceTags,
	}
	spawnHost, err := hc.NewIntentHost(options, authedUser)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error spawning host"))
		return
	}
	if spawnHost == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Spawned host is nil"))
		return
	}
	if putParams.UseTaskConfig {
		task, err := task.FindOneNoMerge(task.ById(putParams.Task))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Error finding task"))
			return
		}
		err = hc.CreateHostsFromTask(task, *authedUser, putParams.PublicKey)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error creating hosts from task",
				"task":    task.Id,
			}))
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Error creating hosts from task"))
			return
		}
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host spawned"))
	gimlet.WriteJSON(w, "Host successfully spawned")
}

func (uis *UIServer) modifySpawnHost(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	updateParams := restModel.APISpawnHostModify{}
	ctx := r.Context()

	if err := util.ReadJSONInto(util.NewRequestReader(r), &updateParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hostId := restModel.FromAPIString(updateParams.HostID)
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

	if updateParams.Action == nil {
		http.Error(w, "no action specified", http.StatusBadRequest)
		return
	}
	env := evergreen.GetEnvironment()
	// determine what action needs to be taken
	switch *updateParams.Action {
	case HostTerminate:
		if h.Status == evergreen.HostTerminated {
			gimlet.WriteJSONError(w, fmt.Sprintf("Host %v is already terminated", h.Id))
			return
		}
		var cancel func()
		ctx, cancel = context.WithCancel(r.Context())
		defer cancel()

		if err := cloud.TerminateSpawnHost(ctx, h, env.Settings(), u.Id); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		gimlet.WriteJSON(w, "host terminated")
		return

	case HostPasswordUpdate:
		pwd := restModel.FromAPIString(updateParams.RDPPwd)
		if !h.Distro.IsWindows() {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("rdp password can only be set on Windows hosts"))
			return
		}
		if !cloud.ValidateRDPPassword(pwd) {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Invalid password"))
			return
		}
		if err := cloud.SetHostRDPPassword(ctx, env, h, pwd); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		gimlet.WriteJSON(w, "Successfully updated host password")
		return

	case HostExpirationExtension:
		if updateParams.Expiration.Before(h.ExpirationTime) {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Expiration can only be extended."))
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("expiration can only be extended"))
			return
		}
		addtTime := updateParams.Expiration.Sub(h.ExpirationTime)
		var futureExpiration time.Time
		futureExpiration, err = cloud.MakeExtendedSpawnHostExpiration(h, addtTime)
		if err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash(err.Error()))
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if err := h.SetExpirationTime(futureExpiration); err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error updating host expiration time"))
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error extending host expiration time"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Host expiration successfully set to %s",
			futureExpiration.Format(time.RFC822))))
		gimlet.WriteJSON(w, "Successfully extended host expiration time")
		return

	default:
		http.Error(w, fmt.Sprintf("Unrecognized action: %v", updateParams.Action), http.StatusBadRequest)
		return
	}
}

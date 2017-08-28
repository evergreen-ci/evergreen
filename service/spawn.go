package service

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/spawn"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	HostPasswordUpdate         = "updateRDPPassword"
	HostExpirationExtension    = "extendHostExpiration"
	HostTerminate              = "terminate"
	MaxExpirationDurationHours = 24 * 7 // 7 days
)

func (uis *UIServer) spawnPage(w http.ResponseWriter, r *http.Request) {
	flashes := PopFlashes(uis.CookieStore, r, w)
	projCtx := MustHaveProjectContext(r)

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
		ProjectData     projectContext
		User            *user.DBUser
		Flashes         []interface{}
		Distro          *distro.Distro
		Task            *task.Task
		MaxHostsPerUser int
	}{projCtx, GetUser(r), flashes, spawnDistro, spawnTask, spawn.MaxPerUser}, "base", "spawned_hosts.html", "base_angular.html", "menu.html")
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
				"name":             d.Id,
				"userDataFile":     d.UserData.File,
				"userDataValidate": d.UserData.Validate})
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
		dbuser, err := user.FindOne(user.ById(authedUser.Username()))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching user"))
			return
		}
		err = model.AddUserPublicKey(dbuser.Id, putParams.KeyName, putParams.PublicKey)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error saving public key"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Public key successfully saved."))
	}
	hc := &data.DBHostConnector{}
	spawnHost, err := hc.NewIntentHost(putParams.Distro, putParams.PublicKey, putParams.Task, putParams.UserData, authedUser)

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
	return

}

func (uis *UIServer) modifySpawnHost(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveUser(r)
	updateParams := struct {
		Action   string `json:"action"`
		HostId   string `json:"host_id"`
		RDPPwd   string `json:"rdp_pwd"`
		AddHours string `json:"add_hours"`
	}{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &updateParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hostId := updateParams.HostId
	host, err := host.FindOne(host.ById(hostId))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error finding host with id %v", hostId))
		return
	}
	if host == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("No host with id %v found", hostId))
		return
	}
	// determine what action needs to be taken
	switch updateParams.Action {
	case HostTerminate:
		if host.Status == evergreen.HostTerminated {
			uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Host %v is already terminated", host.Id))
			return
		}
		cloudHost, err := providers.GetCloudHost(host, &uis.Settings)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if err = cloudHost.TerminateInstance(); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		uis.WriteJSON(w, http.StatusOK, "host terminated")
		return
	case HostPasswordUpdate:
		pwdUpdateCmd, err := constructPwdUpdateCommand(&uis.Settings, host, updateParams.RDPPwd)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error constructing host RDP password"))
			return
		}

		// update RDP and sshd password
		if err = pwdUpdateCmd.Run(context.TODO()); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error updating host RDP password"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host RDP password successfully updated."))
		uis.WriteJSON(w, http.StatusOK, "Successfully updated host password")
	case HostExpirationExtension:
		addtHours, err := strconv.Atoi(updateParams.AddHours)
		if err != nil {
			http.Error(w, "bad hours param", http.StatusBadRequest)
			return
		}
		// ensure this request is valid
		addtHourDuration := time.Duration(addtHours) * time.Hour
		futureExpiration := host.ExpirationTime.Add(addtHourDuration)
		expirationExtensionDuration := futureExpiration.Sub(time.Now()).Hours()
		if expirationExtensionDuration > MaxExpirationDurationHours {
			http.Error(w, fmt.Sprintf("Can not extend %v expiration by %v hours. "+
				"Maximum extension is limited to %v hours", hostId,
				int(expirationExtensionDuration), MaxExpirationDurationHours), http.StatusBadRequest)
			return

		}
		if err = host.SetExpirationTime(futureExpiration); err != nil {
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

// constructPwdUpdateCommand returns a RemoteCommand struct used to
// set the RDP password on a remote windows machine.
func constructPwdUpdateCommand(settings *evergreen.Settings, hostObj *host.Host,
	password string) (*subprocess.RemoteCommand, error) {

	cloudHost, err := providers.GetCloudHost(hostObj, settings)
	if err != nil {
		return nil, err
	}

	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return nil, err
	}

	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, err
	}

	outputLineHandler := evergreen.NewInfoLoggingWriter(&evergreen.Logger)
	errorLineHandler := evergreen.NewErrorLoggingWriter(&evergreen.Logger)

	updatePwdCmd := fmt.Sprintf("net user %v %v && sc config "+
		"sshd obj= '.\\%v' password= \"%v\"", hostObj.User, password,
		hostObj.User, password)

	// construct the required termination command
	remoteCommand := &subprocess.RemoteCommand{
		CmdString:       updatePwdCmd,
		Stdout:          outputLineHandler,
		Stderr:          errorLineHandler,
		LoggingDisabled: true,
		RemoteHostName:  hostInfo.Hostname,
		User:            hostObj.User,
		Options:         append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:      false,
	}
	return remoteCommand, nil
}

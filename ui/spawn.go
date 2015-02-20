package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"10gen.com/mci/notify"
	"10gen.com/mci/spawn"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"net/http"
	"strconv"
	"time"
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

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *model.DBUser
		Flashes     []interface{}
	}{projCtx, GetUser(r), flashes}, "base", "spawned_hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) getSpawnedHosts(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	hosts, err := host.Find(host.ByUserWithRunningStatus(user.Username()))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			fmt.Errorf("Error finding running hosts for user %v: %v", user.Username(), err))
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
	distros, err := model.LoadDistros(uis.MCISettings.ConfigDir)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error loading distros: %v", err))
		return
	}

	distroList := []map[string]interface{}{}

	for _, distro := range distros {
		if distro.SpawnAllowed {
			distroList = append(distroList, map[string]interface{}{
				"name":             distro.Name,
				"userDataFile":     distro.SpawnUserData.File,
				"userDataValidate": distro.SpawnUserData.Validate})
		}
	}
	uis.WriteJSON(w, http.StatusOK, distroList)
}

func (uis *UIServer) requestNewHost(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	putParams := struct {
		Distro    string `json:"distro"`
		KeyName   string `json:"key_name"`
		PublicKey string `json:"public_key"`
		SaveKey   bool   `json:"save_key"`
		UserData  string `json:"userdata"`
	}{}

	if err := util.ReadJSONInto(r.Body, &putParams); err != nil {
		http.Error(w, fmt.Sprintf("Bad json in request: %v", err), http.StatusBadRequest)
		return
	}

	opts := spawn.Options{
		Distro:    putParams.Distro,
		UserName:  user.Username(),
		PublicKey: putParams.PublicKey,
		UserData:  putParams.UserData,
	}

	spawner := spawn.New(&uis.MCISettings)

	if err := spawner.Validate(opts); err != nil {
		errCode := http.StatusBadRequest
		if _, ok := err.(spawn.BadOptionsErr); !ok {
			errCode = http.StatusInternalServerError
		}
		uis.LoggedError(w, r, errCode, err)
		return
	}

	// save the supplied public key if needed
	if putParams.SaveKey {
		dbuser, err := model.FindOneDBUser(user.Username())
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error fetching user: %v", err))
			return
		}
		err = dbuser.AddPublicKey(putParams.KeyName, putParams.PublicKey)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error saving public key: %v", err))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Public key successfully saved."))
	}

	// Start a background goroutine that handles host creation/setup.
	go func() {
		host, err := spawner.CreateHost(opts)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "error spawning host: %v", err)
			mailErr := notify.TrySendNotificationToUser(user.Email(), fmt.Sprintf("Spawning failed"),
				err.Error(), notify.ConstructMailer(uis.MCISettings.Notify))
			if mailErr != nil {
				mci.Logger.Logf(slogger.ERROR, "Failed to send notification: %v", mailErr)
			}
			if host != nil { // a host was inserted - we need to clean it up
				dErr := host.SetDecommissioned()
				if err != nil {
					mci.Logger.Logf(slogger.ERROR, "Failed to set host %v decommissioned: %v", host.Id, dErr)
				}
			}
			return
		}
	}()

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

	if err := util.ReadJSONInto(r.Body, &updateParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hostId := updateParams.HostId
	host, err := host.FindOne(host.ById(hostId))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error finding host with id %v: %v", hostId, err))
		return
	}
	if host == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("No host with id %v found", hostId))
		return
	}
	// determine what action needs to be taken
	switch updateParams.Action {
	case HostTerminate:
		if host.Status == mci.HostTerminated {
			uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Host %v is already terminated", host.Id))
			return
		}
		cloudHost, err := providers.GetCloudHost(host, &uis.MCISettings)
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
		pwdUpdateCmd, err := constructPwdUpdateCommand(&uis.MCISettings, host, updateParams.RDPPwd)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error constructing host RDP password: %v", err))
			return
		}
		// update RDP and sshd password
		if err = pwdUpdateCmd.Run(); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error updating host RDP password: %v", err))
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
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error extending host expiration time: %v", err))
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
func constructPwdUpdateCommand(mciSettings *mci.MCISettings, hostObj *host.Host,
	password string) (*command.RemoteCommand, error) {

	cloudHost, err := providers.GetCloudHost(hostObj, mciSettings)
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

	outputLineHandler := mci.NewInfoLoggingWriter(&mci.Logger)
	errorLineHandler := mci.NewErrorLoggingWriter(&mci.Logger)

	updatePwdCmd := fmt.Sprintf("net user %v %v && sc config "+
		"sshd obj= '.\\%v' password= \"%v\"", hostObj.User, password,
		hostObj.User, password)

	// construct the required termination command
	remoteCommand := &command.RemoteCommand{
		CmdString:      updatePwdCmd,
		Stdout:         outputLineHandler,
		Stderr:         errorLineHandler,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     false,
	}
	return remoteCommand, nil
}

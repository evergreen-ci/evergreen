package hostutil

import (
	"io/ioutil"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
)

const HostCheckTimeout = 10 * time.Second

//CheckSSHResponse runs a test command over SSH to check whether or not the host
//appears to be up and accepting ssh connections. Returns true/false if the check
//passes or fails, or an error if the command cannot be attempted.
func CheckSSHResponse(hostObject *host.Host, sshOptions []string) (bool, error) {
	hostInfo, err := util.ParseSSHInfo(hostObject.Host)
	if err != nil {
		return false, err
	}

	if hostInfo.User == "" {
		hostInfo.User = hostObject.User
	}

	// construct a command to check reachability
	remoteCommand := &subprocess.RemoteCommand{
		CmdString:       "echo hi",
		Stdout:          ioutil.Discard,
		Stderr:          ioutil.Discard,
		RemoteHostName:  hostInfo.Hostname,
		User:            hostInfo.User,
		Options:         append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:      false,
		LoggingDisabled: true,
	}

	done := make(chan error)
	err = remoteCommand.Start()
	if err != nil {
		return false, err
	}

	go func() {
		done <- remoteCommand.Wait()
	}()

	select {
	case <-time.After(HostCheckTimeout):
		grip.Warning(remoteCommand.Stop())
		return false, nil
	case err = <-done:
		if err != nil {
			return false, nil
		}
		return true, nil
	}
}

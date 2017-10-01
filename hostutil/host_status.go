package hostutil

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"golang.org/x/net/context"
)

const HostCheckTimeout = 10 * time.Second

//CheckSSHResponse runs a test command over SSH to check whether or not the host
//appears to be up and accepting ssh connections. Returns true/false if the check
//passes or fails, or an error if the command cannot be attempted.
func CheckSSHResponse(ctx context.Context, hostObject *host.Host, sshOptions []string) (bool, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, HostCheckTimeout)
	defer cancel()

	hostInfo, err := util.ParseSSHInfo(hostObject.Host)
	if err != nil {
		return false, err
	}

	if hostInfo.User == "" {
		hostInfo.User = hostObject.User
	}

	// construct a command to check reachability
	remoteCommand := &subprocess.RemoteCommand{
		Id:             fmt.Sprintf("reachability check %s", hostObject.Id),
		CmdString:      "echo hi",
		Stdout:         ioutil.Discard,
		Stderr:         ioutil.Discard,
		RemoteHostName: hostInfo.Hostname,
		User:           hostInfo.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     false,
	}

	err = remoteCommand.Start(ctx)
	if err != nil {
		return false, err
	}

	if err = remoteCommand.Run(ctx); err != nil {
		return false, nil
	}

	return true, nil
}

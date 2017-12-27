package hostutil

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const hostCheckTimeout = 10 * time.Second

//CheckSSHResponse runs a test command over SSH to check whether or not the host
//appears to be up and accepting ssh connections. Returns true/false if the check
//passes or fails, or an error if the command cannot be attempted.
func CheckSSHResponse(ctx context.Context, hostObject *host.Host, sshOptions []string) (bool, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, hostCheckTimeout)
	defer cancel()

	hostInfo, err := util.ParseSSHInfo(hostObject.Host)
	if err != nil {
		return false, errors.Wrap(err, "problem parsing ssh info for ")
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

	done := make(chan error)

	if err = remoteCommand.Start(ctx); err != nil {
		return false, errors.Wrap(err, "problem starting command")
	}

	go func() {
		select {
		case done <- remoteCommand.Wait():
			return
		case <-ctx.Done():
			return
		}
	}()

	select {
	case <-ctx.Done():
		grip.Warning(remoteCommand.Stop())
		return false, nil
	case err = <-done:
		if err != nil {
			return false, errors.Wrap(err, "error during host check operation")
		}
		return true, nil
	}
}

package hostutil

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
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
	remoteCommand := subprocess.NewRemoteCommand(
		"echo hi",
		hostInfo.Hostname,
		hostInfo.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostInfo.Port}, sshOptions...),
		false, // logging disabled
	)
	if err = remoteCommand.SetOutput(subprocess.OutputOptions{SuppressOutput: true, SuppressError: true}); err != nil {
		return false, errors.Wrap(err, "problem configuring output")
	}

	if err = remoteCommand.Run(ctx); err != nil {
		return false, errors.Wrapf(err, "reachability command encountered error for %s", hostObject.Id)
	}

	return true, nil
}

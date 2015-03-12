package model

import (
	"10gen.com/mci"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	"time"
)

// TODO:
//	This is a temporary package for storing host-related interactions that involve
//	multiple models. We will break this up once MCI-2221 is finished.

// NextTaskForHost the next task that should be run on the host.
func NextTaskForHost(h *host.Host) (*Task, error) {
	taskQueue, err := FindTaskQueueForDistro(h.Distro)
	if err != nil {
		return nil, err
	}

	if taskQueue == nil || taskQueue.IsEmpty() {
		return nil, nil
	}

	nextTaskId := taskQueue.Queue[0].Id
	fullTask, err := FindTask(nextTaskId)
	if err != nil {
		return nil, err
	}

	return fullTask, nil
}

// Make sure the static hosts stored in the database are correct.
func RefreshStaticHosts(configName string) error {

	distros, err := distro.Load(configName)
	if err != nil {
		return err
	}

	activeStaticHosts := make([]string, 0)
	for _, distro := range distros {
		for _, hostId := range distro.Hosts {
			hostInfo, err := util.ParseSSHInfo(hostId)
			if err != nil {
				return err
			}
			user := hostInfo.User
			if user == "" {
				user = distro.User
			}
			staticHost := host.Host{
				Id:           hostId,
				User:         user,
				Host:         hostId,
				Distro:       distro.Name,
				CreationTime: time.Now(),
				Provider:     mci.HostTypeStatic,
				StartedBy:    mci.MCIUser,
				InstanceType: "",
				Status:       mci.HostRunning,
				Provisioned:  true,
			}

			// upsert the host
			_, err = staticHost.Upsert()
			if err != nil {
				return err
			}
			activeStaticHosts = append(activeStaticHosts, hostId)
		}
	}
	return host.DecommissionInactiveStaticHosts(activeStaticHosts)
}

package model

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers/static"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"time"
)

// TODO:
//	This is a temporary package for storing host-related interactions that involve
//	multiple models. We will break this up once MCI-2221 is finished.

// NextTaskForHost the next task that should be run on the host.
func NextTaskForHost(h *host.Host) (*Task, error) {
	taskQueue, err := FindTaskQueueForDistro(h.Distro.Id)
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

func UpdateStaticHosts(e *mci.MCISettings) error {
	distros, err := distro.Find(distro.ByProvider(static.ProviderName))
	if err != nil {
		return err
	}
	activeStaticHosts := make([]string, 0)
	settings := &static.Settings{}

	for _, d := range distros {
		err = mapstructure.Decode(d.ProviderSettings, settings)
		if err != nil {
			return fmt.Errorf("invalid static settings for '%v'", d.Id)
		}
		for _, h := range settings.Hosts {
			hostInfo, err := util.ParseSSHInfo(h.Name)
			if err != nil {
				return err
			}
			user := hostInfo.User
			if user == "" {
				user = d.User
			}
			staticHost := host.Host{
				Id:           h.Name,
				User:         user,
				Host:         h.Name,
				Distro:       d,
				CreationTime: time.Now(),
				Provider:     mci.HostTypeStatic,
				StartedBy:    mci.MCIUser,
				Status:       mci.HostRunning,
				Provisioned:  true,
			}

			// upsert the host
			_, err = staticHost.Upsert()
			if err != nil {
				return err
			}
			activeStaticHosts = append(activeStaticHosts, h.Name)
		}
	}
	return host.DecommissionInactiveStaticHosts(activeStaticHosts)
}

package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/static"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

// TODO(MCI-2245):
//	This is a temporary package for storing host-related interactions that involve multiple models.

// NextTaskForHost the next task that should be run on the host.
func NextTaskForHost(h *host.Host) (*task.Task, error) {
	taskQueue, err := FindTaskQueueForDistro(h.Distro.Id)
	if err != nil {
		return nil, err
	}

	if taskQueue == nil || taskQueue.IsEmpty() {
		return nil, nil
	}

	nextTaskId := taskQueue.Queue[0].Id
	fullTask, err := task.FindOne(task.ById(nextTaskId))
	if err != nil {
		return nil, err
	}

	return fullTask, nil
}

func UpdateStaticHosts(e *evergreen.Settings) error {
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
				Provider:     evergreen.HostTypeStatic,
				StartedBy:    evergreen.User,
				Status:       evergreen.HostRunning,
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

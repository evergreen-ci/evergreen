package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	dynamicDistroRuntimeAlertThreshold = 24 * time.Hour
)

type Configuration struct {
	DistroID         string
	TaskFinder       string
	FreeHostFraction float64
}

func PlanDistro(ctx context.Context, conf Configuration, s *evergreen.Settings) error {
	schedulerInstanceID := utility.RandomString()

	distro, err := distro.FindOne(distro.ById(conf.DistroID))
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	if err = underwaterUnschedule(distro.Id); err != nil {
		return errors.Wrap(err, "problem unscheduling underwater tasks")
	}

	if distro.Disabled {
		// we can just clear these queues, the tasks will persist
		// and get rescheduled once the distro is no longer disabled
		var queue_info model.DistroQueueInfo
		queue_info, err = model.GetDistroQueueInfo(distro.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "cannot get distro queue information for disabled distro",
				"distro":  distro.Id,
			}))
		}
		if queue_info.Length > 0 {
			err = model.ClearTaskQueue(distro.Id)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "cannot clear task queue for disabled distro",
					"distro":  distro.Id,
				}))
			}
			grip.Info(message.Fields{
				"distro":      distro.Id,
				"removed_len": queue_info.Length,
				"operation":   "removed queue of disabled distro",
			})
		}
		grip.InfoWhen(sometimes.Quarter(), message.Fields{
			"message": "scheduling for distro is disabled",
			"runner":  RunnerName,
			"distro":  distro.Id,
		})
		return nil
	}

	if _, err = distro.GetResolvedPlannerSettings(s); err != nil {
		return errors.WithStack(err)
	}

	////////////////////
	// task-finder phase
	////////////////////

	taskFindingBegins := time.Now()
	finder := GetTaskFinder(conf.TaskFinder)
	tasks, err := finder(distro)
	if err != nil {
		return errors.Wrapf(err, "problem while running task finder for distro '%s'", distro.Id)
	}
	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
		"operation":     "runtime-stats",
		"phase":         "task-finder",
		"instance":      schedulerInstanceID,
		"duration_secs": time.Since(taskFindingBegins).Seconds(),
	})

	/////////////////
	// planning phase
	/////////////////

	planningPhaseBegins := time.Now()
	prioritizedTasks, err := PrioritizeTasks(&distro, tasks, TaskPlannerOptions{
		StartedAt:        taskFindingBegins,
		ID:               schedulerInstanceID,
		IsSecondaryQueue: false,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
		"alias":         false,
		"operation":     "runtime-stats",
		"phase":         "planning-distro",
		"instance":      schedulerInstanceID,
		"duration_secs": time.Since(planningPhaseBegins).Seconds(),
		"stat":          "distro-queue-size",
		"size":          len(prioritizedTasks),
		"input_size":    len(tasks),
	})

	return nil
}

func UpdateStaticDistro(d distro.Distro) error {
	if d.Provider != evergreen.ProviderNameStatic {
		return nil
	}

	hosts, err := doStaticHostUpdate(d)
	if err != nil {
		return errors.WithStack(err)
	}

	if d.Id == "" {
		return nil
	}

	return host.MarkInactiveStaticHosts(hosts, d.Id)
}

func doStaticHostUpdate(d distro.Distro) ([]string, error) {
	settings := &cloud.StaticSettings{}
	if err := settings.FromDistroSettings(d, ""); err != nil {
		return nil, errors.Wrapf(err, "invalid static settings for '%s'", d.Id)
	}

	staticHosts := []string{}
	for _, h := range settings.Hosts {
		hostInfo, err := util.ParseSSHInfo(h.Name)
		if err != nil {
			return nil, err
		}
		user := hostInfo.User
		if user == "" {
			user = d.User
		}

		dbHost, err := host.FindOneId(h.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "error finding host named %s", h.Name)
		}
		provisionChange := needsReprovisioning(d, dbHost)

		provisioned := provisionChange == host.ReprovisionNone || (dbHost != nil && dbHost.Provisioned)
		staticHost := host.Host{
			Id:               h.Name,
			User:             user,
			Host:             h.Name,
			Distro:           d,
			CreationTime:     time.Now(),
			StartedBy:        evergreen.User,
			NeedsReprovision: provisionChange,
			Provisioned:      provisioned,
		}
		if dbHost == nil || dbHost.Status == evergreen.HostTerminated {
			if provisioned {
				staticHost.Status = evergreen.HostRunning
			} else {
				staticHost.Status = evergreen.HostProvisioning
			}
			if dbHost != nil {
				event.LogHostStatusChanged(dbHost.Id, dbHost.Status, staticHost.Status, evergreen.User, "host status changed by host allocator")
			}
		} else {
			staticHost.Status = dbHost.Status
		}

		if d.Provider == evergreen.ProviderNameStatic {
			staticHost.Provider = evergreen.HostTypeStatic
		}

		// upsert the host
		_, err = staticHost.Upsert()
		if err != nil {
			return nil, err
		}
		staticHosts = append(staticHosts, h.Name)
	}

	return staticHosts, nil
}

// needsReprovisioning checks if the host needs to be reprovisioned.
func needsReprovisioning(d distro.Distro, h *host.Host) host.ReprovisionType {
	if h == nil {
		if d.BootstrapSettings.Method != "" && d.BootstrapSettings.Method != distro.BootstrapMethodLegacySSH {
			return host.ReprovisionToNew
		}
		return host.ReprovisionNone
	}

	// If the host has already been marked as needing reprovisioning before but
	// has not performed reprovisioning yet, preserve the transition.
	if h.NeedsReprovision != host.ReprovisionNone {
		if d.LegacyBootstrap() && h.NeedsReprovision == host.ReprovisionToLegacy {
			return h.NeedsReprovision
		}
		if !d.LegacyBootstrap() {
			if h.NeedsReprovision == host.ReprovisionToNew || h.NeedsReprovision == host.ReprovisionJasperRestart {
				return h.NeedsReprovision
			}
		}
		return host.ReprovisionNone
	}

	// Transition the host to legacy or non-legacy depending on current distro
	// settings.
	if h.Distro.LegacyBootstrap() && d.BootstrapSettings.Method != "" && d.BootstrapSettings.Method != distro.BootstrapMethodLegacySSH {
		return host.ReprovisionToNew
	}

	if !h.Distro.LegacyBootstrap() && (d.BootstrapSettings.Method == "" || d.BootstrapSettings.Method == distro.BootstrapMethodLegacySSH) {
		return host.ReprovisionToLegacy
	}

	return host.ReprovisionNone
}

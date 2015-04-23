package cleanup

import (
	"10gen.com/mci"
	"10gen.com/mci/apimodels"
	"10gen.com/mci/cloud"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	"10gen.com/mci/notify"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"math"
	"strconv"
	"sync"
	"time"
)

const (
	MaxIdleTime                = 5 * time.Minute
	MinTimeToNextPayment       = 5 * time.Minute
	MaxTimeToMarkAsProvisioned = 35 * time.Minute
	MaxTimeToBeUnproductive    = 60 * time.Minute
	TaskTimeOutDuration        = 7 * time.Minute
	HungTasksDuration          = 12 * time.Hour
	SlowProvisionWarn          = 25 * time.Minute
	LateProvisionWarning       = "late-provision-warning"
)

var (
	// notify the user of immiment termination 2 and 12 hours
	// before the host is actually terminated. Note that these
	// thresholds should be supplied in increasing order of duration
	hostExpirationNotificationThresholds = []time.Duration{
		time.Duration(2) * time.Hour,
		time.Duration(12) * time.Hour,
	}
	terminateTimeoutDuration = time.Duration(10) * time.Second
)

// TerminateHosts is a helper function that terminates instances
// that were started in the cloud
func TerminateHosts(hosts []model.Host, mciSettings *mci.MCISettings) {
	for _, host := range hosts {
		cloudHost, err := providers.GetCloudHost(&host, mciSettings)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Failed to get cloud host for %v: %v", host.Id, err)
			continue
		}

		mci.Logger.Logf(slogger.DEBUG, "Terminating %v host (%v) %v with provider %v",
			host.Distro, host.Id, host.Host, host.Provider)

		err = cloudHost.TerminateInstance()
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Unable to terminate host (%v): "+
				"%v", host.Id, err)
			continue
		}
	}
}

// TerminateRunningTask gets a hung host and terminates the task
// running on each of those hosts. It also optionally marks such running tasks
// as failed
func TerminateRunningTask(mciSettings *mci.MCISettings, host model.Host,
	markFail bool) {

	// get the distro for the host
	distro, err := model.LoadOneDistro(mciSettings.ConfigDir, host.Distro)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error locating distro %v: %v",
			host.Distro, err)
		return
	}

	if distro == nil {
		mci.Logger.Errorf(slogger.ERROR, "No distro exists with name %v",
			host.Distro)
		return
	}

	task, err := model.FindTask(host.RunningTask)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error locating running task %v: %v",
			host.RunningTask, err)
		return
	}

	if task == nil {
		mci.Logger.Errorf(slogger.ERROR, "Task with id %v does not exist",
			host.RunningTask)
		return
	}

	// get the distro's kill pid command
	killPIDCmd, exists := distro.Expansions["kill_pid"]
	if !exists {
		mci.Logger.Errorf(slogger.ERROR, "Could not find kill_pid command for "+
			"distro %v", distro.Name)
		return
	}

	// get the required kill command to be run on the remote agent
	remoteAgentCommand, err := constructKillAgentCommand(mciSettings, &host,
		killPIDCmd)
	if remoteAgentCommand == nil || err != nil {
		mci.Logger.Logf(slogger.ERROR, "Could not construct agent kill command for host %v (skipping): %v", host.Id, err)
	} else {
		mci.Logger.Logf(slogger.INFO, "Running kill command '%v' on host %v",
			remoteAgentCommand.CmdString, host.Id)
		// kill the agent and its children
		err = util.RunFunctionWithTimeout(
			remoteAgentCommand.Run,
			10*time.Second,
		)
		if err != nil {
			if err == util.ErrTimedOut {
				mci.Logger.Errorf(slogger.ERROR, "Agent kill command timed out")
				if err := remoteAgentCommand.Stop(); err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error stopping agent kill"+
						" command: %v", err)
				}
				return
			}
			mci.Logger.Errorf(slogger.ERROR, "Error killing agent on host %v: %v",
				host.Id, err)
			return
		}
	}

	// mark the task as failed if required
	if markFail {
		markEndRequest := &apimodels.TaskEndRequest{Status: mci.TaskFailed}

		project, err := model.FindProject("", task.Project, mciSettings.ConfigDir)
		if err != nil {
			message := fmt.Sprintf("Unable to load config for project %v: %v",
				task.Project, err)
			mci.Logger.Errorf(slogger.ERROR, message)
			return
		}

		err = task.MarkEnd(mci.CleanupPackage, time.Now(), markEndRequest, project)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "MarkEnd returned an error for "+
				"task %v: %v", task.Id, err)
			return
		}

		// update the given host's running_task field accordingly
		if err := host.UpdateRunningTask(task.Id, "", time.Now()); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error updating running task "+
				"%v on host %v to '': %v", task.Id, host.Id, err)
			return
		}
	}

	mci.Logger.Logf(slogger.INFO, "Successfully terminated stalled task %v on "+
		"%v", host.RunningTask, host.Host)
}

// TerminateIdleHosts gets a slice of idle hosts and terminates them
func TerminateIdleHosts(mciSettings *mci.MCISettings) {
	// first, we want to find currently idle hosts
	hosts, err := model.FindFreeHosts()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Could not find idle hosts: %v", err)
		return
	}

	currentTime := time.Now()
	mci.Logger.Logf(slogger.DEBUG, "Finding idle host(s) to be reaped...")
	idleHosts, err := GetIdleHosts(currentTime, hosts)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get idle hosts: %v", err)
		return
	}
	mci.Logger.Logf(slogger.INFO, "Found %v idle host(s) to be terminated",
		len(idleHosts))
	TerminateHosts(idleHosts, mciSettings)
}

// TerminateStaleHosts gets a slice of all spawned hosts and terminates them
// if they are stale
func TerminateStaleHosts(mciSettings *mci.MCISettings) (staleHosts []model.Host) {

	mci.Logger.Logf(slogger.DEBUG, "Finding stale spawned hosts...")
	// first, we want to find all spawned hosts
	spawnedHosts, err := model.FindRunningSpawnedHosts()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get spawned hosts: %v", err)
		return
	}

	staleHosts = make([]model.Host, 0)

	// we'll need this to wait for all the hosts to get terminated
	waitGroup := &sync.WaitGroup{}

	for _, host := range spawnedHosts {
		// terminate the host if its expired
		expirationDur := int(host.ExpirationTime.Sub(time.Now()).Minutes())
		if expirationDur <= 0 {
			staleHosts = append(staleHosts, host)
			mci.Logger.Logf(slogger.INFO, "Will terminate stale host %v "+
				"started by %v", host.Id, host.StartedBy)
			terminateExpiredHost(host, waitGroup, mciSettings)
			continue
		}

		// check if a notification's due for this host
		shouldSend, notificationThreshold := shouldSendExpirationNotification(
			&host, hostExpirationNotificationThresholds)

		if !shouldSend {
			continue
		}

		if err = notifySpawnHostUser(&host, mciSettings); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error sending expiration "+
				"reminder for %v started by %v: %v", host.Id, host.StartedBy,
				err)
			continue
		}

		// update the host to indicate this notification has been sent
		thresholdKey := strconv.Itoa(int(notificationThreshold.Minutes()))
		if err = host.SetExpirationNotification(thresholdKey); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error updating %v's host (%v) "+
				"expiration notification map: %v", host.StartedBy, host.Id, err)
			continue
		}
	}

	// wait for all stale hosts to get terminated
	waitGroup.Wait()
	mci.Logger.Logf(slogger.INFO, "Terminated %v stale host(s)",
		len(staleHosts))
	return
}

// TerminateDecommissionedHosts gets a slice of decommissioned hosts and
// terminates them
func TerminateDecommissionedHosts(mciSettings *mci.MCISettings) {
	// first, we want to find hosts that have been set as decommissioned
	decommissionedHosts, err := GetDecommissionedHosts()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get decommissioned hosts: "+
			"%v", err)
		return
	}
	mci.Logger.Logf(slogger.INFO, "Found %v decommissioned host(s) to be "+
		"terminated", len(decommissionedHosts))
	TerminateHosts(decommissionedHosts, mciSettings)
}

// TerminateHungTasks gets a slice of hung
// hosts, terminates the tasks that are
// running on them, and then frees the hosts
func TerminateHungTasks(mciSettings *mci.MCISettings) {
	currentTime := time.Now()
	mci.Logger.Logf(slogger.DEBUG, "Finding hung host tasks...")

	hungHosts, err := GetHungHosts(currentTime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to find hung hosts: %v", err)
		return
	}

	mci.Logger.Logf(slogger.INFO, "Found %v hung host(s) to be freed",
		len(hungHosts))

	for _, host := range hungHosts {
		// terminate hung task on the host
		mci.Logger.Logf(slogger.INFO, "Terminating running task on %v host %v",
			host.Distro, host.Host)
		TerminateRunningTask(mciSettings, host, true)
	}
}

// TerminateExcessHosts gets a slice of excess hosts and terminates them
func TerminateExcessHosts(mciSettings *mci.MCISettings) {
	mci.Logger.Logf(slogger.DEBUG, "Finding excess hosts for distros...")

	// get all distros
	distros, err := model.LoadDistros(mciSettings.ConfigDir)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to read distros directory:"+
			" %v", err)
		return
	}

	excessHosts, err := GetExcessHosts(distros)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get excess hosts: %v", err)
		return
	}

	excessHostCount := 0
	for _, hostsForDistro := range excessHosts {
		excessHostCount += len(hostsForDistro)
	}

	mci.Logger.Logf(slogger.INFO, "Found %v excess host(s) to be terminated",
		excessHostCount)
	for distroName, hostsForDistro := range excessHosts {
		mci.Logger.Logf(slogger.INFO, "Terminating %v %v host(s)",
			len(hostsForDistro), distroName)
		// terminate hosts for each distro
		TerminateHosts(hostsForDistro, mciSettings)
	}
}

// TerminateUnproductiveHosts gets a slice of unproductive hosts and
// terminates them
func TerminateUnproductiveHosts(mciSettings *mci.MCISettings) {
	currentTime := time.Now()
	mci.Logger.Logf(slogger.DEBUG, "Finding unproductive hosts...")
	hosts, err := GetUnproductiveHosts(currentTime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get unproductive hosts: "+
			"%v", err)
		return
	}
	mci.Logger.Logf(slogger.INFO, "Found %v unproductive host(s) to be "+
		"terminated", len(hosts))
	TerminateHosts(hosts, mciSettings)
}

func WarnSlowProvisioningHosts(mciSettings *mci.MCISettings) {
	mci.Logger.Logf(slogger.DEBUG, "Finding late unprovisioned hosts...")
	currentTime := time.Now()
	hosts, err := GetUnprovisionedHosts(currentTime, SlowProvisionWarn)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get late unprovisioned hosts: "+
			"%v", err)
		return
	}

	for _, host := range hosts {
		if !shouldSendLateProvisionWarning(&host) {
			continue
		}
		hostLink := fmt.Sprintf("%v/host/%v", mciSettings.Ui.Url, host.Id)
		subject := fmt.Sprintf("%v MCI provisioning taking a long time on %v",
			notify.ProvisionLatePreface, host.Distro)
		message := fmt.Sprintf("Provisioning is taking a long time on %v host %v. See %v",
			host.Distro, host.Id, hostLink)
		if err = notify.NotifyAdmins(subject, message, mciSettings); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error sending provision warn email for host %v: %v",
				host.Id, err)
			continue
		}

		//Record the notification as sent so it won't be sent again on next run
		if err = host.SetExpirationNotification(LateProvisionWarning); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Failed to update notifications map for host %v: %v",
				host.Id, err)
		}
	}
}

// TerminateUnprovisionedHosts gets a slice of unprovisioned hosts and
// terminates them
func TerminateUnprovisionedHosts(mciSettings *mci.MCISettings) {
	currentTime := time.Now()
	mci.Logger.Logf(slogger.DEBUG, "Finding unprovisioned hosts...")
	hosts, err := GetUnprovisionedHosts(currentTime, MaxTimeToMarkAsProvisioned)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Unable to get unprovisioned hosts: "+
			"%v", err)
		return
	}

	// send notification to the MCI team about this provisioning time out
	for _, host := range hosts {
		subject := fmt.Sprintf("%v MCI provisioning timed out on %v",
			notify.ProvisionTimeoutPreface, host.Distro)

		hostLink := fmt.Sprintf("%v/host/%v", mciSettings.Ui.Url, host.Id)
		message := fmt.Sprintf("Provisioning timed out on %v host %v. See %v",
			host.Distro, host.Id, hostLink)
		if err = notify.NotifyAdmins(subject, message, mciSettings); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error sending provision time "+
				"out email for host %v: %v", host.Id, err)
		}
	}

	mci.Logger.Logf(slogger.INFO, "Found %v unprovisioned host(s) to be "+
		"terminated", len(hosts))
	TerminateHosts(hosts, mciSettings)
}

// CheckTimeouts returns the ids of the timed-out tasks
func CheckTimeouts(mciSettings *mci.MCISettings) []string {
	mci.Logger.Logf(slogger.INFO, "Checking task timeouts...")
	timedOutTasks := []string{}

	// find all currently running tasks
	inProgressTasks, err := model.FindInProgressTasks()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error finding in-progress tasks: %v",
			err)
		return timedOutTasks
	}

	mci.Logger.Logf(slogger.INFO, "There are %v in-progress tasks",
		len(inProgressTasks))

	for _, task := range inProgressTasks {
		mci.Logger.Logf(slogger.DEBUG, "Checking timeout for task %v", task.Id)

		// get the task's host
		host, err := model.FindHost(task.HostId)
		if err != nil || host == nil {
			mci.Logger.Errorf(slogger.ERROR, "Unable to find host with id %v",
				task.HostId)
			continue
		}

		// if the last heartbeat > x time before now:
		if task.LastHeartbeat.Add(TaskTimeOutDuration).Before(time.Now()) {
			mci.Logger.Logf(slogger.INFO, "Task %v timed out", task.Id)
			timedOutTasks = append(timedOutTasks, task.Id)
			if host.RunningTask != task.Id {
				mci.Logger.Errorf(slogger.ERROR, "Task %v is not running on "+
					"host %v - %v is!", task.Id, host.Id, host.RunningTask)
				continue
			}

			// terminate the heartbeat-timed-out-task on the host machine
			mci.Logger.Logf(slogger.INFO, "Terminating running task on %v "+
				"host %v", host.Distro, host.Host)
			TerminateRunningTask(mciSettings, *host, false)

			project, err := model.FindProject("", task.Project, mciSettings.ConfigDir)
			if err != nil {
				message := fmt.Sprintf("Unable to load config for project %v: "+
					"%v", task.Project, err)
				mci.Logger.Errorf(slogger.ERROR, message)
				continue
			}

			taskEndRequest := &apimodels.TaskEndRequest{
				Status:        mci.TaskFailed,
				StatusDetails: apimodels.TaskEndDetails{"heartbeat", true},
			}

			err = task.TryReset("", mci.CleanupPackage, project, taskEndRequest)
			if err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error resetting task %v: %v",
					task.Id, err)
			}

			// then clear the host's running_task field
			err = host.UpdateRunningTask(task.Id, "", time.Now())
			if err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error updating running task "+
					"%v on host %v to '': %v", task.Id, host.Id, err)
			}

			cloudHost, err := providers.GetCloudHost(host, mciSettings)
			if err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Failed to get cloud host for: %v", host.Id)
				continue
			}

			cloudStatus, err := cloudHost.GetInstanceStatus()
			if err != nil {
				mci.Logger.Logf(slogger.ERROR, "Error getting cloud status for host %v: %v", host.Id, err)
				continue
			}

			if cloudStatus == cloud.StatusTerminated {
				//The instance is already destroyed. Update our database to
				//reflect that.
				mci.Logger.Logf(slogger.ERROR,
					"Host %v already terminated according to provider '%v': "+
						"Marking as terminated.", host.Id, host.Provider)
				err = host.SetTerminated()
				if err != nil {
					mci.Logger.Logf(slogger.ERROR, "Failed to set host %v terminated.")
				}
			} else if cloudStatus == cloud.StatusRunning {
				// The instance is still up, according to the provider itself.
				// Check if it's unreachable via the network.
				mci.Logger.Errorf(slogger.INFO, "Host %v is running according to provider '%v', "+
					"checking if reachable with SSH", host.Id, host.Provider)
				reachable, err := cloudHost.IsSSHReachable()
				if err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error checking if host %v "+
						"is reachable: %v", host.Host, err)
					continue
				}

				if reachable {
					mci.Logger.Logf(slogger.INFO, "Host %v is running, but the "+
						"agent stopped reporting", host.Host)

					//If the host is decommissioned or quarantined, leave it
					if host.Status == mci.HostDecommissioned ||
						host.Status == mci.HostQuarantined {
						continue
					}
					//otherwise, we can return it to the pool to pick up a task
					err = host.SetRunning()
					if err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error setting host "+
							"status for %v: %v", host.Host, err)
					}
					continue
				}

				// At this point, the host has failed our reachability check.
				mci.Logger.Logf(slogger.INFO, "host %v is not heartbeating "+
					"and is unresponsive to SSH: marking unreachable.", host.Id)

				//If the host is decommissioned or quarantined, leave it
				if host.Status == mci.HostDecommissioned ||
					host.Status == mci.HostQuarantined {
					continue
				}

				err = host.SetUnreachable()
				if err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error setting host "+
						"status for host %v: %v", host.Host, err)
				}
				subject := fmt.Sprintf("Host %v not reachable", host.Id)
				message := fmt.Sprintf("Unable to reach host %v (%v). Host "+
					"event page at %v/host/%v", host.Id, host.Host,
					mciSettings.Ui.Url, host.Id)
				err = notify.NotifyAdmins(subject, message, mciSettings)
				if err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
				}
			} else {
				mci.Logger.Logf(slogger.ERROR, "Unexpected: host %v is in state %v"+
					" according to provider %v", host.Id, cloudStatus, host.Provider)
			}
		}
	}
	return timedOutTasks
}

//***********************************************************\/
//      Helper functions to get hosts to be terminated       \/
//***********************************************************\/
// GetDecommissionedHosts returns a slice of hosts
// that have been marked as decommissioned
func GetDecommissionedHosts() ([]model.Host, error) {
	hosts, err := model.FindDecommissionedHosts()
	if err != nil {
		return nil, fmt.Errorf("Unable to find decommissioned hosts: %v", err)
	}
	return hosts, nil
}

// GetExcessHosts returns a slice of hosts that
// execced a given distro's MaxHosts
func GetExcessHosts(distros map[string]model.Distro) (map[string][]model.Host,
	error) {
	// excess hosts grouped by distro
	excessHosts := make(map[string][]model.Host)
	excessHostCount := 0

	for distroName, distroInfo := range distros {
		allDistroHosts, err := model.FindHostsForDistro(distroName)
		if err != nil {
			return nil, fmt.Errorf("FindHostsForDistro function call returned "+
				"error: %v", err)
		}
		// find out if we have more hosts than are allowed for this distro
		if len(allDistroHosts) > distroInfo.MaxHosts {
			for _, distroHost := range allDistroHosts {
				// pick up hosts that aren't currently running
				// any tasks for this distro
				if distroHost.RunningTask == "" {
					excessHosts[distroName] = append(excessHosts[distroName],
						distroHost)
					excessHostCount += 1
				}
				// only pick up as many idle hosts as are needed to satisfy
				// maxhosts
				if len(allDistroHosts)-excessHostCount == distroInfo.MaxHosts {
					break
				}
			}
		}
		excessHostCount = 0
	}
	return excessHosts, nil
}

// GetIdleHosts returns a slice of hosts that
// have been idle based on currentTime,
// MaxIdleTime and MinTimeToNextPayment
func GetIdleHosts(currentTime time.Time, hosts []model.Host) ([]model.Host,
	error) {
	// this implements logic to decide if a machine should be stopped
	idleHosts := make([]model.Host, 0)

	for _, host := range hosts {

		// if the host is quarantined, skip it
		if host.Status == mci.HostQuarantined {
			continue
		}

		//TODO this logic may depend on the cloud provider.
		// should have a method in CloudManager for better options here

		// in case this is the first time we are running
		// a task on this machine
		if host.LastTaskCompleted != "" && host.RunningTask == "" {
			// when was the last task completed
			idleTime := currentTime.Sub(host.LastTaskCompletedTime)
			// how long since host was created
			timeSinceCreation := currentTime.Sub(host.CreationTime)
			// how long before we have to pay for another hour
			hourDurationToNextPayment := time.Duration(
				math.Ceil(timeSinceCreation.Hours()))
			// when does the next payment time start
			nextPaymentTimeStart := host.CreationTime.Add(
				hourDurationToNextPayment * time.Hour)
			// how long before then
			timeToNextPaymentStart := nextPaymentTimeStart.Sub(currentTime)
			// we only want to stop hosts that have been idle
			// for at least 15 minutes provided they are no more than
			// 10 minutes to the next hour (when we'll have to pay a full hour)
			// This logic could be supplied in a config file or somewhere else
			if idleTime >= MaxIdleTime &&
				timeToNextPaymentStart <= MinTimeToNextPayment {
				idleHosts = append(idleHosts, host)
			}
		}
	}
	return idleHosts, nil
}

// GetUnproductiveHosts returns a slice of hosts that
// have not been productive based on currentTime and
// MaxTimeToBeUnproductive
func GetUnproductiveHosts(currentTime time.Time) ([]model.Host, error) {
	threshold := currentTime.Add(-MaxTimeToBeUnproductive)
	hosts, err := model.FindUnproductiveHosts(threshold)
	if err != nil {
		return nil, fmt.Errorf("Unable to find unproductive hosts: %v", err)
	}
	return hosts, nil
}

// GetHungHosts returns a slice of hosts that
// are hung based on currentTime and
// HungTasksDuration
func GetHungHosts(currentTime time.Time) ([]model.Host, error) {
	threshold := currentTime.Add(-HungTasksDuration)
	hosts, err := model.FindHungHosts(threshold)
	if err != nil {
		return nil, fmt.Errorf("Unable to find hung hosts: %v", err)
	}
	return hosts, nil
}

// GetUnprovisionedHosts returns a slice of hosts that
// have not been provisioned within cutoff time
func GetUnprovisionedHosts(currentTime time.Time, cutoff time.Duration) ([]model.Host, error) {
	threshold := currentTime.Add(-cutoff)
	unprovisionedHosts, err := model.FindUnprovisionedHosts(threshold)
	if err != nil {
		return nil, fmt.Errorf("Unable to find unprovisioned hosts: %v", err)
	}
	hosts := make([]model.Host, 0)
	for _, host := range unprovisionedHosts {
		if host.Status == mci.HostQuarantined {
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

// constructKillAgentCommand returns a RemoteCommand struct used to terminate
// the agent on a given host
func constructKillAgentCommand(mciSettings *mci.MCISettings, host *model.Host,
	killPIDCmd string) (terminateAgentCommand *command.RemoteCommand, err error) {
	cloudHost, err := providers.GetCloudHost(host, mciSettings)
	if err != nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Failed to get cloud host for %v: %v",
			host.Id, err)
	}

	hostInfo, err := util.ParseSSHInfo(host.Host)
	if hostInfo.User == "" {
		hostInfo.User = host.User
	}

	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, mci.Logger.Errorf(slogger.ERROR, "Error getting ssh options for host %v: %v", host.Id, err)
	}

	outputLineHandler := mci.NewInfoLoggingWriter(&mci.Logger)
	errorLineHandler := mci.NewErrorLoggingWriter(&mci.Logger)

	// construct the required termination command
	return &command.RemoteCommand{
		CmdString:      fmt.Sprintf(killPIDCmd, host.Pid),
		Stdout:         outputLineHandler,
		Stderr:         errorLineHandler,
		RemoteHostName: hostInfo.Hostname,
		User:           host.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     false,
	}, nil
}

// notifySpawnHostUser sends a notification to the user who requested a spawn
// host reminding them that their host will be terminated soon
func notifySpawnHostUser(hostObj *model.Host,
	mciSettings *mci.MCISettings) error {
	message := fmt.Sprintf("Host with instance id %v will be terminated at "+
		"%v (in %v). If you wish to keep this host active, please visit %v to "+
		"extend its lifetime.", hostObj.Id,
		hostObj.ExpirationTime.Format(time.RFC850),
		hostObj.ExpirationTime.Sub(time.Now()),
		mciSettings.Ui.Url+"/spawn")

	subject := fmt.Sprintf("Your %v host (%v) termination reminder",
		hostObj.Distro, hostObj.Id)

	err := notify.TrySendNotificationToUser(
		hostObj.StartedBy,
		subject,
		message,
		notify.ConstructMailer(mciSettings.Notify),
	)
	if err != nil {
		return fmt.Errorf("Error sending email to %v for host %v: %v",
			hostObj.StartedBy, hostObj.Id, err)
	}
	return nil
}

func shouldSendLateProvisionWarning(host *model.Host) bool {
	//Check if we already sent a warning for this host
	alreadySent := host.Notifications
	if _, hasKey := alreadySent[LateProvisionWarning]; hasKey {
		//This host has had a warning sent already
		return false
	}
	return true
}

// shouldSendExpirationNotification determines if a expiration warning
// notification is due to be sent based on the values in notificationThreshold
func shouldSendExpirationNotification(host *model.Host,
	notificationThresholds []time.Duration) (shouldSend bool,
	notificationThreshold time.Duration) {

	// find how long long the given host has until its expiration
	timeToExpiration := host.ExpirationTime.Sub(time.Now())

	for _, notificationThreshold := range notificationThresholds {
		// a threshold notification should be sent based on the host's
		// expiration time and the absence of an already sent notification
		// on that threshold's key
		notificationKey := strconv.Itoa(int(notificationThreshold.Minutes()))
		_, notificationSent := host.Notifications[notificationKey]
		hostExpiringWithinThreshold := timeToExpiration <= notificationThreshold
		if hostExpiringWithinThreshold && !notificationSent {
			return true, notificationThreshold
		}
		// if an earlier notification has already been sent, don't send any
		// later ones
		if notificationSent {
			break
		}
	}
	return false, notificationThreshold
}

// terminateExpiredHost terminates a running host and times out if the
// termination command does not complete within the terminateTimeoutDuration
func terminateExpiredHost(host model.Host, waitGroup *sync.WaitGroup,
	mciSettings *mci.MCISettings) {
	cloudHost, err := providers.GetCloudHost(&host, mciSettings)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Failed to get cloud host for %v", host.Id)
		return
	}

	// run the command to terminate the host within a goroutine
	// and with a timeout
	waitGroup.Add(1)

	go func() {
		terminateFunc := func() error {
			defer waitGroup.Done()
			if err := cloudHost.TerminateInstance(); err != nil {
				return err
			}
			return nil
		}

		err = util.RunFunctionWithTimeout(
			terminateFunc,
			terminateTimeoutDuration,
		)

		if err == util.ErrTimedOut {
			mci.Logger.Errorf(slogger.ERROR, "Timeout terminating "+
				"host %v", host.Id)
			return
		}
	}()
}

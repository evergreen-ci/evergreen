package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	idleHostJobName = "idle-host-termination"

	// idleTimeCutoff is the amount of time we wait for an idle host to be marked as idle.
	idleTimeCutoff = 4 * time.Minute
	// outdatedIdleTimeCutoff is the amount of time we wait for an outdated idle host to be marked idle.
	outdatedIdleTimeCutoff    = time.Minute
	idleWaitingForAgentCutoff = 10 * time.Minute
	idleTaskGroupHostCutoff   = 10 * time.Minute

	// MaxTimeNextPayment is the amount of time we wait to have left before marking a host as idle
	maxTimeTilNextPayment = 5 * time.Minute
)

func init() {
	registry.AddJobType(idleHostJobName, func() amboy.Job {
		return makeIdleHostJob()
	})
}

type idleHostJob struct {
	job.Base        `bson:"metadata" json:"metadata" yaml:"metadata"`
	Terminated      int      `bson:"terminated" json:"terminated" yaml:"terminated"`
	TerminatedHosts []string `bson:"terminated_hosts" json:"terminated_hosts" yaml:"terminated_hosts"`

	env evergreen.Environment
}

func makeIdleHostJob() *idleHostJob {
	j := &idleHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    idleHostJobName,
				Version: 0,
			},
		},
	}
	return j
}

func NewIdleHostTerminationJob(env evergreen.Environment, id string) amboy.Job {
	j := makeIdleHostJob()
	j.env = env
	j.SetPriority(2)
	j.SetID(fmt.Sprintf("%s.%s", idleHostJobName, id))
	return j
}

func (j *idleHostJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.HasErrors() {
		return
	}

	// Each DistroID's idleHosts are sorted from oldest to newest CreationTime.
	distroHosts, err := host.IdleEphemeralGroupedByDistroID()
	if err != nil {
		j.AddError(errors.Wrap(err, "database error grouping idle hosts by Distro.Id"))
		return
	}

	distroIDsToFind := make([]string, 0, len(distroHosts))
	for _, info := range distroHosts {
		distroIDsToFind = append(distroIDsToFind, info.DistroID)
	}
	distrosFound, err := distro.Find(distro.ByIds(distroIDsToFind))
	if err != nil {
		j.AddError(errors.Wrapf(err, "database error for find() by distro ids in [%s]", strings.Join(distroIDsToFind, ",")))
		return
	}

	if len(distroIDsToFind) > len(distrosFound) {
		distroIDsFound := make([]string, 0, len(distrosFound))
		for _, d := range distrosFound {
			distroIDsFound = append(distroIDsFound, d.Id)
		}
		missingDistroIDs := utility.GetSetDifference(distroIDsToFind, distroIDsFound)
		hosts, err := host.Find(db.Query(host.ByDistroIDs(missingDistroIDs...)))
		if err != nil {
			j.AddError(errors.Wrapf(err, "could not find hosts in missing distros: %s", strings.Join(missingDistroIDs, ", ")))
			return
		}
		for _, h := range hosts {
			j.AddError(errors.Wrapf(h.SetDecommissioned(evergreen.User, false, "distro is missing"), "could not set host '%s' as decommissioned", h.Id))
		}

		if j.HasErrors() {
			return
		}
	}

	distrosMap := make(map[string]distro.Distro, len(distrosFound))
	for i := range distrosFound {
		d := distrosFound[i]
		distrosMap[d.Id] = d
	}
	for _, info := range distroHosts {
		minimumHostsForDistro := distrosMap[info.DistroID].HostAllocatorSettings.MinimumHosts
		minNumHostsToEvaluate := getMinNumHostsToEvaluate(info, minimumHostsForDistro)

		currentDistro := distrosMap[info.DistroID]
		hostsToEvaluateForTermination := make([]host.Host, 0, minNumHostsToEvaluate)
		for i := 0; i < len(info.IdleHosts); i++ {
			if len(hostsToEvaluateForTermination) >= minNumHostsToEvaluate {
				// If we've reached the number that we need to terminate, only terminate hosts with outdated AMIs.
				if !hostHasOutdatedAMI(info.IdleHosts[i], currentDistro) {
					continue
				}
			}
			hostsToEvaluateForTermination = append(hostsToEvaluateForTermination, info.IdleHosts[i])
			j.AddError(j.checkAndTerminateHost(ctx, &info.IdleHosts[i], currentDistro))
		}

		grip.InfoWhen(sometimes.Percent(10), message.Fields{
			"id":                             j.ID(),
			"job_type":                       idleHostJobName,
			"runner":                         "monitor",
			"op":                             "dispatcher",
			"distro_id":                      info.DistroID,
			"minimum_hosts":                  minimumHostsForDistro,
			"num_running_hosts":              info.RunningHostsCount,
			"num_idle_hosts":                 len(info.IdleHosts),
			"min_num_idle_hosts_to_evaluate": minNumHostsToEvaluate,
			"num_idle_hosts_to_evaluate":     len(hostsToEvaluateForTermination),
		})
	}
}

func getMinNumHostsToEvaluate(info host.IdleHostsByDistroID, minimumHosts int) int {
	totalRunningHosts := info.RunningHostsCount
	numIdleHosts := len(info.IdleHosts)

	maxHostsToTerminate := totalRunningHosts - minimumHosts
	if maxHostsToTerminate <= 0 {
		// Even if we're at or below minimum hosts, we should still continue to check hosts with outdated AMIs.
		return 0
	}
	if numIdleHosts > maxHostsToTerminate {
		return maxHostsToTerminate
	}
	return numIdleHosts
}

func (j *idleHostJob) checkAndTerminateHost(ctx context.Context, h *host.Host, d distro.Distro) error {

	exitEarly, err := checkTerminationExemptions(ctx, h, j.env, j.Type().Name, j.ID())
	if exitEarly {
		return err
	}

	idleTime := h.IdleTime()
	communicationTime := h.GetElapsedCommunicationTime()

	idleThreshold := idleTimeCutoff
	if h.RunningTaskGroup != "" {
		idleThreshold = idleTaskGroupHostCutoff
	} else if hostHasOutdatedAMI(*h, d) {
		idleThreshold = outdatedIdleTimeCutoff
	}

	// if we haven't heard from the host or it's been idle for longer than the cutoff, we should terminate
	var terminateReason string
	if communicationTime >= idleThreshold {
		terminateReason = fmt.Sprintf("host is idle or unreachable, communication time %s over threshold time %s", communicationTime.String(), idleThreshold)
	} else if idleTime >= idleThreshold {
		terminateReason = fmt.Sprintf("host is idle or unreachable, idle time %s over threshold time %s", idleTime, idleThreshold)
	}
	if terminateReason != "" {
		j.Terminated++
		j.TerminatedHosts = append(j.TerminatedHosts, h.Id)
		terminationJob := NewHostTerminationJob(j.env, h, HostTerminationOptions{TerminationReason: terminateReason})
		return amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), terminationJob)
	}

	return nil
}

//checkTerminationExemptions checks if some conditions apply where we shouldn't terminate an idle host,
//and returns true if some exemption applies
func checkTerminationExemptions(ctx context.Context, h *host.Host, env evergreen.Environment, jobType string, jid string) (bool, error) {
	if !h.IsEphemeral() {
		grip.Notice(message.Fields{
			"job":      jid,
			"host_id":  h.Id,
			"job_type": jobType,
			"status":   h.Status,
			"provider": h.Distro.Provider,
			"message":  "host termination for a non-ephemeral distro",
			"cause":    "programmer error",
		})
		return true, errors.Errorf("attempted to terminate non-ephemeral host '%s'", h.Id)
	}

	// ask the host how long it has been idle
	idleTime := h.IdleTime()

	// if the communication time is > 10 mins then there may not be an agent on the host.
	communicationTime := h.GetElapsedCommunicationTime()

	if h.IsWaitingForAgent() && (communicationTime < idleWaitingForAgentCutoff || idleTime < idleWaitingForAgentCutoff) {
		grip.Notice(message.Fields{
			"op":                jobType,
			"id":                jid,
			"message":           "not flagging idle host, waiting for an agent",
			"host_id":           h.Id,
			"distro":            h.Distro.Id,
			"idle":              idleTime.String(),
			"last_communicated": communicationTime.String(),
		})
		return true, nil
	}

	// get a cloud manager for the host
	mgrOpts, err := cloud.GetManagerOptions(h.Distro)
	if err != nil {
		return true, errors.Wrapf(err, "can't get ManagerOpts for host '%s'", h.Id)
	}
	manager, err := cloud.GetManager(ctx, env, mgrOpts)
	if err != nil {
		return true, errors.Wrapf(err, "error getting cloud manager for host %v", h.Id)
	}

	// ask how long until the next payment for the host
	tilNextPayment := manager.TimeTilNextPayment(h)

	if tilNextPayment > maxTimeTilNextPayment {
		return true, nil
	}

	return false, nil
}

func hostHasOutdatedAMI(h host.Host, d distro.Distro) bool {
	return h.GetAMI() != d.GetDefaultAMI()
}

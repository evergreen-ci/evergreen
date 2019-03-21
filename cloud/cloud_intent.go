package cloud

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// NewIntent creates an IntentHost using the given host settings. An IntentHost is a host that
// does not exist yet but is intended to be picked up by the hostinit package and started. This
// function takes distro information, the name of the instance, the provider of the instance and
// a HostOptions and returns an IntentHost.
func NewIntent(d distro.Distro, instanceName, provider string, options HostOptions) *host.Host {
	creationTime := time.Now()
	// proactively write all possible information pertaining
	// to the host we want to create. this way, if we are unable
	// to start it or record its instance id, we have a way of knowing
	// something went wrong - and what
	intentHost := &host.Host{
		Id:                    instanceName,
		User:                  d.User,
		Distro:                d,
		Tag:                   instanceName,
		CreationTime:          creationTime,
		Status:                evergreen.HostUninitialized,
		TerminationTime:       util.ZeroTime,
		TaskDispatchTime:      util.ZeroTime,
		Provider:              provider,
		StartedBy:             options.UserName,
		UserHost:              options.UserHost,
		HasContainers:         options.HasContainers,
		ParentID:              options.ParentID,
		ContainerPoolSettings: options.ContainerPoolSettings,
		SpawnOptions:          options.SpawnOptions,
		DockerOptions:         options.DockerOptions,
	}

	if options.ExpirationDuration != nil {
		intentHost.ExpirationTime = creationTime.Add(*options.ExpirationDuration)
	}
	if options.ProvisionOptions != nil {
		intentHost.ProvisionOptions = options.ProvisionOptions
	}
	return intentHost
}

// GenerateContainerHostIntents generates container intent documents by going
// through available parents and packing on the parents with longest expected
// finish time
func GenerateContainerHostIntents(d distro.Distro, newContainersNeeded int, hostOptions HostOptions) ([]host.Host, error) {
	parents, err := host.GetNumContainersOnParents(d)
	if err != nil {
		return nil, errors.Wrap(err, "Could not find number of containers on each parent")
	}
	containerHostIntents := make([]host.Host, 0)

	for _, parent := range parents {
		// find out how many more containers this parent can fit
		containerSpace := parent.ParentHost.ContainerPoolSettings.MaxContainers - parent.NumContainers
		containersToCreate := containerSpace
		// only create containers as many as we need
		if newContainersNeeded < containerSpace {
			containersToCreate = newContainersNeeded
		}
		for i := 0; i < containersToCreate; i++ {
			hostOptions.ParentID = parent.ParentHost.Id
			containerHostIntents = append(containerHostIntents, *NewIntent(d, d.GenerateName(), d.Provider, hostOptions))
		}
		newContainersNeeded -= containersToCreate
		if newContainersNeeded == 0 {
			return containerHostIntents, nil
		}
	}
	return containerHostIntents, nil
}

// createParents creates host intent documents for each parent
func createParents(parent distro.Distro, numNewParents int, pool *evergreen.ContainerPool) []host.Host {
	hostsSpawned := make([]host.Host, numNewParents)

	for idx := range hostsSpawned {
		hostsSpawned[idx] = *NewIntent(parent, parent.GenerateName(), parent.Provider, generateParentHostOptions(pool))
	}
	return hostsSpawned
}

// generateParentHostOptions generates host options for a parent host
func generateParentHostOptions(pool *evergreen.ContainerPool) HostOptions {
	options := HostOptions{
		HasContainers:         true,
		UserName:              evergreen.User,
		ContainerPoolSettings: pool,
	}
	return options
}

func CreateParentIntentsAndHostsToSpawn(pool *evergreen.ContainerPool, newHostsNeeded int) ([]host.Host, int, error) {
	// find all running parents with the specified container pool

	numNewParentsToSpawn, newHostsNeeded, err := host.GetNumNewParentsAndHostsToSpawn(pool, newHostsNeeded)
	if err != nil {
		return []host.Host{}, 0, errors.Wrap(err, "error getting number of parents to spawn")
	}
	newParentHosts := []host.Host{}
	if numNewParentsToSpawn > 0 {
		// get parent distro from pool
		parentDistro, err := distro.FindOne(distro.ById(pool.Distro))
		if err != nil {
			return nil, 0, errors.Wrap(err, "error find parent distro")
		}
		newParentHosts = createParents(parentDistro, numNewParentsToSpawn, pool)
	}

	return newParentHosts, newHostsNeeded, nil
}

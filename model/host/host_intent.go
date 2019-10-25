package host

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// CreateOptions is a struct of options that are commonly passed around when creating a
// new cloud host.
type CreateOptions struct {
	ProvisionOptions   *ProvisionOptions
	ExpirationDuration *time.Duration
	UserName           string
	UserHost           bool

	HasContainers         bool
	ParentID              string
	ContainerPoolSettings *evergreen.ContainerPool
	SpawnOptions          SpawnOptions
	DockerOptions         DockerOptions
	InstanceTags          []Tag
	InstanceType          string
	NoExpiration          bool
}

// NewIntent creates an IntentHost using the given host settings. An IntentHost is a host that
// does not exist yet but is intended to be picked up by the hostinit package and started. This
// function takes distro information, the name of the instance, the provider of the instance and
// a CreateOptions and returns an IntentHost.
func NewIntent(d distro.Distro, instanceName, provider string, options CreateOptions) *Host {
	creationTime := time.Now()
	// proactively write all possible information pertaining
	// to the host we want to create. this way, if we are unable
	// to start it or record its instance id, we have a way of knowing
	// something went wrong - and what
	intentHost := &Host{
		Id:                    instanceName,
		User:                  d.User,
		Distro:                d,
		Tag:                   instanceName,
		CreationTime:          creationTime,
		Status:                evergreen.HostUninitialized,
		TerminationTime:       util.ZeroTime,
		Provider:              provider,
		StartedBy:             options.UserName,
		UserHost:              options.UserHost,
		HasContainers:         options.HasContainers,
		ParentID:              options.ParentID,
		ContainerPoolSettings: options.ContainerPoolSettings,
		SpawnOptions:          options.SpawnOptions,
		DockerOptions:         options.DockerOptions,
		InstanceTags:          options.InstanceTags,
		InstanceType:          options.InstanceType,
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
func GenerateContainerHostIntents(d distro.Distro, newContainersNeeded int, hostOptions CreateOptions) ([]Host, error) {
	parents, err := GetNumContainersOnParents(d)
	if err != nil {
		return nil, errors.Wrap(err, "Could not find number of containers on each parent")
	}
	containerHostIntents := make([]Host, 0)
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
func createParents(parent distro.Distro, numNewParents int, pool *evergreen.ContainerPool) []Host {
	hostsSpawned := make([]Host, numNewParents)

	for idx := range hostsSpawned {
		hostsSpawned[idx] = *NewIntent(parent, parent.GenerateName(), parent.Provider, generateParentCreateOptions(pool))
	}
	return hostsSpawned
}

// generateParentCreateOptions generates host options for a parent host
func generateParentCreateOptions(pool *evergreen.ContainerPool) CreateOptions {
	options := CreateOptions{
		HasContainers:         true,
		UserName:              evergreen.User,
		ContainerPoolSettings: pool,
	}
	return options
}

func InsertParentIntentsAndGetNumHostsToSpawn(pool *evergreen.ContainerPool, newHostsNeeded int, ignoreMaxHosts bool) ([]Host, int, error) {
	// find all running parents with the specified container pool
	numNewParentsToSpawn, newHostsNeeded, err := getNumNewParentsAndHostsToSpawn(pool, newHostsNeeded, ignoreMaxHosts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "error getting number of parents to spawn")
	}
	if numNewParentsToSpawn <= 0 {
		return nil, newHostsNeeded, nil
	}

	// get parent distro from pool
	parentDistro, err := distro.FindOne(distro.ById(pool.Distro))
	if err != nil {
		return nil, 0, errors.Wrap(err, "error find parent distro")
	}
	newParentHosts := createParents(parentDistro, numNewParentsToSpawn, pool)
	if err = InsertMany(newParentHosts); err != nil {
		return nil, 0, errors.Wrap(err, "error inserting new parent hosts")
	}
	return newParentHosts, newHostsNeeded, nil
}

package host

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
)

const defaultMaxImagesPerParent = 3

// CreateOptions is a struct of options that are commonly passed around when creating a
// new cloud host.
type CreateOptions struct {
	Distro           distro.Distro
	ProvisionOptions *ProvisionOptions
	ExpirationTime   time.Time
	Region           string
	UserName         string
	UserHost         bool

	HasContainers         bool
	ParentID              string
	ContainerPoolSettings *evergreen.ContainerPool
	SpawnOptions          SpawnOptions
	DockerOptions         DockerOptions
	InstanceTags          []Tag
	InstanceType          string
	NoExpiration          bool
	SleepScheduleInfo
	IsVirtualWorkstation bool
	IsCluster            bool
	HomeVolumeSize       int
	HomeVolumeID         string
	IsDebug              bool
}

// NewIntent creates an intent host using the given host settings. An intent host is a host that
// does not exist yet in the cloud but will be eventually started by the system. This
// function takes distro information, the name of the instance, the provider of the instance and
// a CreateOptions and returns an intent host.
func NewIntent(options CreateOptions) *Host {
	creationTime := time.Now()
	instanceName := options.Distro.GenerateName()

	// proactively write all possible information pertaining
	// to the host we want to create. this way, if we are unable
	// to start it or record its instance id, we have a way of knowing
	// something went wrong - and what

	intentHost := &Host{
		Id:                    instanceName,
		User:                  options.Distro.User,
		Distro:                options.Distro,
		Tag:                   instanceName,
		CreationTime:          creationTime,
		Status:                evergreen.HostUninitialized,
		TerminationTime:       utility.ZeroTime,
		Provider:              options.Distro.Provider,
		StartedBy:             options.UserName,
		UserHost:              options.UserHost,
		HasContainers:         options.HasContainers,
		ParentID:              options.ParentID,
		ContainerPoolSettings: options.ContainerPoolSettings,
		SpawnOptions:          options.SpawnOptions,
		DockerOptions:         options.DockerOptions,
		InstanceTags:          options.InstanceTags,
		InstanceType:          options.InstanceType,
		IsVirtualWorkstation:  options.IsVirtualWorkstation,
		HomeVolumeSize:        options.HomeVolumeSize,
		HomeVolumeID:          options.HomeVolumeID,
		NoExpiration:          options.NoExpiration,
		SleepSchedule:         options.SleepScheduleInfo,
		ExpirationTime:        options.ExpirationTime,
		ProvisionOptions:      options.ProvisionOptions,
		IsDebug:               options.IsDebug,
	}

	return intentHost
}

func (h *Host) GetCreateOptions() CreateOptions {
	return CreateOptions{
		Distro:                h.Distro,
		UserName:              h.StartedBy,
		UserHost:              h.UserHost,
		HasContainers:         h.HasContainers,
		ParentID:              h.ParentID,
		ContainerPoolSettings: h.ContainerPoolSettings,
		SpawnOptions:          h.SpawnOptions,
		DockerOptions:         h.DockerOptions,
		InstanceTags:          h.InstanceTags,
		IsVirtualWorkstation:  h.IsVirtualWorkstation,
		HomeVolumeSize:        h.HomeVolumeSize,
		HomeVolumeID:          h.HomeVolumeID,
		NoExpiration:          h.NoExpiration,
		ExpirationTime:        h.ExpirationTime,
		ProvisionOptions:      h.ProvisionOptions,
		IsDebug:               h.IsDebug,
	}
}

// partitionParents will split parent hosts based on those that already have/will have the image for this distro
// it does not handle scenarios where the image for a distro has changed recently or multiple app servers calling this
// and racing each other, on the assumption that having to download a small number of extra images is not a big deal
func partitionParents(parents []ContainersOnParents, distro string, maxImages int) ([]ContainersOnParents, []ContainersOnParents) {
	matched := []ContainersOnParents{}
	notMatched := []ContainersOnParents{}
parentLoop:
	for _, parent := range parents {
		currentImages := map[string]bool{}
		for _, c := range parent.Containers {
			currentImages[c.Distro.Id] = true
			if c.Distro.Id == distro {
				matched = append(matched, parent)
				continue parentLoop
			}
		}
		// only return parent hosts that would be below the max if they downloaded another image
		if len(currentImages) < maxImages {
			notMatched = append(notMatched, parent)
		}
	}
	return matched, notMatched
}

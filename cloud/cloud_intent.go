package cloud

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
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
		Id:               instanceName,
		User:             d.User,
		Distro:           d,
		Tag:              instanceName,
		CreationTime:     creationTime,
		Status:           evergreen.HostUninitialized,
		TerminationTime:  util.ZeroTime,
		TaskDispatchTime: util.ZeroTime,
		Provider:         provider,
		StartedBy:        options.UserName,
		UserHost:         options.UserHost,
	}

	if options.ExpirationDuration != nil {
		intentHost.ExpirationTime = creationTime.Add(*options.ExpirationDuration)
	}
	if options.ProvisionOptions != nil {
		intentHost.ProvisionOptions = options.ProvisionOptions
	}
	return intentHost

}

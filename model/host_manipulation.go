package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// TODO(MCI-2245):
//	This is a temporary package for storing host-related interactions that involve multiple models.

func UpdateStaticHosts() error {
	distros, err := distro.Find(distro.ByProvider(evergreen.ProviderNameStatic))
	if err != nil {
		return err
	}
	activeStaticHosts := make([]string, 0)
	settings := &cloud.StaticSettings{}

	for _, d := range distros {
		err = mapstructure.Decode(d.ProviderSettings, settings)
		if err != nil {
			return errors.Errorf("invalid static settings for '%v'", d.Id)
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
	return host.MarkInactiveStaticHosts(activeStaticHosts)
}

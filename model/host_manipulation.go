package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TODO(MCI-2245):
//	This is a temporary package for storing host-related interactions that involve multiple models.

func UpdateStaticHosts() error {
	distros, err := distro.Find(distro.ByProvider(evergreen.ProviderNameStatic))
	if err != nil {
		return err
	}
	activeHosts := []string{}
	catcher := grip.NewBasicCatcher()
	for _, d := range distros {
		hosts, err := doStatcHostUpdate(d)
		if err != nil {
			catcher.Add(err)
			continue
		}
		activeHosts = append(activeHosts, hosts...)

	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	return host.MarkInactiveStaticHosts(activeHosts)
}

func UpdateStaticDistro(name string) error {
	distro, err := distro.FindOne(distro.ById(name))
	if err != nil {
		return errors.WithStack(err)
	}

	hosts, err := doStatcHostUpdate(distro)
	if err != nil {
		return errors.WithStack(err)
	}

	return host.MarkInactiveStaticHosts(hosts)
}

func doStatcHostUpdate(d distro.Distro) ([]string, error) {
	settings := &cloud.StaticSettings{}
	err := mapstructure.Decode(d.ProviderSettings, settings)
	if err != nil {
		return nil, errors.Errorf("invalid static settings for '%v'", d.Id)
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
			return nil, err
		}
		staticHosts = append(staticHosts, h.Name)
	}

	return staticHosts, nil
}

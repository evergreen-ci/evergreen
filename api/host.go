package api

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
)

// ModifyHostsWithPermissions performs an update on each of the given hosts
// for which the permissions allow updates on that host.
func ModifyHostsWithPermissions(hosts []host.Host, perm map[string]gimlet.Permissions, modifyHost func(h *host.Host) error) (updated int, err error) {
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if perm[h.Distro.Id][evergreen.PermissionHosts] < evergreen.HostsEdit.Value {
			continue
		}
		if err := modifyHost(&h); err != nil {
			catcher.Wrapf(err, "could not modify host '%s'", h.Id)
			continue
		}
		updated++
	}
	return updated, catcher.Resolve()
}

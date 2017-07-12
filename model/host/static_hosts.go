package host

import (
	"github.com/evergreen-ci/evergreen"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// DecommissionInactiveStaticHosts marks static hosts
// in the database as terminated provided their ids aren't contained in the
// passed in activeStaticHosts slice. This is called in the scheduler,
// and marks any static host in the system that was removed from the
// distro as "terminated".
//
// Previously this oepration marked these hosts as "decommissioned,"
// which is not a state that makes sense for static hosts.
func MarkInactiveStaticHosts(activeStaticHosts []string) error {
	if activeStaticHosts == nil {
		return nil
	}
	err := UpdateAll(
		bson.M{
			IdKey: bson.M{
				"$nin": activeStaticHosts,
			},
			ProviderKey: evergreen.HostTypeStatic,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostTerminated,
			},
		},
	)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

package host

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
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
//
// If the distro is the empty string ("") then this operation affects
// all distros.
func MarkInactiveStaticHosts(activeStaticHosts []string, distroID string) error {
	query := bson.M{
		IdKey:       bson.M{"$nin": activeStaticHosts},
		ProviderKey: evergreen.HostTypeStatic,
	}

	if distroID != "" {
		query[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = distroID
	}

	err := UpdateAll(
		query,
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostTerminated,
			},
		},
	)
	if err == mgo.ErrNotFound {
		return nil
	}
	return errors.Wrap(err, "could not terminate static hosts")
}

package host

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// MarkInactiveStaticHosts marks static hosts in the database as terminated
// provided their ids aren't contained in the passed in activeStaticHosts slice.
// This is called in the scheduler, and marks any static host in the system that
// was removed from the distro as "terminated".
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

	toTerminate, err := Find(db.Query(query))
	if adb.ResultsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "could not get hosts to be terminated")
	}
	catcher := grip.NewBasicCatcher()
	for _, h := range toTerminate {
		catcher.Wrapf(h.SetStatus(evergreen.HostTerminated, evergreen.User, "static host removed from distro"), "could not terminate host '%s'", h.Id)
	}

	return errors.Wrap(catcher.Resolve(), "could not terminate static hosts")
}

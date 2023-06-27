package host

import (
	"context"

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
// Previously this operation marked these hosts as "decommissioned,"
// which is not a state that makes sense for static hosts.
//
// If the distro is the empty string ("") then this operation affects all distros.
// If distro aliases are included, then this operation affects also hosts with the alias.
func MarkInactiveStaticHosts(ctx context.Context, activeStaticHosts []string, d *distro.Distro) error {
	query := bson.M{
		IdKey:       bson.M{"$nin": activeStaticHosts},
		ProviderKey: evergreen.HostTypeStatic,
	}
	if d != nil {
		if len(d.Aliases) > 0 {
			ids := append(d.Aliases, d.Id)
			query[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = bson.M{"$in": ids}
		} else {
			query[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = d.Id
		}
	}

	toTerminate, err := Find(db.Query(query))
	if adb.ResultsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "getting inactive static hosts for termination")
	}
	catcher := grip.NewBasicCatcher()
	for _, h := range toTerminate {
		catcher.Wrapf(h.SetStatus(ctx, evergreen.HostTerminated, evergreen.User, "static host removed from distro"), "terminating host '%s'", h.Id)
	}

	return errors.Wrap(catcher.Resolve(), "terminating inactive static hosts")
}

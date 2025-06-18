package hoststat

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
)

var (
	distroIDKey  = bsonutil.MustHaveTag(HostStat{}, "DistroID")
	timestampKey = bsonutil.MustHaveTag(HostStat{}, "Timestamp")
)

// Collection stores host usage statistics as time series data.
const Collection = "host_stats"

// Find finds all host stats that match the given query.
func Find(ctx context.Context, q db.Q) ([]HostStat, error) {
	stats := []HostStat{}
	err := db.FindAllQ(ctx, Collection, q, &stats)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// FindByDistroSince finds all host stats for a given distro since the start
// timestamp.
// kim: TODO: add unit test
func FindByDistroSince(ctx context.Context, distroID string, startAt time.Time) ([]HostStat, error) {
	q := db.Query(bson.M{
		distroIDKey:  distroID,
		timestampKey: bson.M{"$gte": startAt},
	})
	return Find(ctx, q)
}

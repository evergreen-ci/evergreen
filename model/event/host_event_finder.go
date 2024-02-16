package event

import (
	"context"
	"github.com/evergreen-ci/evergreen/db"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type hostStatusDistro struct {
	Count  int      `bson:"count"`
	Status []string `bson:"status"`
}

func (s *hostStatusDistro) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(s) }
func (s *hostStatusDistro) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, s) }

func getRecentStatusesForHost(ctx context.Context, hostId string, n int) (int, []string) {
	or := ResourceTypeKeyIs(ResourceTypeHost)
	or[TypeKey] = EventHostTaskFinished
	or[ResourceIdKey] = hostId

	pipeline := []bson.M{
		{"$match": or},
		{"$sort": bson.M{TimestampKey: -1}},
		{"$limit": n},
		{"$group": bson.M{
			"_id":    nil,
			"count":  bson.M{"$sum": 1},
			"status": bson.M{"$addToSet": "$" + bsonutil.GetDottedKeyName(DataKey, hostDataStatusKey)}}},
		{"$project": bson.M{
			"_id":    0,
			"count":  1,
			"status": 1}},
	}

	cursor, err := evergreen.GetEnvironment().DB().Collection(EventCollection).Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not get recent host statuses",
			"host_id": hostId,
			"count":   n,
		}))
		return 0, []string{}
	}

	hostStatusDistros := []hostStatusDistro{}
	if err := cursor.All(ctx, &hostStatusDistros); err != nil {
		grip.Warning(err)
		return 0, []string{}
	}

	if len(hostStatusDistros) != 1 {
		return 0, []string{}
	}

	return hostStatusDistros[0].Count, hostStatusDistros[0].Status
}

func AllRecentHostEventsMatchStatus(ctx context.Context, hostId string, n int, status string) bool {
	if n == 0 {
		return false
	}

	count, statuses := getRecentStatusesForHost(ctx, hostId, n)
	if count == 0 {
		return false
	}

	if count != n {
		return false
	}

	for _, stat := range statuses {
		if stat != status {
			return false
		}
	}

	return true

}

// CountFailedExecuteEvents counts the number of failed execute script events
// for a host
func CountFailedExecuteEvents(id string) (int, error) {
	filter := ResourceTypeKeyIs(ResourceTypeHost)
	filter[ResourceIdKey] = id
	filter[TypeKey] = EventHostScriptExecuteFailed

	num, err := db.CountQ(EventCollection, db.Query(filter))

	return num, err
}

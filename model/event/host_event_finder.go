package event

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
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

func getRecentFinishedStatusesForHost(ctx context.Context, hostId string, hostProvisionTime time.Time, n int) (int, []string) {
	query := ResourceTypeKeyIs(ResourceTypeHost)
	query[TypeKey] = EventHostTaskFinished
	query[ResourceIdKey] = hostId
	query[TimestampKey] = bson.M{"$gte": hostProvisionTime}

	pipeline := []bson.M{
		{"$match": query},
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

// AllRecentHostEventsAreSystemFailed returns true if all recent host events are system failures, and false if any are not.
// Only takes into account task finished events that occurred since the last time the task started running.
func AllRecentHostEventsAreSystemFailed(ctx context.Context, hostId string, hostProvisionTime time.Time, n int) bool {
	if n == 0 {
		return false
	}

	count, statuses := getRecentFinishedStatusesForHost(ctx, hostId, hostProvisionTime, n)
	if count == 0 {
		return false
	}

	if count != n {
		return false
	}

	for _, stat := range statuses {
		if !utility.StringSliceContains(evergreen.TaskSystemFailureStatuses, stat) {
			return false
		}
	}

	return true

}

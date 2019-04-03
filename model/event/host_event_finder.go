package event

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgobson "gopkg.in/mgo.v2/bson"
)

type hostStatusDistro struct {
	Count  int      `bson:"count"`
	Status []string `bson:"status"`
}

func (s *hostStatusDistro) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(s) }
func (s *hostStatusDistro) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, s) }

func getRecentStatusesForHost(hostId string, n int) (int, []string) {
	or := ResourceTypeKeyIs(ResourceTypeHost)
	or[TypeKey] = EventTaskFinished
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

	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	cursor, err := env.DB().Collection(AllLogCollection).Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		grip.Warning(message.WrapError(err,
			message.Fields{
				"op": "host stats for distro agg",
			}))
		return 0, []string{}
	}

	out := []hostStatusDistro{}
	for cursor.Next(ctx) {
		doc := hostStatusDistro{}
		if err := cursor.Decode(&doc); err != nil {
			grip.Warning(err)
			continue
		}
		out = append(out, doc)
	}
	grip.Warning(cursor.Close(ctx))

	if len(out) != 1 {
		return 0, []string{}
	}

	return out[0].Count, out[0].Status
}

func AllRecentHostEventsMatchStatus(hostId string, n int, status string) bool {
	if n == 0 {
		return false
	}

	count, statuses := getRecentStatusesForHost(hostId, n)
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

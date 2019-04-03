package event

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"go.mongodb.org/mongo-driver/bson"
)

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

	out := []struct {
		Count  int      `bson:"count"`
		Status []string `bson:"status"`
	}{}

	if err := db.Aggregate(AllLogCollection, pipeline, out); err != nil {
		return 0, []string{}
	}

	if len(out) != 1 {
		return 0, []string{}
	}

	return out[0].Count, out[0].Status
}

func AllRecentHostEventsMatchStatus(hostId string, n int, status string) bool {
	count, statuses := getRecentStatusesForHost(hostId, n)
	if n == 0 || count == 0 {
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

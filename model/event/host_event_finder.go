package event

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func getRecentStatusesForHost(hostId string, n int) (int, []string) {
	or := resourceTypeKeyIs(ResourceTypeHost)
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

	if err := db.Aggregate(AllLogCollection, pipeline, &out); err != nil {
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

type RecentHostAgentDeploys struct {
	Last    string `bson:"last" json:"last"`
	Count   int    `bson:"count" json:"count"`
	Failed  int    `bson:"failed" json:"failed"`
	Success int    `bson:"success" json:"success"`
	Total   int    `bson:"total" json:"total"`
}

func GetRecentAgentDeployStatuses(hostId string, n int) (RecentHostAgentDeploys, error) {
	query := resourceTypeKeyIs(ResourceTypeHost)
	query[TypeKey] = bson.M{"$in": []string{EventHostAgentDeployed, EventHostAgentDeployFailed}}
	query[ResourceIdKey] = hostId

	pipeline := []bson.M{
		{"$match": query},
		{"$sort": bson.M{TimestampKey: -1}},
		{"$limit": n},
		{"$group": bson.M{
			"_id":    nil,
			"count":  bson.M{"$sum": 1},
			"states": bson.M{"$push": "$" + TypeKey},
			"last":   bson.M{"$last": "$" + TypeKey},
		}},
		{"$addFields": bson.M{
			"failed": bson.M{"$size": bson.M{
				"$filter": bson.M{
					"input": "$states",
					"cond":  bson.M{"$eq": []string{"$$this", EventHostAgentDeployFailed}},
				},
			}},
			"success": bson.M{"$size": bson.M{
				"$filter": bson.M{
					"input": "$states",
					"cond":  bson.M{"$eq": []string{"$$this", EventHostAgentDeployFailed}},
				},
			}},
		}},
		{"$project": bson.M{
			"_id":     false,
			"count":   true,
			"states":  true,
			"last":    true,
			"success": true,
			"failed":  true,
		}},
	}

	out := RecentHostAgentDeploys{}

	if err := db.Aggregate(AllLogCollection, pipeline, &out); err != nil {
		return RecentHostAgentDeploys{}, errors.Wrap(err, "problem running pipeline")
	}

	out.Total = n

	return out, nil
}

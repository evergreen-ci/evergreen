package event

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// RecentHostAgentDeploys is a type used to capture the results of an agent
// deploy.
type RecentHostAgentDeploys struct {
	HostID            string `bson:"host_id" json:"host_id" yaml:"host_id"`
	Count             int    `bson:"count" json:"count" yaml:"count"`
	Failed            int    `bson:"failed" json:"failed" yaml:"failed"`
	Success           int    `bson:"success" json:"success" yaml:"success"`
	HostStatusChanged int    `bson:"host_status_changed" json:"host_status_changed" yaml:"host_status_changed"`
	Last              string `bson:"last" json:"last" yaml:"last"`
	Total             int    `bson:"total" json:"total" yaml:"total"`
	message.Base      `bson:"metadata" json:"metadata" yaml:"metadata"`
}

// GetRecentAgentDeployStatuses gets status of the the n most recent agent
// deploy attempts for the given hostID.
func GetRecentAgentDeployStatuses(hostID string, n int) (*RecentHostAgentDeploys, error) {
	return getRecentDeployStatuses(hostID, n, EventHostAgentDeployed, EventHostAgentDeployFailed)
}

// GetRecentAgentMonitorDeployStatuses gets status of the the n most recent
// agent monitor deploy attempts for the given hostID.
func GetRecentAgentMonitorDeployStatuses(hostID string, n int) (*RecentHostAgentDeploys, error) {
	return getRecentDeployStatuses(hostID, n, EventHostAgentMonitorDeployed, EventHostAgentMonitorDeployFailed)
}

func getRecentDeployStatuses(hostID string, n int, successStatus, failedStatus string) (*RecentHostAgentDeploys, error) {
	query := ResourceTypeKeyIs(ResourceTypeHost)
	query[TypeKey] = bson.M{"$in": []string{successStatus, failedStatus, EventHostStatusChanged}}
	query[ResourceIdKey] = hostID

	pipeline := []bson.M{
		{"$match": query},
		{"$sort": bson.M{TimestampKey: -1}},
		{"$limit": n},
		{"$group": bson.M{
			"_id":    nil,
			"count":  bson.M{"$sum": 1},
			"states": bson.M{"$push": "$" + TypeKey},
			"last":   bson.M{"$first": "$" + TypeKey},
		}},
		{"$addFields": bson.M{
			"failed": bson.M{"$size": bson.M{
				"$filter": bson.M{
					"input": "$states",
					"cond":  bson.M{"$eq": []string{"$$this", failedStatus}},
				},
			}},
			"success": bson.M{"$size": bson.M{
				"$filter": bson.M{
					"input": "$states",
					"cond":  bson.M{"$eq": []string{"$$this", successStatus}},
				},
			}},
			"host_status_changed": bson.M{"$size": bson.M{
				"$filter": bson.M{
					"input": "$states",
					"cond":  bson.M{"$eq": []string{"$$this", EventHostStatusChanged}},
				},
			}},
		}},
		{"$project": bson.M{
			"_id":                 false,
			"count":               true,
			"success":             true,
			"failed":              true,
			"host_status_changed": true,
			"last":                true,
		}},
	}

	out := []RecentHostAgentDeploys{}

	if err := db.Aggregate(AllLogCollection, pipeline, &out); err != nil {
		return nil, errors.Wrap(err, "problem running pipeline")
	}

	if len(out) != 1 {
		return nil, errors.Errorf("aggregation returned %d results", len(out))
	}

	out[0].Total = n
	out[0].HostID = hostID

	return &out[0], nil
}

////////////////////////////////////////////////////////////////////////
//
// Implementation of methods to satisfy the grip/message.Composer interface

func (m *RecentHostAgentDeploys) Raw() interface{} { return m }
func (m *RecentHostAgentDeploys) Loggable() bool {
	return m.HostID != "" && m.Last != "" && m.Count > 0
}
func (m *RecentHostAgentDeploys) String() string {
	return fmt.Sprintf("host=%s last=%s failed=%d success=%d num=%d",
		m.HostID, m.Last, m.Failed, m.Success, m.Count)
}

////////////////////////////////////////////////////////////////////////
//
// Predicates to support error checking during the agent deploy process
func (m *RecentHostAgentDeploys) LastAttemptFailed() bool { return m.Last == EventHostAgentDeployFailed }
func (m *RecentHostAgentDeploys) AllAttemptsFailed() bool {
	return m.Count > 0 && m.Success == 0 && m.HostStatusChanged == 0
}

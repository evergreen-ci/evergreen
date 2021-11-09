package host

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mongodb/anser/bsonutil"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

type DistroStats []StatsByDistro
type StatsByDistro struct {
	// ID of the distro the below stats are for
	Distro string `bson:"distro" json:"distro,omitempty"`
	// Provider is the provider type of the distro
	Provider string `bson:"provider" json:"provider,omitempty"`
	// Host status that the below stats are for
	Status string `bson:"status" json:"status"`
	// Number of hosts in this status
	Count int `bson:"count" json:"count"`
	// Number of tasks running on hosts in the above group (should only be nonzero for running hosts)
	NumTasks int `bson:"num_tasks_running" json:"num_tasks_running"`
	// MaxHosts reports the pool size of the distro.
	MaxHosts int `bson:"max_hosts" json:"max_hosts"`
}

func (d *StatsByDistro) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(d) }
func (d *StatsByDistro) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, d) }

type ProviderStats []StatsByProvider
type StatsByProvider struct {
	// the name of a host provider
	Provider string `bson:"provider" json:"provider"`
	// Number of hosts with this provider
	Count int `bson:"count" json:"count"`
}

func (p ProviderStats) Map() map[string]int {
	out := map[string]int{}

	for _, s := range p {
		out[s.Provider] = s.Count
	}

	return out
}

func (d DistroStats) CountMap() map[string]int {
	out := map[string]int{}

	for _, s := range d {
		out[s.Distro] += s.Count
	}

	return out
}

func (d DistroStats) TasksMap() map[string]int {
	out := map[string]int{}

	for _, s := range d {
		out[s.Distro] += s.NumTasks
	}

	return out
}

func (d DistroStats) MaxHostsExceeded() map[string]int {
	out := map[string]int{}

	for _, s := range d {
		if s.Provider == evergreen.ProviderNameStatic {
			continue
		}
		overage := -1 * (s.MaxHosts - s.Count)
		if overage > 0 {
			out[s.Distro] = overage
		}
	}

	return out
}

// GetStatsByDistro returns counts of up hosts broken down by distro
func GetStatsByDistro() (DistroStats, error) {
	stats := []StatsByDistro{}
	if err := db.Aggregate(Collection, statsByDistroPipeline(), &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// GetProviderCounts returns data on the number of hosts by different provider stats.
func GetProviderCounts() (ProviderStats, error) {
	stats := []StatsByProvider{}
	if err := db.Aggregate(Collection, statsByProviderPipeline(), &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

////////////////////////////////////////////////////////////////////////
//
// Pipeline impelementations

// statsByDistroPipeline returns a pipeline that will group all up hosts by distro
// and return the count of hosts as well as how many are running tasks
func statsByDistroPipeline() []bson.M {
	return []bson.M{
		{
			"$match": bson.M{
				// Don't count user-spawned hosts (EVG-15232).
				// This also excludes hostcreate tasks (EVG-14363), as their started_by field is the task name.
				StartedByKey: evergreen.User,
				StatusKey: bson.M{
					"$in": evergreen.ActiveStatus,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"distro": "$distro._id",
					"status": "$" + StatusKey,
				},
				"count": bson.M{
					"$sum": 1,
				},
				"max_hosts": bson.M{
					"$max": "$" + DistroKey + "." + bsonutil.GetDottedKeyName(distro.HostAllocatorSettingsKey, distro.HostAllocatorSettingsMaximumHostsKey),
				},
				"tasks": bson.M{
					"$addToSet": "$" + RunningTaskKey,
				},
				"provider": bson.M{
					// Grab any provider, since all hosts in a distro have the same provider
					"$first": "$" + bsonutil.GetDottedKeyName(DistroKey, distro.ProviderKey),
				},
			},
		},
		{
			"$project": bson.M{
				"distro":            "$_id.distro",
				"status":            "$_id.status",
				"max_hosts":         1,
				"count":             1,
				"num_tasks_running": bson.M{"$size": "$tasks"},
				"_id":               0,
				"provider":          1,
			},
		},
	}
}

func statsByProviderPipeline() []bson.M {
	return []bson.M{
		{
			"$match": bson.M{
				StatusKey: bson.M{
					"$in": evergreen.ActiveStatus,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"provider": "$" + bsonutil.GetDottedKeyName(DistroKey, distro.ProviderKey),
				},
				"count": bson.M{
					"$sum": 1,
				},
			},
		},
		{
			"$project": bson.M{
				"provider": "$_id.provider",
				"count":    1,
				"_id":      0,
			},
		},
	}
}

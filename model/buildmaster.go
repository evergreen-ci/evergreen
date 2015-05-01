package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"labix.org/v2/mgo/bson"
)

//GetBuildMaster fetches the most recent "depth" builds and
//returns the task info in their task caches, grouped by
//task display name and buildvariant.
func GetBuildmasterData(v version.Version, depth int) ([]bson.M, error) {
	output := []bson.M{}
	pipeline := []bson.M{
		{"$match": bson.M{
			build.RequesterKey:           evergreen.RepotrackerVersionRequester,
			build.RevisionOrderNumberKey: bson.M{"$lt": v.RevisionOrderNumber},
			build.ProjectKey:             v.Project,
		}},
		{"$sort": bson.M{build.RevisionOrderNumberKey: -1}},
		{"$limit": depth},
		{"$project": bson.M{
			build.TasksKey: 1,
			"v":            "$" + build.BuildVariantKey,
			"s":            1,
			"n":            "$" + build.BuildNumberKey,
		}},
		{"$unwind": "$tasks"},
		{"$project": bson.M{
			"_id": 0,
			"v":   1,
			"d":   "$" + build.TasksKey + "." + build.TaskCacheDisplayNameKey,
			"st":  "$" + build.TasksKey + "." + build.TaskCacheStatusKey,
			"tt":  "$" + build.TasksKey + "." + build.TaskCacheTimeTakenKey,
			"a":   "$" + build.TasksKey + "." + build.TaskCacheActivatedKey,
			"id":  "$" + build.TasksKey + "." + build.TaskCacheIdKey,
		}},
		{"$group": bson.M{
			"_id": bson.M{
				"v": "$v",
				"d": "$d",
			},
			"tests": bson.M{
				"$push": bson.M{
					"s":  "$st",
					"id": "$id",
					"tt": "$tt",
					"a":  "$a",
				},
			},
		}},
	}
	db.Aggregate(build.Collection, pipeline, &output)
	return output, nil
}

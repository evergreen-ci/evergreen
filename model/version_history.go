package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// Maximum number of versions to consider for last_green, more than 100
// revisions back considered "stale"
const (
	StaleVersionCutoff = 100
)

// Given a project name and a list of build variants, return the latest version
// on which all the given build variants succeeded. Gives up after 100 versions.
func FindLastPassingVersionForBuildVariants(project *Project, buildVariantNames []string) (*version.Version, error) {
	if len(buildVariantNames) == 0 {
		return nil, errors.New("No build variants specified!")
	}

	// Get latest commit order number for this project
	latestVersion, err := version.FindOne(version.ByMostRecentSystemRequester(project.Identifier).WithFields(version.RevisionOrderNumberKey))
	if err != nil {
		return nil, errors.Wrap(err, "Error getting latest version")
	}
	if latestVersion == nil {
		return nil, nil
	}

	mostRecentRevisionOrderNumber := latestVersion.RevisionOrderNumber

	// Earliest commit order number to consider
	leastRecentRevisionOrderNumber := mostRecentRevisionOrderNumber - StaleVersionCutoff
	if leastRecentRevisionOrderNumber < 0 {
		leastRecentRevisionOrderNumber = 0
	}

	pipeline := []bson.M{
		// Limit ourselves to builds for non-stale versions and the given project
		// and build variants
		{
			"$match": bson.M{
				build.ProjectKey:             project.Identifier,
				build.RevisionOrderNumberKey: bson.M{"$gte": leastRecentRevisionOrderNumber},
				build.BuildVariantKey:        bson.M{"$in": buildVariantNames},
				build.StatusKey:              evergreen.BuildSucceeded,
				build.RequesterKey: bson.M{
					"$in": evergreen.SystemVersionRequesterTypes,
				},
			},
		},
		// Sum up the number of builds that succeeded for each commit order number
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%v", build.RevisionOrderNumberKey),
				"numSucceeded": bson.M{
					"$sum": 1,
				},
			},
		},
		// Find builds that succeeded on all of the requested build variants
		{
			"$match": bson.M{"numSucceeded": len(buildVariantNames)},
		},
		// Order by commit order number, descending
		{
			"$sort": bson.M{"_id": -1},
		},
		// Get the highest commit order number where builds succeeded on all the
		// requested build variants
		{
			"$limit": 1,
		},
	}

	var result []bson.M

	err = db.Aggregate(build.Collection, pipeline, &result)
	if err != nil {
		return nil, errors.Wrap(err, "Aggregation failed")
	}

	if len(result) == 0 {
		return nil, nil
	}

	// Get the version corresponding to the resulting commit order number
	v, err := version.FindOne(
		db.Query(bson.M{
			version.RequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			version.IdentifierKey:          project.Identifier,
			version.RevisionOrderNumberKey: result[0]["_id"],
		}))
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errors.Errorf("Couldn't find version with id `%v` after "+
			"successful aggregation.", result[0]["_id"])
	}
	return v, nil
}

package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"fmt"
	"labix.org/v2/mgo/bson"
)

// Maximum number of versions to consider for last_green, more than 100
// revisions back considered "stale"
const (
	StaleVersionCutoff = 100
)

// Given a project name and a list of build variants, return the latest version
// on which all the given build variants succeeded. Gives up after 100 versions.
func FindLastPassingVersionForBuildVariants(project Project, buildVariantNames []string) (*Version, error) {
	if len(buildVariantNames) == 0 {
		return nil, fmt.Errorf("No build variants specified!")
	}

	// Get latest commit order number for this project
	latestVersions, err := FindAllVersions(
		bson.M{
			VersionRequesterKey: mci.RepotrackerVersionRequester,
			VersionProjectKey:   project.Identifier,
		},
		bson.M{
			VersionIdKey:                  0,
			VersionRevisionOrderNumberKey: 1,
		},
		[]string{"-order"},
		0,
		1)
	if err != nil {
		return nil, fmt.Errorf("Error getting latest version: %v", err)
	}
	if len(latestVersions) == 0 {
		return nil, nil
	}

	mostRecentRevisionOrderNumber := latestVersions[0].RevisionOrderNumber

	// Earliest commit order number to consider
	leastRecentRevisionOrderNumber := mostRecentRevisionOrderNumber -
		StaleVersionCutoff
	if leastRecentRevisionOrderNumber < 0 {
		leastRecentRevisionOrderNumber = 0
	}

	pipeline := []bson.M{
		// Limit ourselves to builds for non-stale versions and the given project
		// and build variants
		{
			"$match": bson.M{
				BuildProjectKey:             project.Identifier,
				BuildRevisionOrderNumberKey: bson.M{"$gte": leastRecentRevisionOrderNumber},
				BuildBuildVariantKey:        bson.M{"$in": buildVariantNames},
				BuildStatusKey:              mci.BuildSucceeded,
			},
		},
		// Sum up the number of builds that succeeded for each commit order number
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%v", BuildRevisionOrderNumberKey),
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
	err = db.Aggregate(BuildsCollection, pipeline, &result)

	if err != nil {
		return nil, fmt.Errorf("Aggregation failed: %v", err)
	}

	if len(result) == 0 {
		return nil, nil
	}

	// Get the version corresponding to the resulting commit order number
	version, err := FindOneVersion(
		bson.M{
			VersionRequesterKey:           mci.RepotrackerVersionRequester,
			VersionProjectKey:             project.Identifier,
			VersionRevisionOrderNumberKey: result[0]["_id"],
		},
		db.NoProjection)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("Couldn't find version with id `%v` after "+
			"successful aggregation.", result[0]["_id"])
	}
	return version, nil
}

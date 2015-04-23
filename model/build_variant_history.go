package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"labix.org/v2/mgo/bson"
)

type buildVariantHistoryIterator struct {
	BuildVariantInTask    string
	BuildVariantInVersion string
	ProjectName           string
}

// Interface around getting task and version history for a given build variant
// in a given project.
type BuildVariantHistoryIterator interface {
	GetItems(beforeCommit *Version, numCommits int) ([]bson.M, []Version, error)
}

// Since version currently uses build variant display name and task uses build variant
// name, we need both.
func NewBuildVariantHistoryIterator(buildVariantInTask string, buildVariantInVersion string,
	projectName string) BuildVariantHistoryIterator {
	return &buildVariantHistoryIterator{buildVariantInTask, buildVariantInVersion, projectName}
}

// Returns versions and tasks grouped by gitspec, newest first (sorted by order number desc)
func (self *buildVariantHistoryIterator) GetItems(beforeCommit *Version, numRevisions int) ([]bson.M, []Version, error) {
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	defer session.Close()
	if err != nil {
		return nil, nil, err
	}

	var versionFilter bson.M
	if beforeCommit != nil {
		versionFilter = bson.M{
			VersionRequesterKey:           mci.RepotrackerVersionRequester,
			VersionRevisionOrderNumberKey: bson.M{"$lt": beforeCommit.RevisionOrderNumber},
			VersionProjectKey:             self.ProjectName,
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					BuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		}
	} else {
		versionFilter = bson.M{
			VersionRequesterKey: mci.RepotrackerVersionRequester,
			VersionProjectKey:   self.ProjectName,
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					BuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		}
	}

	//Get the next numCommits
	versions, err := FindAllVersions(
		versionFilter,
		bson.M{
			VersionIdKey:                  1,
			VersionRevisionOrderNumberKey: 1,
			VersionRevisionKey:            1,
			VersionMessageKey:             1,
			VersionCreateTimeKey:          1,
		},
		[]string{"-" + VersionRevisionOrderNumberKey},
		0,
		numRevisions)

	if err != nil {
		return nil, nil, err
	}

	if len(versions) == 0 {
		return nil, []Version{}, nil
	}

	//versionEndBoundary is the *earliest* version which should be included in results
	versionEndBoundary := versions[len(versions)-1]

	matchFilter := bson.M{
		TaskRequesterKey:    mci.RepotrackerVersionRequester,
		TaskBuildVariantKey: self.BuildVariantInTask,
		TaskProjectKey:      self.ProjectName,
	}

	if beforeCommit != nil {
		matchFilter[TaskRevisionOrderNumberKey] = bson.M{
			"$gte": versionEndBoundary.RevisionOrderNumber,
			"$lt":  beforeCommit.RevisionOrderNumber,
		}
	} else {
		matchFilter[TaskRevisionOrderNumberKey] = bson.M{
			"$gte": versionEndBoundary.RevisionOrderNumber,
		}
	}

	pipeline := db.C(TasksCollection).Pipe(
		[]bson.M{
			{"$match": matchFilter},
			bson.M{"$sort": bson.D{{TaskRevisionOrderNumberKey, 1}}},
			bson.M{
				"$group": bson.M{
					"_id":   "$" + TaskRevisionKey,
					"order": bson.M{"$first": "$" + TaskRevisionOrderNumberKey},
					"tasks": bson.M{
						"$push": bson.M{
							"_id":          "$" + TaskIdKey,
							"status":       "$" + TaskStatusKey,
							"activated":    "$" + TaskActivatedKey,
							"time_taken":   "$" + TaskTimeTakenKey,
							"display_name": "$" + TaskDisplayNameKey,
						},
					},
				},
			},
			bson.M{"$sort": bson.M{TaskRevisionOrderNumberKey: -1, TaskDisplayNameKey: 1}},
		},
	)

	var output []bson.M
	err = pipeline.All(&output)
	if err != nil {
		return nil, nil, err
	}

	return output, versions, nil
}

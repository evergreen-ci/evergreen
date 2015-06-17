package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/version"
	"gopkg.in/mgo.v2/bson"
)

type buildVariantHistoryIterator struct {
	BuildVariantInTask    string
	BuildVariantInVersion string
	ProjectName           string
}

// Interface around getting task and version history for a given build variant
// in a given project.
type BuildVariantHistoryIterator interface {
	GetItems(beforeCommit *version.Version, numCommits int) ([]bson.M, []version.Version, error)
}

// Since version currently uses build variant display name and task uses build variant
// name, we need both.
func NewBuildVariantHistoryIterator(buildVariantInTask string, buildVariantInVersion string,
	projectName string) BuildVariantHistoryIterator {
	return &buildVariantHistoryIterator{buildVariantInTask, buildVariantInVersion, projectName}
}

// Returns versions and tasks grouped by gitspec, newest first (sorted by order number desc)
func (self *buildVariantHistoryIterator) GetItems(beforeCommit *version.Version, numRevisions int) ([]bson.M, []version.Version, error) {
	session, dbobj, err := db.GetGlobalSessionFactory().GetSession()
	defer session.Close()
	if err != nil {
		return nil, nil, err
	}

	var versionQuery db.Q
	if beforeCommit != nil {
		versionQuery = db.Query(bson.M{
			version.RequesterKey:           evergreen.RepotrackerVersionRequester,
			version.RevisionOrderNumberKey: bson.M{"$lt": beforeCommit.RevisionOrderNumber},
			version.ProjectKey:             self.ProjectName,
			version.BuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					version.BuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		})
	} else {
		versionQuery = db.Query(bson.M{
			version.RequesterKey: evergreen.RepotrackerVersionRequester,
			version.ProjectKey:   self.ProjectName,
			version.BuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					version.BuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		})
	}
	versionQuery = versionQuery.WithFields(
		version.IdKey,
		version.RevisionOrderNumberKey,
		version.RevisionKey,
		version.MessageKey,
		version.CreateTimeKey,
	).Sort([]string{"-" + version.RevisionOrderNumberKey}).Limit(numRevisions)

	//Get the next numCommits
	versions, err := version.Find(versionQuery)

	if err != nil {
		return nil, nil, err
	}

	if len(versions) == 0 {
		return nil, []version.Version{}, nil
	}

	//versionEndBoundary is the *earliest* version which should be included in results
	versionEndBoundary := versions[len(versions)-1]

	matchFilter := bson.M{
		TaskRequesterKey:    evergreen.RepotrackerVersionRequester,
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

	pipeline := dbobj.C(TasksCollection).Pipe(
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

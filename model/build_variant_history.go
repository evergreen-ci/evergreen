package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"go.mongodb.org/mongo-driver/bson"
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
	var versionQuery db.Q
	if beforeCommit != nil {
		versionQuery = db.Query(bson.M{
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionRevisionOrderNumberKey: bson.M{"$lt": beforeCommit.RevisionOrderNumber},
			VersionIdentifierKey:          self.ProjectName,
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					VersionBuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		})
	} else {
		versionQuery = db.Query(bson.M{
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionIdentifierKey: self.ProjectName,
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					VersionBuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		})
	}
	versionQuery = versionQuery.WithFields(
		VersionIdKey,
		VersionRevisionOrderNumberKey,
		VersionRevisionKey,
		VersionMessageKey,
		VersionCreateTimeKey,
	).Sort([]string{"-" + VersionRevisionOrderNumberKey}).Limit(numRevisions)

	//Get the next numCommits
	versions, err := VersionFind(versionQuery)

	if err != nil {
		return nil, nil, err
	}

	if len(versions) == 0 {
		return nil, []Version{}, nil
	}

	//versionEndBoundary is the *earliest* version which should be included in results
	versionEndBoundary := versions[len(versions)-1]

	matchFilter := bson.M{
		task.RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		task.BuildVariantKey: self.BuildVariantInTask,
		task.ProjectKey:      self.ProjectName,
	}

	if beforeCommit != nil {
		matchFilter[task.RevisionOrderNumberKey] = bson.M{
			"$gte": versionEndBoundary.RevisionOrderNumber,
			"$lt":  beforeCommit.RevisionOrderNumber,
		}
	} else {
		matchFilter[task.RevisionOrderNumberKey] = bson.M{
			"$gte": versionEndBoundary.RevisionOrderNumber,
		}
	}

	pipeline := []bson.M{
		{"$match": matchFilter},
		{"$sort": bson.D{{Key: task.RevisionOrderNumberKey, Value: 1}}},
		{
			"$group": bson.M{
				"_id":   "$" + task.RevisionKey,
				"order": bson.M{"$first": "$" + task.RevisionOrderNumberKey},
				"tasks": bson.M{
					"$push": bson.M{
						"_id":              "$" + task.IdKey,
						"status":           "$" + task.StatusKey,
						"task_end_details": "$" + task.DetailsKey,
						"activated":        "$" + task.ActivatedKey,
						"time_taken":       "$" + task.TimeTakenKey,
						"display_name":     "$" + task.DisplayNameKey,
					},
				},
			},
		},
		{"$sort": bson.M{task.RevisionOrderNumberKey: -1, task.DisplayNameKey: 1}},
	}

	var output []bson.M
	if err = db.Aggregate(task.Collection, pipeline, &output); err != nil {
		return nil, nil, err
	}

	return output, versions, nil
}

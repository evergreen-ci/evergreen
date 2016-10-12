package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
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
	var versionQuery db.Q
	if beforeCommit != nil {
		versionQuery = db.Query(bson.M{
			version.RequesterKey:           evergreen.RepotrackerVersionRequester,
			version.RevisionOrderNumberKey: bson.M{"$lt": beforeCommit.RevisionOrderNumber},
			version.IdentifierKey:          self.ProjectName,
			version.BuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					version.BuildStatusVariantKey: self.BuildVariantInVersion,
				},
			},
		})
	} else {
		versionQuery = db.Query(bson.M{
			version.RequesterKey:  evergreen.RepotrackerVersionRequester,
			version.IdentifierKey: self.ProjectName,
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
		task.RequesterKey:    evergreen.RepotrackerVersionRequester,
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
		{"$sort": bson.D{{task.RevisionOrderNumberKey, 1}}},
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

package version

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type Version struct {
	Id                  string        `bson:"_id" json:"id,omitempty"`
	CreateTime          time.Time     `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time     `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time     `bson:"finish_time" json:"finish_time,omitempty"`
	Revision            string        `bson:"gitspec" json:"revision,omitempty"`
	Author              string        `bson:"author" json:"author,omitempty"`
	AuthorEmail         string        `bson:"author_email" json:"author_email,omitempty"`
	Message             string        `bson:"message" json:"message,omitempty"`
	Status              string        `bson:"status" json:"status,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty" json:"order,omitempty"`
	Config              string        `bson:"config" json:"config,omitempty"`
	Ignored             bool          `bson:"ignored" json:"ignored"`
	Owner               string        `bson:"owner_name" json:"owner_name,omitempty"`
	Repo                string        `bson:"repo_name" json:"repo_name,omitempty"`
	Branch              string        `bson:"branch_name" json:"branch_name,omitempty"`
	RepoKind            string        `bson:"repo_kind" json:"repo_kind,omitempty"`
	BuildVariants       []BuildStatus `bson:"build_variants_status,omitempty" json:"build_variants_status,omitempty"`

	// This is technically redundant, but a lot of code relies on it, so I'm going to leave it
	BuildIds []string `bson:"builds" json:"builds,omitempty"`

	Identifier string `bson:"identifier" json:"identifier,omitempty"`
	Remote     bool   `bson:"remote" json:"remote,omitempty"`
	RemotePath string `bson:"remote_path" json:"remote_path,omitempty"`
	// version requester - this is used to help tell the
	// reason this version was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"requester,omitempty"`
	// version errors - this is used to keep track of any errors that were
	// encountered in the process of creating a version. If there are no errors
	// this field is omitted in the database
	Errors   []string `bson:"errors,omitempty" json:"errors,omitempty"`
	Warnings []string `bson:"warnings,omitempty" json:"warnings,omitempty"`

	// AuthorID is an optional reference to the Evergreen user that authored
	// this comment, if they can be identified
	AuthorID string `bson:"author_id,omitempty" json:"author_id,omitempty"`

	// Fields set if triggered by an upstream build
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`
}

func (v *Version) LastSuccessful() (*Version, error) {
	lastGreen, err := FindOne(BySuccessfulBeforeRevision(v.Identifier, v.RevisionOrderNumber).Sort(
		[]string{"-" + RevisionOrderNumberKey}))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving last successful version")
	}
	return lastGreen, nil
}

func (self *Version) UpdateBuildVariants() error {
	return UpdateOne(
		bson.M{IdKey: self.Id},
		bson.M{
			"$set": bson.M{
				BuildVariantsKey: self.BuildVariants,
			},
		},
	)
}

func (self *Version) Insert() error {
	return db.Insert(Collection, self)
}

// BuildStatus stores metadata relating to each build
type BuildStatus struct {
	BuildVariant string    `bson:"build_variant" json:"id"`
	Activated    bool      `bson:"activated" json:"activated"`
	ActivateAt   time.Time `bson:"activate_at,omitempty" json:"activate_at,omitempty"`
	BuildId      string    `bson:"build_id,omitempty" json:"build_id,omitempty"`
}

var (
	BuildStatusVariantKey    = bsonutil.MustHaveTag(BuildStatus{}, "BuildVariant")
	BuildStatusActivatedKey  = bsonutil.MustHaveTag(BuildStatus{}, "Activated")
	BuildStatusActivateAtKey = bsonutil.MustHaveTag(BuildStatus{}, "ActivateAt")
	BuildStatusBuildIdKey    = bsonutil.MustHaveTag(BuildStatus{}, "BuildId")
)

type DuplicateVersionsID struct {
	Hash      string `bson:"hash"`
	ProjectID string `bson:"project_id"`
}

type DuplicateVersions struct {
	ID       DuplicateVersionsID `bson:"_id"`
	Versions []Version           `bson:"versions"`
}

func FindDuplicateVersions(since time.Time) ([]DuplicateVersions, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				RequesterKey: bson.M{
					"$in": evergreen.SystemVersionRequesterTypes,
				},
				CreateTimeKey: bson.M{
					"$gte": since,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"project_id": "$" + IdentifierKey,
					"hash":       "$" + RevisionKey,
				},
				"count": bson.M{
					"$sum": 1,
				},
				"versions": bson.M{
					"$push": "$$ROOT",
				},
			},
		},
		{
			"$match": bson.M{
				"count": bson.M{
					"$gt": 1,
				},
			},
		},
	}

	out := []DuplicateVersions{}
	if err := db.Aggregate(Collection, pipeline, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func GetHistory(versionId string, N int) ([]Version, error) {
	v, err := FindOne(ById(versionId))
	if err != nil {
		return nil, errors.WithStack(err)
	} else if v == nil {
		return nil, errors.Errorf("Version '%v' not found", versionId)
	}

	// Versions in the same push event, assuming that no two push events happen at the exact same time
	// Never want more than 2N+1 versions, so make sure we add a limit

	siblingVersions, err := Find(db.Query(
		bson.M{
			RevisionOrderNumberKey: v.RevisionOrderNumber,
			RequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			IdentifierKey: v.Identifier,
		}).WithoutFields(ConfigKey).Sort([]string{RevisionOrderNumberKey}).Limit(2*N + 1))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	versionIndex := -1
	for i := 0; i < len(siblingVersions); i++ {
		if siblingVersions[i].Id == v.Id {
			versionIndex = i
		}
	}

	numSiblings := len(siblingVersions) - 1
	versions := siblingVersions

	if versionIndex < N {
		// There are less than N later versions from the same push event
		// N subsequent versions plus the specified one
		subsequentVersions, err := Find(
			//TODO encapsulate this query in version pkg
			db.Query(bson.M{
				RevisionOrderNumberKey: bson.M{"$gt": v.RevisionOrderNumber},
				RequesterKey: bson.M{
					"$in": evergreen.SystemVersionRequesterTypes,
				},
				IdentifierKey: v.Identifier,
			}).WithoutFields(ConfigKey).Sort([]string{RevisionOrderNumberKey}).Limit(N - versionIndex))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Reverse the second array so we have the versions ordered "newest one first"
		for i := 0; i < len(subsequentVersions)/2; i++ {
			subsequentVersions[i], subsequentVersions[len(subsequentVersions)-1-i] = subsequentVersions[len(subsequentVersions)-1-i], subsequentVersions[i]
		}

		versions = append(subsequentVersions, versions...)
	}

	if numSiblings-versionIndex < N {
		previousVersions, err := Find(db.Query(bson.M{
			RevisionOrderNumberKey: bson.M{"$lt": v.RevisionOrderNumber},
			RequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			IdentifierKey: v.Identifier,
		}).WithoutFields(ConfigKey).Sort([]string{fmt.Sprintf("-%v", RevisionOrderNumberKey)}).Limit(N))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		versions = append(versions, previousVersions...)
	}

	return versions, nil
}

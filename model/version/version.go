package version

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	//"gopkg.in/yaml.v1"
	"labix.org/v2/mgo/bson"
	"time"
)

type Version struct {
	Id                  string        `bson:"_id" json:"id,omitempty"`
	CreateTime          time.Time     `bson:"create_time" json:"create_time,omitempty"`
	StartTime           time.Time     `bson:"start_time" json:"start_time,omitempty"`
	FinishTime          time.Time     `bson:"finish_time" json:"finish_time,omitempty"`
	Project             string        `bson:"branch" json:"branch,omitempty"`
	Revision            string        `bson:"gitspec" json:"revision,omitempty"`
	Author              string        `bson:"author" json:"author,omitempty"`
	AuthorEmail         string        `bson:"author_email" json:"author_email,omitempty"`
	Message             string        `bson:"message" json:"message,omitempty"`
	Status              string        `bson:"status" json:"status,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty" json:"order,omitempty"`
	Config              string        `bson:"config" json:"config,omitempty"`
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
	Errors []string `bson:"errors,omitempty" json:"errors,omitempty"`
}

// TODO get rid of this.
func TotalVersions(query interface{}) (int, error) {
	return db.Count(Collection, query)
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

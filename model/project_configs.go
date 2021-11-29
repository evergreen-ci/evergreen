package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
)

type ProjectConfig struct {
	Id          string    `yaml:"_id" bson:"_id"`
	Enabled     *bool     `yaml:"enabled,omitempty" bson:"enabled,omitempty"`
	Owner       *string   `yaml:"owner,omitempty" bson:"owner,omitempty"`
	Repo        *string   `yaml:"repo,omitempty" bson:"repo,omitempty"`
	RemotePath  *string   `yaml:"remote_path,omitempty" bson:"remote_path,omitempty"`
	Branch      *string   `yaml:"branch,omitempty" bson:"branch,omitempty"`
	Identifier  *string   `yaml:"identifier,omitempty" bson:"identifier,omitempty"`
	DisplayName *string   `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
	CreateTime  time.Time `yaml:"create_time,omitempty" bson:"create_time,omitempty"`

	// These fields can be set for the ProjectRef struct on the project page, or in the project config yaml.
	// Values for the below fields set on the project page will take precedence over this struct and will
	// be the configs used for a given project during runtime.
	TaskAnnotationSettings *evergreen.AnnotationsSettings `yaml:"task_annotation_settings,omitempty" bson:"task_annotation_settings,omitempty"`
	BuildBaronSettings     *evergreen.BuildBaronSettings  `yaml:"build_baron_settings,omitempty" bson:"build_baron_settings,omitempty"`
	PerfEnabled            *bool                          `yaml:"perf_enabled,omitempty" bson:"perf_enabled,omitempty"`
	CommitQueueAliases     []ProjectAlias                 `yaml:"commit_queue_aliases,omitempty" bson:"commit_queue_aliases,omitempty"`
	GitHubPRAliases        []ProjectAlias                 `yaml:"github_pr_aliases,omitempty" bson:"github_pr_aliases,omitempty"`
	GitTagAliases          []ProjectAlias                 `yaml:"git_tag_aliases,omitempty" bson:"git_tag_aliases,omitempty"`
	GitHubChecksAliases    []ProjectAlias                 `yaml:"github_checks_aliases,omitempty" bson:"github_checks_aliases,omitempty"`
	PatchAliases           []ProjectAlias                 `yaml:"patch_aliases,omitempty" bson:"patch_aliases,omitempty"`
	DeactivatePrevious     *bool                          `yaml:"deactivate_previous" bson:"deactivate_previous,omitempty"`
	WorkstationConfig      *WorkstationConfig             `yaml:"workstation_config,omitempty" bson:"workstation_config,omitempty"`
	CommitQueue            *CommitQueueParams             `yaml:"commit_queue,omitempty" bson:"commit_queue,omitempty"`
	TaskSync               *TaskSyncOptions               `yaml:"task_sync,omitempty" bson:"task_sync,omitempty"`
} // End of ProjectConfig struct
// Comment above is used by the linter to detect the end of the struct.

func (pc *ProjectConfig) Insert() error {
	return db.Insert(ProjectConfigsCollection, pc)
}

func (pc *ProjectConfig) MarshalBSON() ([]byte, error) {
	return mgobson.Marshal(pc)
}

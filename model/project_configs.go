package model

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

type ProjectConfig struct {
	Id         string    `yaml:"_id" bson:"_id"`
	Identifier string    `yaml:"identifier,omitempty" bson:"identifier,omitempty"`
	CreateTime time.Time `yaml:"create_time,omitempty" bson:"create_time,omitempty"`

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
}

// Comment above is used by the linter to detect the end of the struct.

func (pc *ProjectConfig) Insert() error {
	return db.Insert(ProjectConfigCollection, pc)
}

func (pc *ProjectConfig) MarshalBSON() ([]byte, error) {
	return mgobson.Marshal(pc)
}

func (pc *ProjectConfig) isEmpty() bool {
	reflectedConfig := reflect.ValueOf(pc).Elem()
	types := reflect.TypeOf(pc).Elem()

	for i := 0; i < reflectedConfig.NumField(); i++ {
		field := reflectedConfig.Field(i)
		name := types.Field(i).Name
		if name != "Id" && name != "Identifier" {
			if !util.IsFieldUndefined(field) {
				return false
			}
		}
	}
	return true
}

// createProjectConfig marshals the supplied YAML into our
// intermediate configs representation.
func createProjectConfig(yml []byte) (*ProjectConfig, error) {
	p := &ProjectConfig{}
	if err := util.UnmarshalYAMLWithFallback(yml, p); err != nil {
		yamlErr := thirdparty.YAMLFormatError{Message: err.Error()}
		return nil, errors.Wrap(yamlErr, "error unmarshalling into project config")
	}
	if p.isEmpty() {
		return nil, nil
	}
	p.CreateTime = time.Now()
	return p, nil
}

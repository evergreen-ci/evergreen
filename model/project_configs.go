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
	Id string `yaml:"_id" bson:"_id"`
	ProjectConfigFields
}

type ProjectConfigFields struct {
	Project                string                         `yaml:"project,omitempty" bson:"project,omitempty"`
	ConfigCreateTime       time.Time                      `yaml:"config_create_time,omitempty" bson:"config_create_time,omitempty"`
	TaskAnnotationSettings *evergreen.AnnotationsSettings `yaml:"task_annotation_settings,omitempty" bson:"task_annotation_settings,omitempty"`
	BuildBaronSettings     *evergreen.BuildBaronSettings  `yaml:"build_baron_settings,omitempty" bson:"build_baron_settings,omitempty"`
	CommitQueueAliases     []ProjectAlias                 `yaml:"commit_queue_aliases,omitempty" bson:"commit_queue_aliases,omitempty"`
	GitHubPRAliases        []ProjectAlias                 `yaml:"github_pr_aliases,omitempty" bson:"github_pr_aliases,omitempty"`
	GitTagAliases          []ProjectAlias                 `yaml:"git_tag_aliases,omitempty" bson:"git_tag_aliases,omitempty"`
	GitHubChecksAliases    []ProjectAlias                 `yaml:"github_checks_aliases,omitempty" bson:"github_checks_aliases,omitempty"`
	PatchAliases           []ProjectAlias                 `yaml:"patch_aliases,omitempty" bson:"patch_aliases,omitempty"`
	WorkstationConfig      *WorkstationConfig             `yaml:"workstation_config,omitempty" bson:"workstation_config,omitempty"`
	TaskSync               *TaskSyncOptions               `yaml:"task_sync,omitempty" bson:"task_sync,omitempty"`
	GithubTriggerAliases   []string                       `yaml:"github_trigger_aliases,omitempty" bson:"github_trigger_aliases,omitempty"`
	PeriodicBuilds         []PeriodicBuildDefinition      `yaml:"periodic_builds,omitempty" bson:"periodic_builds,omitempty"`
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

// CreateProjectConfig marshals the supplied YAML into our
// intermediate configs representation.
func CreateProjectConfig(yml []byte) (*ProjectConfig, error) {
	p := &ProjectConfig{}
	if err := util.UnmarshalYAMLWithFallback(yml, p); err != nil {
		yamlErr := thirdparty.YAMLFormatError{Message: err.Error()}
		return nil, errors.Wrap(yamlErr, "error unmarshalling into project config")
	}
	if p.isEmpty() {
		return nil, nil
	}
	p.ConfigCreateTime = time.Now()
	return p, nil
}

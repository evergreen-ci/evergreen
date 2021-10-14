package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const LoadProjectError = "load project error(s)"
const TranslateProjectError = "error translating project"

// This file contains the infrastructure for turning a YAML project configuration
// into a usable Project struct. A basic overview of the project parsing process is:
//
// First, the YAML bytes are unmarshalled into an intermediary ParserProject.
// The ParserProject's internal types define custom YAML unmarshal hooks, allowing
// users to do things like offer a single definition where we expect a list, e.g.
//   `tags: "single_tag"` instead of the more verbose `tags: ["single_tag"]`
// or refer to task by a single selector. Custom YAML handling allows us to
// add other useful features like detecting fatal errors and reporting them
// through the YAML parser's error code, which supplies helpful line number information
// that we would lose during validation against already-parsed data. In the future,
// custom YAML hooks will allow us to add even more helpful features, like alerting users
// when they use fields that aren't actually defined.
//
// Once the intermediary project is created, we crawl it to evaluate tag selectors
// and matrix definitions. This step recursively crawls variants, tasks, their
// dependencies, and so on, to replace selectors with the tasks they reference and return
// a populated Project type.
//
// Code outside of this file should never have to consider selectors or parser* types
// when handling project code.

// ParserProject serves as an intermediary struct for parsing project
// configuration YAML. It implements the Unmarshaler interface
// to allow for flexible handling.
type ParserProject struct {
	// Id and ConfigdUpdateNumber are not pointers because they are only used internally
	Id                     string                         `yaml:"_id" bson:"_id"` // should be the same as the version's ID
	ConfigUpdateNumber     int                            `yaml:"config_number,omitempty" bson:"config_number,omitempty"`
	Enabled                *bool                          `yaml:"enabled,omitempty" bson:"enabled,omitempty"`
	Stepback               *bool                          `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	PreErrorFailsTask      *bool                          `yaml:"pre_error_fails_task,omitempty" bson:"pre_error_fails_task,omitempty"`
	PostErrorFailsTask     *bool                          `yaml:"post_error_fails_task,omitempty" bson:"post_error_fails_task,omitempty"`
	OomTracker             *bool                          `yaml:"oom_tracker,omitempty" bson:"oom_tracker,omitempty"`
	BatchTime              *int                           `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	Owner                  *string                        `yaml:"owner,omitempty" bson:"owner,omitempty"`
	Repo                   *string                        `yaml:"repo,omitempty" bson:"repo,omitempty"`
	RemotePath             *string                        `yaml:"remote_path,omitempty" bson:"remote_path,omitempty"`
	Branch                 *string                        `yaml:"branch,omitempty" bson:"branch,omitempty"`
	Identifier             *string                        `yaml:"identifier,omitempty" bson:"identifier,omitempty"`
	DisplayName            *string                        `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
	CommandType            *string                        `yaml:"command_type,omitempty" bson:"command_type,omitempty"`
	Ignore                 parserStringSlice              `yaml:"ignore,omitempty" bson:"ignore,omitempty"`
	Parameters             []ParameterInfo                `yaml:"parameters,omitempty" bson:"parameters,omitempty"`
	Pre                    *YAMLCommandSet                `yaml:"pre,omitempty" bson:"pre,omitempty"`
	Post                   *YAMLCommandSet                `yaml:"post,omitempty" bson:"post,omitempty"`
	Timeout                *YAMLCommandSet                `yaml:"timeout,omitempty" bson:"timeout,omitempty"`
	EarlyTermination       *YAMLCommandSet                `yaml:"early_termination,omitempty" bson:"early_termination,omitempty"`
	CallbackTimeout        *int                           `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs,omitempty"`
	Modules                []Module                       `yaml:"modules,omitempty" bson:"modules,omitempty"`
	BuildVariants          []parserBV                     `yaml:"buildvariants,omitempty" bson:"buildvariants,omitempty"`
	Functions              map[string]*YAMLCommandSet     `yaml:"functions,omitempty" bson:"functions,omitempty"`
	TaskGroups             []parserTaskGroup              `yaml:"task_groups,omitempty" bson:"task_groups,omitempty"`
	Tasks                  []parserTask                   `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	ExecTimeoutSecs        *int                           `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	Loggers                *LoggerConfig                  `yaml:"loggers,omitempty" bson:"loggers,omitempty"`
	CreateTime             time.Time                      `yaml:"create_time,omitempty" bson:"create_time,omitempty"`
	TaskAnnotationSettings *evergreen.AnnotationsSettings `yaml:"task_annotation_settings,omitempty" bson:"task_annotation_settings,omitempty"`
	BuildBaronSettings     *evergreen.BuildBaronSettings  `yaml:"build_baron_settings,omitempty" bson:"build_baron_settings,omitempty"`
	PerfEnabled            *bool                          `yaml:"perf_enabled,omitempty" bson:"perf_enabled,omitempty"`

	// The below fields can be set for the ProjectRef struct on the project page, or in the project config yaml.
	// Values for the below fields set on this struct will take precedence over the project page and will
	// be the configs used for a given project during runtime.
	DeactivatePrevious *bool `yaml:"deactivate_previous" bson:"deactivate_previous,omitempty"`

	// List of yamls to merge
	Include []Include `yaml:"include,omitempty" bson:"include,omitempty"`

	// Matrix code
	Axes []matrixAxis `yaml:"axes,omitempty" bson:"axes,omitempty"`
} // End of ParserProject struct
// Comment above is used by the linter to detect the end of the struct.

type parserTaskGroup struct {
	Name                    string             `yaml:"name,omitempty" bson:"name,omitempty"`
	Priority                int64              `yaml:"priority,omitempty" bson:"priority,omitempty"`
	Patchable               *bool              `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly               *bool              `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	AllowForGitTag          *bool              `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly              *bool              `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	ExecTimeoutSecs         int                `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	Stepback                *bool              `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	MaxHosts                int                `yaml:"max_hosts,omitempty" bson:"max_hosts,omitempty"`
	SetupGroupFailTask      bool               `yaml:"setup_group_can_fail_task,omitempty" bson:"setup_group_can_fail_task,omitempty"`
	TeardownTaskCanFailTask bool               `yaml:"teardown_task_can_fail_task,omitempty" bson:"teardown_task_can_fail_task,omitempty"`
	SetupGroupTimeoutSecs   int                `yaml:"setup_group_timeout_secs,omitempty" bson:"setup_group_timeout_secs,omitempty"`
	SetupGroup              *YAMLCommandSet    `yaml:"setup_group,omitempty" bson:"setup_group,omitempty"`
	TeardownGroup           *YAMLCommandSet    `yaml:"teardown_group,omitempty" bson:"teardown_group,omitempty"`
	SetupTask               *YAMLCommandSet    `yaml:"setup_task,omitempty" bson:"setup_task,omitempty"`
	TeardownTask            *YAMLCommandSet    `yaml:"teardown_task,omitempty" bson:"teardown_task,omitempty"`
	Timeout                 *YAMLCommandSet    `yaml:"timeout,omitempty" bson:"timeout,omitempty"`
	Tasks                   []string           `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	DependsOn               parserDependencies `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	Tags                    parserStringSlice  `yaml:"tags,omitempty" bson:"tags,omitempty"`
	ShareProcs              bool               `yaml:"share_processes,omitempty" bson:"share_processes,omitempty"`
}

func (ptg *parserTaskGroup) name() string   { return ptg.Name }
func (ptg *parserTaskGroup) tags() []string { return ptg.Tags }

// parserTask represents an intermediary state of task definitions.
type parserTask struct {
	Name            string              `yaml:"name,omitempty" bson:"name,omitempty"`
	Priority        int64               `yaml:"priority,omitempty" bson:"priority,omitempty"`
	ExecTimeoutSecs int                 `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	DependsOn       parserDependencies  `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	Commands        []PluginCommandConf `yaml:"commands,omitempty" bson:"commands,omitempty"`
	Tags            parserStringSlice   `yaml:"tags,omitempty" bson:"tags,omitempty"`
	RunOn           parserStringSlice   `yaml:"run_on,omitempty" bson:"run_on,omitempty"`
	Patchable       *bool               `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly       *bool               `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	AllowForGitTag  *bool               `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly      *bool               `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	Stepback        *bool               `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	MustHaveResults *bool               `yaml:"must_have_test_results,omitempty" bson:"must_have_test_results,omitempty"`
}

func (pp *ParserProject) Insert() error {
	return db.Insert(ParserProjectCollection, pp)
}

func (pp *ParserProject) MarshalBSON() ([]byte, error) {
	return mgobson.Marshal(pp)
}

func (pp *ParserProject) MarshalYAML() (interface{}, error) {
	for i, pt := range pp.Tasks {
		for j := range pt.Commands {
			if err := pp.Tasks[i].Commands[j].resolveParams(); err != nil {
				return nil, errors.Wrapf(err, "error marshalling commands for task")
			}
		}
	}

	// returning a pointer causes MarshalYAML to get stuck in infinite recursion
	return *pp, nil
}

type displayTask struct {
	Name           string   `yaml:"name,omitempty" bson:"name,omitempty"`
	ExecutionTasks []string `yaml:"execution_tasks,omitempty" bson:"execution_tasks,omitempty"`
}

// helper methods for task tag evaluations
func (pt *parserTask) name() string   { return pt.Name }
func (pt *parserTask) tags() []string { return pt.Tags }

// parserDependency represents the intermediary state for referencing dependencies.
type parserDependency struct {
	TaskSelector  taskSelector `yaml:",inline"`
	Status        string       `yaml:"status,omitempty" bson:"status,omitempty"`
	PatchOptional bool         `yaml:"patch_optional,omitempty" bson:"patch_optional,omitempty"`
}

// parserDependencies is a type defined for unmarshalling both a single
// dependency or multiple dependencies into a slice.
type parserDependencies []parserDependency

// UnmarshalYAML reads YAML into an array of parserDependency. It will
// successfully unmarshal arrays of dependency entries or single dependency entry.
func (pds *parserDependencies) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first check if we are handling a single dep that is not in an array.
	pd := parserDependency{}
	if err := unmarshal(&pd); err == nil {
		*pds = parserDependencies([]parserDependency{pd})
		return nil
	}
	var slice []parserDependency
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pds = parserDependencies(slice)
	return nil
}

// UnmarshalYAML reads YAML into a parserDependency. A single selector string
// will be also be accepted.
func (pd *parserDependency) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type copyType parserDependency
	var copy copyType
	if err := unmarshal(&copy); err != nil {
		// try unmarshalling for a single-string selector instead
		if err := unmarshal(&pd.TaskSelector); err != nil {
			return err
		}
		otherFields := struct {
			Status        string `yaml:"status"`
			PatchOptional bool   `yaml:"patch_optional"`
		}{}
		// ignore error here: expected to fail considering the single-string selector
		_ = unmarshal(&otherFields)
		pd.Status = otherFields.Status
		pd.PatchOptional = otherFields.PatchOptional
		return nil
	}
	*pd = parserDependency(copy)
	if pd.TaskSelector.Name == "" {
		return errors.WithStack(errors.New("task selector must have a name"))
	}
	return nil
}

// TaskSelector handles the selection of specific task/variant combinations
// in the context of dependencies. //TODO no export?
type taskSelector struct {
	Name    string           `yaml:"name,omitempty"`
	Variant *variantSelector `yaml:"variant,omitempty" bson:"variant,omitempty"`
}

// TaskSelectors is a helper type for parsing arrays of TaskSelector.
type taskSelectors []taskSelector

// VariantSelector handles the selection of a variant, either by a id/tag selector
// or by matching against matrix axis values.
type variantSelector struct {
	StringSelector string           `yaml:"string_selector,omitempty" bson:"string_selector,omitempty"`
	MatrixSelector matrixDefinition `yaml:"matrix_selector,omitempty" bson:"matrix_selector,omitempty"`
}

// UnmarshalYAML allows variants to be referenced as single selector strings or
// as a matrix definition. This works by first attempting to unmarshal the YAML
// into a string and then falling back to the matrix.
func (vs *variantSelector) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	// ignore errors here, because there may be other fields that are valid with single-string selectors
	var onlySelector string
	_ = unmarshal(&onlySelector)
	if onlySelector != "" {
		vs.StringSelector = onlySelector
		return nil
	}

	md := matrixDefinition{}
	if err := unmarshal(&md); err != nil {
		return err
	}
	if len(md) == 0 {
		return errors.New("variant selector must not be empty")
	}
	vs.MatrixSelector = md
	return nil
}

func (vs *variantSelector) MarshalYAML() (interface{}, error) {
	if vs == nil || vs.StringSelector == "" {
		return nil, nil
	}
	// Note: Generate tasks will not work with matrix variant selectors,
	// since this will only marshal the string part of a variant selector.
	return vs.StringSelector, nil
}

// UnmarshalYAML reads YAML into an array of TaskSelector. It will
// successfully unmarshal arrays of dependency selectors or a single selector.
func (tss *taskSelectors) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal a single selector
	var single taskSelector
	if err := unmarshal(&single); err == nil {
		*tss = taskSelectors([]taskSelector{single})
		return nil
	}
	var slice []taskSelector
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*tss = taskSelectors(slice)
	return nil
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the TaskSelector struct.
func (ts *taskSelector) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if _ = unmarshal(&onlySelector); onlySelector != "" {
		ts.Name = onlySelector
		return nil
	}
	// we define a new type so that we can grab the yaml struct tags without the struct methods,
	// preventing infinite recursion on the UnmarshalYAML() method.
	type copyType taskSelector
	var tsc copyType
	if err := unmarshal(&tsc); err != nil {
		return err
	}
	if tsc.Name == "" {
		return errors.WithStack(errors.New("task selector must have a name"))
	}
	*ts = taskSelector(tsc)
	return nil
}

// parserBV is a helper type storing intermediary variant definitions.
type parserBV struct {
	Name          string             `yaml:"name,omitempty" bson:"name,omitempty"`
	DisplayName   string             `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
	Expansions    util.Expansions    `yaml:"expansions,omitempty" bson:"expansions,omitempty"`
	Tags          parserStringSlice  `yaml:"tags,omitempty,omitempty" bson:"tags,omitempty"`
	Modules       parserStringSlice  `yaml:"modules,omitempty" bson:"modules,omitempty"`
	Disabled      bool               `yaml:"disabled,omitempty" bson:"disabled,omitempty"`
	Push          bool               `yaml:"push,omitempty" bson:"push,omitempty"`
	BatchTime     *int               `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	CronBatchTime string             `yaml:"cron,omitempty" bson:"cron,omitempty"`
	Stepback      *bool              `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	RunOn         parserStringSlice  `yaml:"run_on,omitempty" bson:"run_on,omitempty"`
	Tasks         parserBVTaskUnits  `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	DisplayTasks  []displayTask      `yaml:"display_tasks,omitempty" bson:"display_tasks,omitempty"`
	DependsOn     parserDependencies `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	// If Activate is set to false, then we don't initially activate the build variant.
	Activate *bool `yaml:"activate,omitempty" bson:"activate,omitempty"`

	// internal matrix stuff
	MatrixId  string      `yaml:"matrix_id,omitempty" bson:"matrix_id,omitempty"`
	MatrixVal matrixValue `yaml:"matrix_val,omitempty" bson:"matrix_val,omitempty"`
	Matrix    *matrix     `yaml:"matrix,omitempty" bson:"matrix,omitempty"`

	MatrixRules []ruleAction `yaml:"matrix_rules,omitempty" bson:"matrix_rules,omitempty"`
}

// helper methods for variant tag evaluations
func (pbv *parserBV) name() string   { return pbv.Name }
func (pbv *parserBV) tags() []string { return pbv.Tags }

func (pbv *parserBV) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first attempt to unmarshal into a matrix
	m := matrix{}
	merr := unmarshal(&m)
	if merr == nil {
		if m.Id != "" {
			*pbv = parserBV{Matrix: &m}
			return nil
		}
	}
	// otherwise use a BV copy type to skip this Unmarshal method
	type copyType parserBV
	var bv copyType
	if err := unmarshal(&bv); err != nil {
		return errors.WithStack(err)
	}
	// it's possible the matrix has already been correctly stored in the build variant
	if bv.Matrix != nil && bv.Matrix.Id != "" {
		*pbv = parserBV(bv)
		return nil
	}

	if bv.Name == "" {
		// if we're here, it's very likely that the user was building a matrix but broke
		// the syntax, so we try and surface the matrix error if they used "matrix_name".
		if m.Id != "" {
			return errors.Wrap(merr, "parsing matrix")
		}
		return errors.New("buildvariant missing name")
	}
	*pbv = parserBV(bv)
	return nil
}

// canMerge checks that all fields are empty besides name and tasks;
// otherwise, the variant is being defined more than once.
func (pbv *parserBV) canMerge() bool {
	return pbv.Name != "" &&
		len(pbv.Tasks) != 0 &&
		pbv.DisplayName == "" &&
		pbv.Expansions == nil &&
		pbv.Tags == nil &&
		pbv.Modules == nil &&
		!pbv.Disabled &&
		!pbv.Push &&
		pbv.BatchTime == nil &&
		pbv.CronBatchTime == "" &&
		pbv.Stepback == nil &&
		pbv.RunOn == nil &&
		pbv.DisplayTasks == nil &&
		pbv.DependsOn == nil &&
		pbv.Activate == nil &&
		pbv.MatrixId == "" &&
		pbv.MatrixVal == nil &&
		pbv.Matrix == nil &&
		pbv.MatrixRules == nil
}

// parserBVTaskUnit is a helper type storing intermediary variant task configurations.
type parserBVTaskUnit struct {
	Name             string             `yaml:"name,omitempty" bson:"name,omitempty"`
	Patchable        *bool              `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly        *bool              `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	AllowForGitTag   *bool              `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly       *bool              `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	Priority         int64              `yaml:"priority,omitempty" bson:"priority,omitempty"`
	DependsOn        parserDependencies `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	ExecTimeoutSecs  int                `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	Stepback         *bool              `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	Distros          parserStringSlice  `yaml:"distros,omitempty" bson:"distros,omitempty"`
	RunOn            parserStringSlice  `yaml:"run_on,omitempty" bson:"run_on,omitempty"` // Alias for "Distros" TODO: deprecate Distros
	CommitQueueMerge bool               `yaml:"commit_queue_merge,omitempty" bson:"commit_queue_merge,omitempty"`
	// Use a *int for 2 possible states
	// nil - not overriding the project setting
	// non-nil - overriding the project setting with this BatchTime
	BatchTime *int `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	// If CronBatchTime is not empty, then override the project settings with cron syntax,
	// with BatchTime and CronBatchTime being mutually exclusive.
	CronBatchTime string `yaml:"cron,omitempty" bson:"cron,omitempty"`
	// If Activate is set to false, then we don't initially activate the task.
	Activate *bool `yaml:"activate,omitempty" bson:"activate,omitempty"`
}

// UnmarshalYAML allows the YAML parser to read both a single selector string or
// a fully defined parserBVTaskUnit.
func (pbvt *parserBVTaskUnit) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var onlySelector string
	if err := unmarshal(&onlySelector); err == nil {
		if onlySelector != "" {
			pbvt.Name = onlySelector
			return nil
		}
	}
	// we define a new type so that we can grab the YAML struct tags without the struct methods,
	// preventing infinite recursion on the UnmarshalYAML() method.
	type copyType parserBVTaskUnit
	var copy copyType
	if err := unmarshal(&copy); err != nil {
		return err
	}
	if copy.Name == "" {
		return errors.New("buildvariant task selector must have a name")
	}
	// logic for aliasing the "distros" field to "run_on"
	if len(copy.Distros) > 0 {
		if len(copy.RunOn) > 0 {
			return errors.New("cannot use both 'run_on' and 'distros' fields")
		}
		copy.RunOn, copy.Distros = copy.Distros, nil
	}
	*pbvt = parserBVTaskUnit(copy)
	return nil
}

// parserBVTaskUnits is a helper type for handling arrays of parserBVTaskUnit.
type parserBVTaskUnits []parserBVTaskUnit

// UnmarshalYAML allows the YAML parser to read both a single parserBVTaskUnit or
// an array of them into a slice.
func (pbvts *parserBVTaskUnits) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// first, attempt to unmarshal just a selector string
	var single parserBVTaskUnit
	if err := unmarshal(&single); err == nil {
		*pbvts = parserBVTaskUnits([]parserBVTaskUnit{single})
		return nil
	}
	var slice []parserBVTaskUnit
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pbvts = parserBVTaskUnits(slice)
	return nil
}

// parserStringSlice is YAML helper type that accepts both an array of strings
// or single string value during unmarshalling.
type parserStringSlice []string

// UnmarshalYAML allows the YAML parser to read both a single string or
// an array of them into a slice.
func (pss *parserStringSlice) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*pss = []string{single}
		return nil
	}
	var slice []string
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*pss = slice
	return nil
}

// LoadProjectForVersion returns the project for a version, either from the parser project or the config string.
// If read from the config string and shouldSave is set, the resulting parser project will be saved.
func LoadProjectForVersion(v *Version, id string, shouldSave bool) (*Project, *ParserProject, error) {
	var pp *ParserProject
	var err error

	pp, err = ParserProjectFindOneById(v.Id)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding parser project")
	}

	// if parser project config number is old then we should default to legacy
	if pp != nil && pp.ConfigUpdateNumber >= v.ConfigUpdateNumber {
		if pp.Functions == nil {
			pp.Functions = map[string]*YAMLCommandSet{}
		}
		pp.Identifier = utility.ToStringPtr(id)
		var p *Project
		p, err = TranslateProject(pp)
		return p, pp, err
	}

	if v.Config == "" {
		return nil, nil, errors.New("version has no config")
	}
	p := &Project{}
	// opts empty because project yaml with `include` will not hit this case
	ctx := context.Background()
	pp, err = LoadProjectInto(ctx, []byte(v.Config), nil, id, p)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error loading project")
	}
	pp.Id = v.Id
	pp.Identifier = utility.ToStringPtr(id)
	pp.ConfigUpdateNumber = v.ConfigUpdateNumber
	pp.CreateTime = v.CreateTime

	if shouldSave {
		if err = pp.TryUpsert(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"project": id,
				"version": v.Id,
				"message": "error inserting parser project for version",
			}))
			return nil, nil, errors.Wrap(err, "error updating version with project")
		}
	}
	return p, pp, nil
}

func GetProjectFromBSON(data []byte) (*Project, error) {
	pp := &ParserProject{}
	if err := bson.Unmarshal(data, pp); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling bson into parser project")
	}
	return TranslateProject(pp)
}

// LoadProjectInto loads the raw data from the config file into project
// and sets the project's identifier field to identifier. Tags are evaluated. Returns the intermediate step.
// If reading from a version config, LoadProjectForVersion should be used to persist the resulting parser project.
// opts is used to look up files on github if the main parser project has an Include.
func LoadProjectInto(ctx context.Context, data []byte, opts *GetProjectOpts, identifier string, project *Project) (*ParserProject, error) {
	intermediateProject, err := createIntermediateProject(data)
	if err != nil {
		return nil, errors.Wrapf(err, LoadProjectError)
	}

	// return intermediateProject even if we run into issues to show merge progress
	for _, path := range intermediateProject.Include {
		if opts == nil {
			err = errors.New("Trying to open include files with empty opts")
			grip.Critical(message.NewError(errors.WithStack(err)))
			return nil, errors.Wrapf(err, LoadProjectError)
		}
		// read from the patch diff if a change has been made in any of the include files
		if opts.ReadFileFrom == ReadFromPatch || opts.ReadFileFrom == ReadFromPatchDiff {
			if opts.PatchOpts.patch != nil && opts.PatchOpts.patch.ConfigChanged(path.FileName) {
				opts.ReadFileFrom = ReadFromPatchDiff
			} else {
				opts.ReadFileFrom = ReadFromPatch
			}
		}
		var yaml []byte
		opts.Identifier = identifier
		if path.Module != "" {
			opts.RemotePath = path.FileName
			yaml, err = retrieveFileForModule(ctx, *opts, intermediateProject.Modules, path.Module)
		} else {
			yaml, err = retrieveFile(ctx, *opts)
		}
		if err != nil {
			return intermediateProject, errors.Wrapf(err, "%s: failed to retrieve file '%s'", LoadProjectError, path.FileName)
		}
		add, err := createIntermediateProject(yaml)
		if err != nil {
			return intermediateProject, errors.Wrapf(err, LoadProjectError)
		}
		err = intermediateProject.mergeMultipleProjectConfigs(add)
		if err != nil {
			return intermediateProject, errors.Wrapf(err, LoadProjectError)
		}
	}
	intermediateProject.Include = nil

	// return project even with errors
	p, err := TranslateProject(intermediateProject)
	if p != nil {
		*project = *p
	}
	project.Identifier = identifier
	return intermediateProject, errors.Wrapf(err, LoadProjectError)
}

const (
	ReadfromGithub    = "github"
	ReadFromLocal     = "local"
	ReadFromPatch     = "patch"
	ReadFromPatchDiff = "patch_diff"
)

type GetProjectOpts struct {
	Ref          *ProjectRef
	PatchOpts    *PatchOpts
	LocalModules map[string]string
	RemotePath   string
	Revision     string
	Token        string
	ReadFileFrom string
	Identifier   string
}

type PatchOpts struct {
	patch *patch.Patch
	env   evergreen.Environment
}

func retrieveFile(ctx context.Context, opts GetProjectOpts) ([]byte, error) {
	if opts.RemotePath == "" && opts.Ref != nil {
		opts.RemotePath = opts.Ref.RemotePath
	}
	switch opts.ReadFileFrom {
	case ReadFromLocal:
		fileContents, err := ioutil.ReadFile(opts.RemotePath)
		if err != nil {
			return nil, errors.Wrap(err, "error reading project config")
		}
		return fileContents, nil
	case ReadFromPatch:
		fileContents, err := getFileForPatchDiff(ctx, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "could not fetch remote configuration file")
		}
		return fileContents, nil
	case ReadFromPatchDiff:
		originalConfig, err := getFileForPatchDiff(ctx, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "could not fetch remote configuration file")
		}
		fileContents, err := MakePatchedConfig(ctx, opts.PatchOpts.env, opts.PatchOpts.patch, opts.RemotePath, string(originalConfig))
		if err != nil {
			return nil, errors.Wrapf(err, "could not patch remote configuration file")
		}
		return fileContents, nil
	default:
		if opts.Token == "" {
			conf, err := evergreen.GetConfig()
			if err != nil {
				return nil, errors.Wrap(err, "can't get evergreen configuration")
			}
			ghToken, err := conf.GetGithubOauthToken()
			if err != nil {
				return nil, errors.Wrap(err, "can't get Github OAuth token from configuration")
			}
			opts.Token = ghToken
		}
		configFile, err := thirdparty.GetGithubFile(ctx, opts.Token, opts.Ref.Owner, opts.Ref.Repo, opts.RemotePath, opts.Revision)
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching project file for '%s' at '%s'", opts.Identifier, opts.Revision)
		}
		fileContents, err := base64.StdEncoding.DecodeString(*configFile.Content)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to decode config file for '%s'", opts.Identifier)
		}
		return fileContents, nil
	}
}

func retrieveFileForModule(ctx context.Context, opts GetProjectOpts, modules ModuleList, moduleName string) ([]byte, error) {
	// Look through any given local modules first
	if path, ok := opts.LocalModules[moduleName]; ok {
		moduleOpts := GetProjectOpts{
			RemotePath:   fmt.Sprintf("%s/%s", path, opts.RemotePath),
			ReadFileFrom: ReadFromLocal,
		}
		return retrieveFile(ctx, moduleOpts)
	} else if opts.ReadFileFrom == ReadFromLocal {
		return nil, errors.Errorf("local path for module '%s' is unspecified", moduleName)
	}
	// Retrieve from github
	module, err := GetModuleByName(modules, moduleName)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get module for module name '%s'", moduleName)
	}
	repoOwner, repoName := module.GetRepoOwnerAndName()
	moduleOpts := GetProjectOpts{
		Ref: &ProjectRef{
			Owner: repoOwner,
			Repo:  repoName,
		},
		RemotePath:   opts.RemotePath,
		Revision:     module.Branch,
		Token:        opts.Token,
		ReadFileFrom: ReadfromGithub,
		Identifier:   moduleName,
	}
	return retrieveFile(ctx, moduleOpts)
}

func getFileForPatchDiff(ctx context.Context, opts GetProjectOpts) ([]byte, error) {
	if opts.PatchOpts == nil {
		return nil, errors.New("patch not passed in")
	}
	if opts.Ref == nil {
		return nil, errors.New("project not passed in")
	}
	var projectFileBytes []byte
	githubFile, err := thirdparty.GetGithubFile(ctx, opts.Token, opts.Ref.Owner,
		opts.Ref.Repo, opts.RemotePath, opts.Revision)
	if err != nil {
		// if the project file doesn't exist, but our patch includes a project file,
		// we try to apply the diff and proceed.
		if !(opts.PatchOpts.patch.ConfigChanged(opts.RemotePath) && thirdparty.IsFileNotFound(err)) {
			// return an error if the github error is network/auth-related or we aren't patching the config
			return nil, errors.Wrapf(err, "Could not get github file at '%s/%s'@%s: %s", opts.Ref.Owner,
				opts.Ref.Repo, opts.RemotePath, opts.Revision)
		}
	} else {
		// we successfully got the project file in base64, so we decode it
		projectFileBytes, err = base64.StdEncoding.DecodeString(*githubFile.Content)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not decode github file at '%s/%s'@%s: %s", opts.Ref.Owner,
				opts.Ref.Repo, opts.RemotePath, opts.Revision)
		}
	}
	return projectFileBytes, nil
}

func GetProjectFromFile(ctx context.Context, opts GetProjectOpts) (*Project, *ParserProject, error) {
	fileContents, err := retrieveFile(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	config := Project{}
	pp, err := LoadProjectInto(ctx, fileContents, &opts, opts.Ref.Id, &config)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error parsing config file for '%s'", opts.Ref.Id)
	}
	return &config, pp, nil
}

// createIntermediateProject marshals the supplied YAML into our
// intermediate project representation (i.e. before selectors or
// matrix logic has been evaluated).
func createIntermediateProject(yml []byte) (*ParserProject, error) {
	p := &ParserProject{}
	if err := util.UnmarshalYAMLWithFallback(yml, p); err != nil {
		yamlErr := thirdparty.YAMLFormatError{Message: err.Error()}
		return nil, errors.Wrap(yamlErr, "error unmarshalling into parser project")
	}
	if p.Functions == nil {
		p.Functions = map[string]*YAMLCommandSet{}
	}

	return p, nil
}

// TranslateProject converts our intermediate project representation into
// the Project type that Evergreen actually uses. Errors are added to
// pp.errors and pp.warnings and must be checked separately.
func TranslateProject(pp *ParserProject) (*Project, error) {
	// Transfer top level fields
	proj := &Project{
		Enabled:                utility.FromBoolPtr(pp.Enabled),
		Stepback:               utility.FromBoolPtr(pp.Stepback),
		PreErrorFailsTask:      utility.FromBoolPtr(pp.PreErrorFailsTask),
		PostErrorFailsTask:     utility.FromBoolPtr(pp.PostErrorFailsTask),
		OomTracker:             utility.FromBoolPtr(pp.OomTracker),
		BatchTime:              utility.FromIntPtr(pp.BatchTime),
		Owner:                  utility.FromStringPtr(pp.Owner),
		Repo:                   utility.FromStringPtr(pp.Repo),
		RemotePath:             utility.FromStringPtr(pp.RemotePath),
		Branch:                 utility.FromStringPtr(pp.Branch),
		Identifier:             utility.FromStringPtr(pp.Identifier),
		DisplayName:            utility.FromStringPtr(pp.DisplayName),
		CommandType:            utility.FromStringPtr(pp.CommandType),
		Ignore:                 pp.Ignore,
		Parameters:             pp.Parameters,
		Pre:                    pp.Pre,
		Post:                   pp.Post,
		EarlyTermination:       pp.EarlyTermination,
		Timeout:                pp.Timeout,
		CallbackTimeout:        utility.FromIntPtr(pp.CallbackTimeout),
		Modules:                pp.Modules,
		Functions:              pp.Functions,
		ExecTimeoutSecs:        utility.FromIntPtr(pp.ExecTimeoutSecs),
		Loggers:                pp.Loggers,
		TaskAnnotationSettings: pp.TaskAnnotationSettings,
		BuildBaronSettings:     pp.BuildBaronSettings,
		PerfEnabled:            utility.FromBoolPtr(pp.PerfEnabled),
		DeactivatePrevious:     utility.FromBoolPtr(pp.DeactivatePrevious),
	}
	catcher := grip.NewBasicCatcher()
	tse := NewParserTaskSelectorEvaluator(pp.Tasks)
	tgse := newTaskGroupSelectorEvaluator(pp.TaskGroups)
	ase := NewAxisSelectorEvaluator(pp.Axes)
	regularBVs, matrices := sieveMatrixVariants(pp.BuildVariants)

	matrixVariants, errs := buildMatrixVariants(pp.Axes, ase, matrices)
	catcher.Extend(errs)
	buildVariants := append(regularBVs, matrixVariants...)
	vse := NewVariantSelectorEvaluator(buildVariants, ase)
	proj.Tasks, proj.TaskGroups, errs = evaluateTaskUnits(tse, tgse, vse, pp.Tasks, pp.TaskGroups)
	catcher.Extend(errs)

	proj.BuildVariants, errs = evaluateBuildVariants(tse, tgse, vse, buildVariants, pp.Tasks, proj.TaskGroups)
	catcher.Extend(errs)
	return proj, errors.Wrap(catcher.Resolve(), TranslateProjectError)
}

func (pp *ParserProject) AddTask(name string, commands []PluginCommandConf) {
	t := parserTask{
		Name:     name,
		Commands: commands,
	}
	pp.Tasks = append(pp.Tasks, t)
}
func (pp *ParserProject) AddBuildVariant(name, displayName, runOn string, batchTime *int, tasks []string) {
	bv := parserBV{
		Name:        name,
		DisplayName: displayName,
		BatchTime:   batchTime,
		Tasks:       []parserBVTaskUnit{},
	}
	for _, taskName := range tasks {
		bv.Tasks = append(bv.Tasks, parserBVTaskUnit{Name: taskName})
	}
	if runOn != "" {
		bv.RunOn = []string{runOn}
	}
	pp.BuildVariants = append(pp.BuildVariants, bv)
}

// sieveMatrixVariants takes a set of parserBVs and groups them into regular
// buildvariant matrix definitions and matrix definitions.
func sieveMatrixVariants(bvs []parserBV) (regular []parserBV, matrices []matrix) {
	for _, bv := range bvs {
		if bv.Matrix != nil {
			matrices = append(matrices, *bv.Matrix)
		} else {
			regular = append(regular, bv)
		}
	}
	return regular, matrices
}

// evaluateTaskUnits translates intermediate tasks into true ProjectTask types,
// evaluating any selectors in the DependsOn field.
func evaluateTaskUnits(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pts []parserTask, tgs []parserTaskGroup) ([]ProjectTask, []TaskGroup, []error) {
	tasks := []ProjectTask{}
	groups := []TaskGroup{}
	var evalErrs, errs []error
	for _, pt := range pts {
		t := ProjectTask{
			Name:            pt.Name,
			Priority:        pt.Priority,
			ExecTimeoutSecs: pt.ExecTimeoutSecs,
			Commands:        pt.Commands,
			Tags:            pt.Tags,
			RunOn:           pt.RunOn,
			Patchable:       pt.Patchable,
			PatchOnly:       pt.PatchOnly,
			AllowForGitTag:  pt.AllowForGitTag,
			GitTagOnly:      pt.GitTagOnly,
			Stepback:        pt.Stepback,
			MustHaveResults: pt.MustHaveResults,
		}
		if strings.Contains(strings.TrimSpace(pt.Name), " ") {
			evalErrs = append(evalErrs, errors.Errorf("spaces are unauthorized in task names ('%s')", pt.Name))
		}
		t.DependsOn, errs = evaluateDependsOn(tse.tagEval, tgse, vse, pt.DependsOn)
		evalErrs = append(evalErrs, errs...)
		tasks = append(tasks, t)
	}
	for _, ptg := range tgs {
		tg := TaskGroup{
			Name:                    ptg.Name,
			SetupGroupFailTask:      ptg.SetupGroupFailTask,
			TeardownTaskCanFailTask: ptg.TeardownTaskCanFailTask,
			SetupGroupTimeoutSecs:   ptg.SetupGroupTimeoutSecs,
			SetupGroup:              ptg.SetupGroup,
			TeardownGroup:           ptg.TeardownGroup,
			SetupTask:               ptg.SetupTask,
			TeardownTask:            ptg.TeardownTask,
			Tags:                    ptg.Tags,
			MaxHosts:                ptg.MaxHosts,
			Timeout:                 ptg.Timeout,
			ShareProcs:              ptg.ShareProcs,
		}
		if tg.MaxHosts < 1 {
			tg.MaxHosts = 1
		}
		// expand, validate that tasks defined in a group are listed in the project tasks
		var taskNames []string
		for _, taskName := range ptg.Tasks {
			names, err := tse.evalSelector(ParseSelector(taskName))
			if err != nil {
				evalErrs = append(evalErrs, err)
			}
			taskNames = append(taskNames, names...)
		}
		tg.Tasks = taskNames
		groups = append(groups, tg)
	}
	return tasks, groups, evalErrs
}

// evaluateBuildsVariants translates intermediate tasks into true BuildVariant types,
// evaluating any selectors in the Tasks fields.
func evaluateBuildVariants(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pbvs []parserBV, tasks []parserTask, tgs []TaskGroup) ([]BuildVariant, []error) {
	bvs := []BuildVariant{}
	var evalErrs, errs []error
	for _, pbv := range pbvs {
		bv := BuildVariant{
			DisplayName:   pbv.DisplayName,
			Name:          pbv.Name,
			Expansions:    pbv.Expansions,
			Modules:       pbv.Modules,
			Disabled:      pbv.Disabled,
			Push:          pbv.Push,
			BatchTime:     pbv.BatchTime,
			CronBatchTime: pbv.CronBatchTime,
			Activate:      pbv.Activate,
			Stepback:      pbv.Stepback,
			RunOn:         pbv.RunOn,
			Tags:          pbv.Tags,
		}
		bv.Tasks, errs = evaluateBVTasks(tse, tgse, vse, pbv, tasks)

		// evaluate any rules passed in during matrix construction
		for _, r := range pbv.MatrixRules {
			// remove_tasks removes all tasks with matching names
			if len(r.RemoveTasks) > 0 {
				prunedTasks := []BuildVariantTaskUnit{}
				toRemove := []string{}
				for _, t := range r.RemoveTasks {
					removed, err := tse.evalSelector(ParseSelector(t))
					if err != nil {
						evalErrs = append(evalErrs, errors.Wrap(err, "remove rule"))
						continue
					}
					toRemove = append(toRemove, removed...)
				}
				for _, t := range bv.Tasks {
					if !utility.StringSliceContains(toRemove, t.Name) {
						prunedTasks = append(prunedTasks, t)
					}
				}
				bv.Tasks = prunedTasks
			}

			// add_tasks adds the given BuildVariantTasks, returning errors for any collisions
			if len(r.AddTasks) > 0 {
				// cache existing tasks so we can check for duplicates
				existing := map[string]*BuildVariantTaskUnit{}
				for i, t := range bv.Tasks {
					existing[t.Name] = &bv.Tasks[i]
				}

				var added []BuildVariantTaskUnit
				pbv.Tasks = r.AddTasks
				added, errs = evaluateBVTasks(tse, tgse, vse, pbv, tasks)
				evalErrs = append(evalErrs, errs...)
				// check for conflicting duplicates
				for _, t := range added {
					if old, ok := existing[t.Name]; ok {
						if !reflect.DeepEqual(t, *old) {
							evalErrs = append(evalErrs, errors.Errorf(
								"conflicting definitions of added tasks '%v': %v != %v", t.Name, t, old))
						}
					} else {
						bv.Tasks = append(bv.Tasks, t)
						existing[t.Name] = &t
					}
				}
			}
		}

		tgMap := map[string]TaskGroup{}
		for _, tg := range tgs {
			tgMap[tg.Name] = tg
		}
		dtse := newDisplayTaskSelectorEvaluator(bv, tasks, tgMap)

		// check that display tasks contain real tasks that are not duplicated
		bvTasks := make(map[string]struct{})        // map of all execution tasks
		displayTaskContents := make(map[string]int) // map of execution tasks in a display task
		for _, t := range bv.Tasks {
			if tg, exists := tgMap[t.Name]; exists {
				for _, tgTask := range tg.Tasks {
					bvTasks[tgTask] = struct{}{}
				}
			} else {
				bvTasks[t.Name] = struct{}{}
			}
		}

		// save display task if it contains valid execution tasks
		for _, dt := range pbv.DisplayTasks {
			projectDt := patch.DisplayTask{Name: dt.Name}
			if _, exists := bvTasks[dt.Name]; exists {
				errs = append(errs, fmt.Errorf("display task %s cannot have the same name as an execution task", dt.Name))
				continue
			}

			//resolve tags for display tasks
			tasks := []string{}
			for _, et := range dt.ExecutionTasks {
				results, err := dtse.evalSelector(ParseSelector(et))
				if err != nil {
					errs = append(errs, err)
				}
				tasks = append(tasks, results...)
			}

			for _, et := range tasks {
				if _, exists := bvTasks[et]; !exists {
					errs = append(errs, fmt.Errorf("display task %s contains execution task %s which does not exist in build variant", dt.Name, et))
				} else {
					projectDt.ExecTasks = append(projectDt.ExecTasks, et)
					displayTaskContents[et]++
				}
			}
			if len(projectDt.ExecTasks) > 0 {
				bv.DisplayTasks = append(bv.DisplayTasks, projectDt)
			}
		}
		for taskId, count := range displayTaskContents {
			if count > 1 {
				errs = append(errs, fmt.Errorf("execution task %s is listed in more than 1 display task", taskId))
				bv.DisplayTasks = nil
			}
		}

		evalErrs = append(evalErrs, errs...)
		bvs = append(bvs, bv)
	}
	return bvs, evalErrs
}

// evaluateBVTasks translates intermediate tasks into true BuildVariantTaskUnit types,
// evaluating any selectors referencing tasks, and further evaluating any selectors
// in the DependsOn field of those tasks.
func evaluateBVTasks(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pbv parserBV, tasks []parserTask) ([]BuildVariantTaskUnit, []error) {
	var evalErrs, errs []error
	ts := []BuildVariantTaskUnit{}
	taskUnitsByName := map[string]BuildVariantTaskUnit{}
	tasksByName := map[string]parserTask{}
	for _, t := range tasks {
		tasksByName[t.Name] = t
	}
	for _, pbvt := range pbv.Tasks {
		// evaluate each task against both the task and task group selectors
		// only error if both selectors error because each task should only be found
		// in one or the other
		var names, temp []string
		var err1, err2 error
		isGroup := false
		if tse != nil {
			temp, err1 = tse.evalSelector(ParseSelector(pbvt.Name))
			names = append(names, temp...)
		}
		if tgse != nil {
			temp, err2 = tgse.evalSelector(ParseSelector(pbvt.Name))
			if len(temp) > 0 {
				names = append(names, temp...)
				isGroup = true
			}
		}
		if err1 != nil && err2 != nil {
			evalErrs = append(evalErrs, err1, err2)
			continue
		}
		// create new task definitions--duplicates must have the same status requirements
		for _, name := range names {
			parserTask := tasksByName[name]
			// create a new task by copying the task that selected it,
			// so we can preserve the "Variant" and "Status" field.
			t := getParserBuildVariantTaskUnit(name, parserTask, pbvt)

			// Task-level dependencies defined in the variant override variant-level dependencies which override
			// task-level dependencies defined in the task.
			var dependsOn parserDependencies
			if len(pbvt.DependsOn) > 0 {
				dependsOn = pbvt.DependsOn
			} else if len(pbv.DependsOn) > 0 {
				dependsOn = pbv.DependsOn
			} else if len(parserTask.DependsOn) > 0 {
				dependsOn = parserTask.DependsOn
			}
			t.DependsOn, errs = evaluateDependsOn(tse.tagEval, tgse, vse, dependsOn)
			evalErrs = append(evalErrs, errs...)
			t.IsGroup = isGroup

			// add the new task if it doesn't already exists (we must avoid conflicting status fields)
			if old, ok := taskUnitsByName[t.Name]; !ok {
				ts = append(ts, t)
				taskUnitsByName[t.Name] = t
			} else {
				// it's already in the new list, so we check to make sure the status definitions match.
				if !reflect.DeepEqual(t, old) {
					evalErrs = append(evalErrs, errors.Errorf(
						"conflicting definitions of build variant tasks '%v': %v != %v", name, t, old))
					continue
				}
			}
		}
	}
	return ts, evalErrs
}

// getParserBuildVariantTaskUnit combines the parser project's task definition with the build variant task definition.
// DependsOn will be handled separately.
func getParserBuildVariantTaskUnit(name string, pt parserTask, bvt parserBVTaskUnit) BuildVariantTaskUnit {
	res := BuildVariantTaskUnit{
		Name:             name,
		Patchable:        bvt.Patchable,
		PatchOnly:        bvt.PatchOnly,
		AllowForGitTag:   bvt.AllowForGitTag,
		GitTagOnly:       bvt.GitTagOnly,
		Priority:         bvt.Priority,
		ExecTimeoutSecs:  bvt.ExecTimeoutSecs,
		Stepback:         bvt.Stepback,
		RunOn:            bvt.RunOn,
		CommitQueueMerge: bvt.CommitQueueMerge,
		CronBatchTime:    bvt.CronBatchTime,
		BatchTime:        bvt.BatchTime,
		Activate:         bvt.Activate,
	}
	if res.Priority == 0 {
		res.Priority = pt.Priority
	}
	if res.Patchable == nil {
		res.Patchable = pt.Patchable
	}
	if res.PatchOnly == nil {
		res.PatchOnly = pt.PatchOnly
	}
	if res.AllowForGitTag == nil {
		res.AllowForGitTag = pt.AllowForGitTag
	}
	if res.GitTagOnly == nil {
		res.GitTagOnly = pt.GitTagOnly
	}
	if res.ExecTimeoutSecs == 0 {
		res.ExecTimeoutSecs = pt.ExecTimeoutSecs
	}
	if res.Stepback == nil {
		res.Stepback = pt.Stepback
	}
	if len(res.RunOn) == 0 {
		// first consider that we may be using the legacy "distros" field
		res.RunOn = bvt.Distros
	}
	if len(res.RunOn) == 0 {
		res.RunOn = pt.RunOn
	}
	return res
}

// evaluateDependsOn expands any selectors in a dependency definition.
func evaluateDependsOn(tse *tagSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	deps []parserDependency) ([]TaskUnitDependency, []error) {
	var evalErrs []error
	var err error
	newDeps := []TaskUnitDependency{}
	newDepsByNameAndVariant := map[TVPair]TaskUnitDependency{}
	for _, d := range deps {
		var names []string

		if d.TaskSelector.Name == AllDependencies {
			// * is a special case for dependencies, so don't eval it
			names = []string{AllDependencies}
		} else {
			var temp []string
			var err1, err2 error
			if tse != nil {
				temp, err1 = tse.evalSelector(ParseSelector(d.TaskSelector.Name))
				names = append(names, temp...)
			}
			if tgse != nil {
				temp, err2 = tgse.evalSelector(ParseSelector(d.TaskSelector.Name))
				names = append(names, temp...)
			}
			if err1 != nil && err2 != nil {
				evalErrs = append(evalErrs, err1, err2)
				continue
			}
		}
		// we default to handle the empty variant, but expand the list of variants
		// if the variant field is set.
		variants := []string{""}
		if d.TaskSelector.Variant != nil {
			// * is a special case for dependencies, so don't eval it
			if d.TaskSelector.Variant.MatrixSelector == nil && d.TaskSelector.Variant.StringSelector == AllVariants {
				variants = []string{AllVariants}
			} else {
				variants, err = vse.evalSelector(d.TaskSelector.Variant)
				if err != nil {
					evalErrs = append(evalErrs, err)
					continue
				}
			}
		}
		// create new dependency definitions--duplicates must have the same status requirements
		for _, name := range names {
			for _, variant := range variants {
				// create a newDep by copying the dep that selected it,
				// so we can preserve the "Status" and "PatchOptional" field.
				newDep := TaskUnitDependency{
					Name:          name,
					Variant:       variant,
					Status:        d.Status,
					PatchOptional: d.PatchOptional,
				}
				// add the new dep if it doesn't already exist (we must avoid conflicting status fields)
				if oldDep, ok := newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}]; !ok {
					newDeps = append(newDeps, newDep)
					newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}] = newDep
				} else {
					// it's already in the new list, so we check to make sure the status definitions match.
					if !reflect.DeepEqual(newDep, oldDep) {
						evalErrs = append(evalErrs, errors.Errorf(
							"conflicting definitions of dependency '%v': %v != %v", name, newDep, oldDep))
						continue
					}
				}
			}
		}
	}
	return newDeps, evalErrs
}

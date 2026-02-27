package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const LoadProjectError = "load project error(s)"
const TranslateProjectError = "error translating project"
const TranslateProjectConfigError = "unmarshalling project config from YAML"
const EmptyConfigurationError = "received empty configuration file"

// MaxConfigSetPriority represents the highest value for a task's priority a user can set in theit
// config YAML.
const MaxConfigSetPriority = 50

// priorityBypassProjects is a list of projects that can set task priorities above MaxConfigSetPriority.
var priorityBypassProjects = []string{"mongo-release"}

// This file contains the infrastructure for turning a YAML project configuration
// into a usable Project struct. A basic overview of the project parsing process is:
//
// First, the YAML bytes are unmarshalled into an intermediary ParserProject.
// The ParserProject's internal types define custom YAML unmarshal hooks, allowing
// users to do things like offer a single definition where we expect a list, e.g.
//   `tags: "single_tag"` instead of the more verbose `tags: ["single_tag"]`
// or refer to a task by a single selector. Custom YAML handling allows us to
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
// From a mental model perspective, the ParserProject is the project
// configuration after YAML rules have been evaluated (e.g. matching YAML fields
// to Go struct fields, evaluating YAML anchors and aliases), but before any
// Evergreen-specific evaluation rules have been applied. For example, Evergreen
// has a custom feature to support tagging a set of tasks and expanding those
// tags into a list of tasks under the build variant's list of tasks (i.e.
// ".tagname" syntax). In the ParserProject, these are stored as the unexpanded
// tag text (i.e. ".tagname"), and these tags are not evaluated until the
// ParserProject is turned into a final Project.
type ParserProject struct {
	// Id and ConfigdUpdateNumber are not pointers because they are only used internally
	Id string `yaml:"_id" bson:"_id"` // should be the same as the version's ID
	// UpdatedByGenerators is used to determine if the parser project needs to be re-saved or not.
	UpdatedByGenerators []string `yaml:"updated_by_generators,omitempty" bson:"updated_by_generators,omitempty"`
	// List of yamls to merge
	Include []parserInclude `yaml:"include,omitempty" bson:"include,omitempty"`

	// Beginning of ParserProject mergeable fields (this comment is used by the linter).
	Stepback           *bool                      `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	PreTimeoutSecs     *int                       `yaml:"pre_timeout_secs,omitempty" bson:"pre_timeout_secs,omitempty"`
	PostTimeoutSecs    *int                       `yaml:"post_timeout_secs,omitempty" bson:"post_timeout_secs,omitempty"`
	PreErrorFailsTask  *bool                      `yaml:"pre_error_fails_task,omitempty" bson:"pre_error_fails_task,omitempty"`
	PostErrorFailsTask *bool                      `yaml:"post_error_fails_task,omitempty" bson:"post_error_fails_task,omitempty"`
	OomTracker         *bool                      `yaml:"oom_tracker,omitempty" bson:"oom_tracker,omitempty"`
	Ps                 *string                    `yaml:"ps,omitempty" bson:"ps,omitempty"`
	Owner              *string                    `yaml:"owner,omitempty" bson:"owner,omitempty"`
	Repo               *string                    `yaml:"repo,omitempty" bson:"repo,omitempty"`
	RemotePath         *string                    `yaml:"remote_path,omitempty" bson:"remote_path,omitempty"`
	Branch             *string                    `yaml:"branch,omitempty" bson:"branch,omitempty"`
	Identifier         *string                    `yaml:"identifier,omitempty" bson:"identifier,omitempty"`
	DisplayName        *string                    `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
	CommandType        *string                    `yaml:"command_type,omitempty" bson:"command_type,omitempty"`
	Ignore             parserStringSlice          `yaml:"ignore,omitempty" bson:"ignore,omitempty"`
	Parameters         []ParameterInfo            `yaml:"parameters,omitempty" bson:"parameters,omitempty"`
	Pre                *YAMLCommandSet            `yaml:"pre,omitempty" bson:"pre,omitempty"`
	Post               *YAMLCommandSet            `yaml:"post,omitempty" bson:"post,omitempty"`
	Timeout            *YAMLCommandSet            `yaml:"timeout,omitempty" bson:"timeout,omitempty"`
	CallbackTimeout    *int                       `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs,omitempty"`
	Modules            []Module                   `yaml:"modules,omitempty" bson:"modules,omitempty"`
	BuildVariants      []parserBV                 `yaml:"buildvariants,omitempty" bson:"buildvariants,omitempty"`
	Functions          map[string]*YAMLCommandSet `yaml:"functions,omitempty" bson:"functions,omitempty"`
	TaskGroups         []parserTaskGroup          `yaml:"task_groups,omitempty" bson:"task_groups,omitempty"`
	Tasks              []parserTask               `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	ExecTimeoutSecs    *int                       `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	TimeoutSecs        *int                       `yaml:"timeout_secs,omitempty" bson:"timeout_secs,omitempty"`
	CreateTime         time.Time                  `yaml:"create_time,omitempty" bson:"create_time,omitempty"`

	// DisableMergeQueuePathFiltering, if true, skips path filtering for merge queue versions.
	DisableMergeQueuePathFiltering *bool `yaml:"disable_merge_queue_path_filtering,omitempty" bson:"disable_merge_queue_path_filtering,omitempty"`

	// Matrix code
	Axes []matrixAxis `yaml:"axes,omitempty" bson:"axes,omitempty"`
} // End of ParserProject mergeable fields (this comment is used by the linter).

type parserTaskGroup struct {
	Name                     string            `yaml:"name,omitempty" bson:"name,omitempty"`
	MaxHosts                 int               `yaml:"max_hosts,omitempty" bson:"max_hosts,omitempty"`
	SetupGroup               *YAMLCommandSet   `yaml:"setup_group,omitempty" bson:"setup_group,omitempty"`
	SetupGroupCanFailTask    bool              `yaml:"setup_group_can_fail_task,omitempty" bson:"setup_group_can_fail_task,omitempty"`
	SetupGroupTimeoutSecs    int               `yaml:"setup_group_timeout_secs,omitempty" bson:"setup_group_timeout_secs,omitempty"`
	TeardownGroup            *YAMLCommandSet   `yaml:"teardown_group,omitempty" bson:"teardown_group,omitempty"`
	TeardownGroupTimeoutSecs int               `yaml:"teardown_group_timeout_secs,omitempty" bson:"teardown_group_timeout_secs,omitempty"`
	SetupTask                *YAMLCommandSet   `yaml:"setup_task,omitempty" bson:"setup_task,omitempty"`
	SetupTaskCanFailTask     bool              `yaml:"setup_task_can_fail_task,omitempty" bson:"setup_task_can_fail_task,omitempty"`
	SetupTaskTimeoutSecs     int               `yaml:"setup_task_timeout_secs,omitempty" bson:"setup_task_timeout_secs,omitempty"`
	TeardownTask             *YAMLCommandSet   `yaml:"teardown_task,omitempty" bson:"teardown_task,omitempty"`
	TeardownTaskCanFailTask  bool              `yaml:"teardown_task_can_fail_task,omitempty" bson:"teardown_task_can_fail_task,omitempty"`
	TeardownTaskTimeoutSecs  int               `yaml:"teardown_task_timeout_secs,omitempty" bson:"teardown_task_timeout_secs,omitempty"`
	Timeout                  *YAMLCommandSet   `yaml:"timeout,omitempty" bson:"timeout,omitempty"`
	CallbackTimeoutSecs      int               `yaml:"callback_timeout_secs,omitempty" bson:"callback_timeout_secs,omitempty"`
	Tasks                    []string          `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	Tags                     parserStringSlice `yaml:"tags,omitempty" bson:"tags,omitempty"`
	ShareProcs               bool              `yaml:"share_processes,omitempty" bson:"share_processes,omitempty"`
}

func (ptg *parserTaskGroup) name() string   { return ptg.Name }
func (ptg *parserTaskGroup) tags() []string { return ptg.Tags }

// parserTask represents an intermediary state of task definitions.
type parserTask struct {
	Name              string                    `yaml:"name,omitempty" bson:"name,omitempty"`
	Priority          int64                     `yaml:"priority,omitempty" bson:"priority,omitempty"`
	ExecTimeoutSecs   int                       `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	DependsOn         parserDependencies        `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	Commands          []PluginCommandConf       `yaml:"commands,omitempty" bson:"commands,omitempty"`
	Tags              parserStringSlice         `yaml:"tags,omitempty" bson:"tags,omitempty"`
	RunOn             parserStringSlice         `yaml:"run_on,omitempty" bson:"run_on,omitempty"`
	Patchable         *bool                     `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly         *bool                     `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	Disable           *bool                     `yaml:"disable,omitempty" bson:"disable,omitempty"`
	AllowForGitTag    *bool                     `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly        *bool                     `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	AllowedRequesters []evergreen.UserRequester `yaml:"allowed_requesters,omitempty" bson:"allowed_requesters,omitempty"`
	Stepback          *bool                     `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	MustHaveResults   *bool                     `yaml:"must_have_test_results,omitempty" bson:"must_have_test_results,omitempty"`
	Ps                *string                   `yaml:"ps,omitempty" bson:"ps,omitempty"`
}

func (pp *ParserProject) Insert(ctx context.Context) error {
	return db.Insert(ctx, ParserProjectCollection, pp)
}

func (pp *ParserProject) MarshalBSON() ([]byte, error) {
	return mgobson.Marshal(pp)
}

func (pp *ParserProject) MarshalYAML() (any, error) {
	for i, pt := range pp.Tasks {
		for j := range pt.Commands {
			if err := pp.Tasks[i].Commands[j].resolveParams(); err != nil {
				return nil, errors.Wrapf(err, "marshalling command '%s' for task", pp.Tasks[i].Commands[j].GetDisplayName())
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
	TaskSelector       taskSelector `yaml:",inline"`
	Status             string       `yaml:"status,omitempty" bson:"status,omitempty"`
	PatchOptional      bool         `yaml:"patch_optional,omitempty" bson:"patch_optional,omitempty"`
	OmitGeneratedTasks bool         `yaml:"omit_generated_tasks,omitempty" bson:"omit_generated_tasks,omitempty"`
}

// parserDependencies is a type defined for unmarshalling both a single
// dependency or multiple dependencies into a slice.
type parserDependencies []parserDependency

// UnmarshalYAML reads YAML into an array of parserDependency. It will
// successfully unmarshal arrays of dependency entries or single dependency entry.
func (pds *parserDependencies) UnmarshalYAML(unmarshal func(any) error) error {
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
func (pd *parserDependency) UnmarshalYAML(unmarshal func(any) error) error {
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

type parserInclude struct {
	FileName string `yaml:"filename,omitempty" bson:"filename,omitempty"`
	Module   string `yaml:"module,omitempty" bson:"module,omitempty"`
}

// TaskSelector handles the selection of specific task/variant combinations
// in the context of dependencies.
type taskSelector struct {
	Name    string           `yaml:"name,omitempty"`
	Variant *variantSelector `yaml:"variant,omitempty" bson:"variant,omitempty"`
}

// VariantSelector handles the selection of a variant, either by a id/tag selector
// or by matching against matrix axis values.
type variantSelector struct {
	StringSelector string           `yaml:"string_selector,omitempty" bson:"string_selector,omitempty"`
	MatrixSelector matrixDefinition `yaml:"matrix_selector,omitempty" bson:"matrix_selector,omitempty"`
}

// UnmarshalYAML allows variants to be referenced as single selector strings or
// as a matrix definition. This works by first attempting to unmarshal the YAML
// into a string and then falling back to the matrix.
func (vs *variantSelector) UnmarshalYAML(unmarshal func(any) error) error {
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

func (vs *variantSelector) MarshalYAML() (any, error) {
	if vs == nil || vs.StringSelector == "" {
		return nil, nil
	}
	// Note: Generate tasks will not work with matrix variant selectors,
	// since this will only marshal the string part of a variant selector.
	return vs.StringSelector, nil
}

// UnmarshalYAML allows tasks to be referenced as single selector strings.
// This works by first attempting to unmarshal the YAML into a string
// and then falling back to the TaskSelector struct.
func (ts *taskSelector) UnmarshalYAML(unmarshal func(any) error) error {
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
	Name               string            `yaml:"name,omitempty" bson:"name,omitempty"`
	DisplayName        string            `yaml:"display_name,omitempty" bson:"display_name,omitempty"`
	Expansions         util.Expansions   `yaml:"expansions,omitempty" bson:"expansions,omitempty"`
	Tags               parserStringSlice `yaml:"tags,omitempty,omitempty" bson:"tags,omitempty"`
	Modules            parserStringSlice `yaml:"modules,omitempty" bson:"modules,omitempty"`
	Disable            *bool             `yaml:"disable,omitempty" bson:"disable,omitempty"`
	BatchTime          *int              `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	CronBatchTime      string            `yaml:"cron,omitempty" bson:"cron,omitempty"`
	Stepback           *bool             `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	DeactivatePrevious *bool             `yaml:"deactivate_previous,omitempty" bson:"deactivate_previous,omitempty"`

	RunOn        parserStringSlice  `yaml:"run_on,omitempty" bson:"run_on,omitempty"`
	Tasks        parserBVTaskUnits  `yaml:"tasks,omitempty" bson:"tasks,omitempty"`
	DisplayTasks []displayTask      `yaml:"display_tasks,omitempty" bson:"display_tasks,omitempty"`
	DependsOn    parserDependencies `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	// If Activate is set to false, then we don't initially activate the build variant.
	Activate          *bool                     `yaml:"activate,omitempty" bson:"activate,omitempty"`
	Patchable         *bool                     `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly         *bool                     `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	AllowForGitTag    *bool                     `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly        *bool                     `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	AllowedRequesters []evergreen.UserRequester `yaml:"allowed_requesters,omitempty" bson:"allowed_requesters,omitempty"`
	Paths             parserStringSlice         `yaml:"paths,omitempty" bson:"paths,omitempty"`

	// internal matrix stuff
	MatrixId  string      `yaml:"matrix_id,omitempty" bson:"matrix_id,omitempty"`
	MatrixVal matrixValue `yaml:"matrix_val,omitempty" bson:"matrix_val,omitempty"`
	Matrix    *matrix     `yaml:"matrix,omitempty" bson:"matrix,omitempty"`

	MatrixRules []ruleAction `yaml:"matrix_rules,omitempty" bson:"matrix_rules,omitempty"`
}

// helper methods for variant tag evaluations
func (pbv *parserBV) name() string   { return pbv.Name }
func (pbv *parserBV) tags() []string { return pbv.Tags }

func (pbv *parserBV) UnmarshalYAML(unmarshal func(any) error) error {
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
		return errors.New("build variant missing name")
	}
	*pbv = parserBV(bv)
	return nil
}

// canMerge checks that all fields are empty besides name and tasks;
// otherwise, the variant is being defined more than once.
func (pbv *parserBV) canMerge() bool {
	return pbv.Name != "" &&
		len(pbv.Tasks)+len(pbv.DisplayTasks) != 0 &&
		pbv.DisplayName == "" &&
		pbv.Expansions == nil &&
		pbv.Tags == nil &&
		pbv.Modules == nil &&
		pbv.Disable == nil &&
		pbv.BatchTime == nil &&
		pbv.CronBatchTime == "" &&
		pbv.Stepback == nil &&
		pbv.DeactivatePrevious == nil &&
		pbv.RunOn == nil &&
		pbv.DependsOn == nil &&
		pbv.Activate == nil &&
		pbv.Patchable == nil &&
		pbv.PatchOnly == nil &&
		pbv.AllowForGitTag == nil &&
		pbv.GitTagOnly == nil &&
		len(pbv.AllowedRequesters) == 0 &&
		len(pbv.Paths) == 0 &&
		pbv.MatrixId == "" &&
		pbv.MatrixVal == nil &&
		pbv.Matrix == nil &&
		pbv.MatrixRules == nil
}

// parserBVTaskUnit is a helper type storing intermediary variant task configurations.
type parserBVTaskUnit struct {
	Name              string                    `yaml:"name,omitempty" bson:"name,omitempty"`
	Patchable         *bool                     `yaml:"patchable,omitempty" bson:"patchable,omitempty"`
	PatchOnly         *bool                     `yaml:"patch_only,omitempty" bson:"patch_only,omitempty"`
	Disable           *bool                     `yaml:"disable,omitempty" bson:"disable,omitempty"`
	AllowForGitTag    *bool                     `yaml:"allow_for_git_tag,omitempty" bson:"allow_for_git_tag,omitempty"`
	GitTagOnly        *bool                     `yaml:"git_tag_only,omitempty" bson:"git_tag_only,omitempty"`
	AllowedRequesters []evergreen.UserRequester `yaml:"allowed_requesters,omitempty" bson:"allowed_requesters,omitempty"`
	Priority          int64                     `yaml:"priority,omitempty" bson:"priority,omitempty"`
	DependsOn         parserDependencies        `yaml:"depends_on,omitempty" bson:"depends_on,omitempty"`
	ExecTimeoutSecs   int                       `yaml:"exec_timeout_secs,omitempty" bson:"exec_timeout_secs,omitempty"`
	Stepback          *bool                     `yaml:"stepback,omitempty" bson:"stepback,omitempty"`
	Distros           parserStringSlice         `yaml:"distros,omitempty" bson:"distros,omitempty"`
	RunOn             parserStringSlice         `yaml:"run_on,omitempty" bson:"run_on,omitempty"` // Alias for "Distros" TODO: deprecate Distros
	// Use a *int for 2 possible states
	// nil - not overriding the project setting
	// non-nil - overriding the project setting with this BatchTime
	BatchTime *int `yaml:"batchtime,omitempty" bson:"batchtime,omitempty"`
	// If CronBatchTime is not empty, then override the project settings with cron syntax,
	// with BatchTime and CronBatchTime being mutually exclusive.
	CronBatchTime string `yaml:"cron,omitempty" bson:"cron,omitempty"`
	// If Activate is set to false, then we don't initially activate the task.
	Activate *bool `yaml:"activate,omitempty" bson:"activate,omitempty"`
	// PS is the command to run for process diagnostics.
	PS *string `yaml:"ps,omitempty" bson:"ps,omitempty"`
	// CreateCheckRun will create a check run on GitHub if set.
	CreateCheckRun *CheckRun `yaml:"create_check_run,omitempty" bson:"create_check_run,omitempty"`
}

// UnmarshalYAML allows the YAML parser to read both a single selector string or
// a fully defined parserBVTaskUnit.
func (pbvt *parserBVTaskUnit) UnmarshalYAML(unmarshal func(any) error) error {
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
		return errors.New("build variant task selector must have a name")
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
func (pbvts *parserBVTaskUnits) UnmarshalYAML(unmarshal func(any) error) error {
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
func (pss *parserStringSlice) UnmarshalYAML(unmarshal func(any) error) error {
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

// HasSpecificActivation returns if the build variant task specifies an activation condition that
// overrides the default, such as cron/batchtime, disabling the task, or explicitly deactivating it.
func (bvt *parserBVTaskUnit) hasSpecificActivation() bool {
	return bvt.BatchTime != nil || bvt.CronBatchTime != "" ||
		!utility.FromBoolTPtr(bvt.Activate) || utility.FromBoolPtr(bvt.Disable)
}

// HasSpecificActivation returns if the build variant specifies an activation condition that
// overrides the default, such as cron/batchtime, disabling the task, or explicitly deactivating it.
func (bv *parserBV) hasSpecificActivation() bool {
	return bv.BatchTime != nil || bv.CronBatchTime != "" ||
		!utility.FromBoolTPtr(bv.Activate) || utility.FromBoolPtr(bv.Disable)
}

// FindAndTranslateProjectForPatch translates a parser project for a patch into
// a project. This assumes that the version may not exist yet (i.e. the patch is
// unfinalized); otherwise FindAndTranslateProjectForVersion is equivalent.
func FindAndTranslateProjectForPatch(ctx context.Context, settings *evergreen.Settings, p *patch.Patch) (*Project, *ParserProject, error) {
	if p.ProjectStorageMethod != "" {
		pp, err := ParserProjectFindOneByID(ctx, settings, p.ProjectStorageMethod, p.Id.Hex())
		if err != nil {
			return nil, nil, errors.Wrapf(err, "finding parser project '%s' stored using method '%s'", p.Id.Hex(), p.ProjectStorageMethod)
		}
		if pp == nil {
			return nil, nil, errors.Errorf("parser project '%s' not found in storage using method '%s'", p.Id.Hex(), p.ProjectStorageMethod)
		}
		project, err := TranslateProject(pp)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "translating project '%s'", pp.Id)
		}
		return project, pp, nil
	}

	// This fallback handles the case where the patch is already finalized.
	v, err := VersionFindOneId(ctx, p.Version)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "finding version '%s' for patch '%s'", p.Version, p.Id.Hex())
	}
	if v == nil {
		return nil, nil, errors.Errorf("version '%s' not found for patch '%s'", p.Version, p.Id.Hex())
	}
	return FindAndTranslateProjectForVersion(ctx, settings, v, false)
}

// FindAndTranslateProjectForVersion translates a parser project for a version into a Project.
// Also sets the project ID. If the preGeneration flag is true, this function will attempt to
// fetch and translate the parser project from before it was modified by generate.tasks
func FindAndTranslateProjectForVersion(ctx context.Context, settings *evergreen.Settings, v *Version, preGeneration bool) (*Project, *ParserProject, error) {
	var pp *ParserProject
	var err error
	if preGeneration {
		preGeneratedId := preGeneratedParserProjectId(v.Id)
		pp, err = ParserProjectFindOneByID(ctx, settings, v.PreGenerationProjectStorageMethod, preGeneratedId)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "finding parser project '%s'", preGeneratedId)
		}
		// Fall back to the parser project post-generation if the pre-generation parser project was not found
	}
	if pp == nil {
		pp, err = ParserProjectFindOneByID(ctx, settings, v.ProjectStorageMethod, v.Id)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "finding parser project '%s'", v.Id)
		}
		if pp == nil {
			return nil, nil, errors.Errorf("parser project not found for version '%s'", v.Id)
		}
	}
	// Setting the translated project's identifier is necessary here because the
	// version always has an identifier, but some old parser projects used to
	// not be stored with the identifier.
	pp.Identifier = utility.ToStringPtr(v.Identifier)
	var p *Project
	p, err = TranslateProject(pp)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "translating parser project '%s'", v.Id)
	}
	return p, pp, err
}

// LoadProjectInfoForVersion returns the project info for a version from its parser project.
func LoadProjectInfoForVersion(ctx context.Context, settings *evergreen.Settings, v *Version, id string) (ProjectInfo, error) {
	var err error

	pRef, err := FindMergedProjectRef(ctx, id, "", false)
	if err != nil {
		return ProjectInfo{}, errors.Wrap(err, "finding project ref")
	}
	if pRef == nil {
		return ProjectInfo{}, errors.Errorf("project ref '%s' not found", id)
	}
	var pc *ProjectConfig
	if pRef.IsVersionControlEnabled() {
		pc, err = FindProjectConfigById(ctx, v.Id)
		if err != nil {
			return ProjectInfo{}, errors.Wrap(err, "finding project config")
		}
	}
	p, pp, err := FindAndTranslateProjectForVersion(ctx, settings, v, false)
	if err != nil {
		return ProjectInfo{}, errors.Wrap(err, "translating project")
	}
	return ProjectInfo{
		Project:             p,
		IntermediateProject: pp,
		Config:              pc,
	}, nil
}

func GetProjectFromBSON(data []byte) (*Project, error) {
	pp := &ParserProject{}
	if err := bson.Unmarshal(data, pp); err != nil {
		return nil, errors.Wrap(err, "unmarshalling BSON into parser project")
	}
	return TranslateProject(pp)
}

func processIntermediateProjectIncludes(ctx context.Context, identifier string, intermediateProject *ParserProject,
	include parserInclude, outputYAMLs chan<- yamlTuple, projectOpts *GetProjectOpts, dirs *gitIncludeDirs, workerIdx int) {
	// Make a copy of opts because otherwise parts of opts would be
	// modified concurrently.  Note, however, that Ref and PatchOpts are
	// themselves pointers, so should not be modified.
	localOpts := &GetProjectOpts{
		Ref:                       projectOpts.Ref,
		PatchOpts:                 projectOpts.PatchOpts,
		LocalModules:              projectOpts.LocalModules,
		RemotePath:                include.FileName,
		Revision:                  projectOpts.Revision,
		ReadFileFrom:              projectOpts.ReadFileFrom,
		Identifier:                identifier,
		UnmarshalStrict:           projectOpts.UnmarshalStrict,
		LocalModuleIncludes:       projectOpts.LocalModuleIncludes,
		ReferencePatchID:          projectOpts.ReferencePatchID,
		ReferenceManifestID:       projectOpts.ReferenceManifestID,
		AutoUpdateModuleRevisions: projectOpts.AutoUpdateModuleRevisions,
		IsIncludedFile:            true,
	}
	if projectOpts.Ref != nil {
		localOpts.Worktree = dirs.getWorktreeForOwnerRepoWorker(projectOpts.Ref.Owner, projectOpts.Ref.Repo, workerIdx)
	}
	localOpts.UpdateReadFileFrom(include.FileName)

	var yaml []byte
	var err error
	grip.Debug(message.Fields{
		"message":     "retrieving included YAML file",
		"remote_path": localOpts.RemotePath,
		"read_from":   localOpts.ReadFileFrom,
		"module":      include.Module,
	})
	if include.Module != "" {
		yaml, err = retrieveFileForModule(ctx, *localOpts, intermediateProject.Modules, include, dirs, workerIdx)
		err = errors.Wrapf(err, "%s: retrieving file for module '%s'", LoadProjectError, include.Module)
	} else {
		yaml, err = retrieveFile(ctx, *localOpts)
		err = errors.Wrapf(err, "%s: retrieving file for include '%s'", LoadProjectError, include.FileName)
	}
	outputYAMLs <- yamlTuple{
		yaml: yaml,
		name: include.FileName,
		err:  err,
	}
}

type yamlTuple struct {
	yaml []byte
	name string
	err  error
}

// LoadProjectInto loads the raw data from the config file into project
// and sets the project's ID field to projectID. Tags are evaluated. Returns the intermediate step.
// If reading from a version config, LoadProjectInfoForVersion should be used to persist the resulting parser project.
// opts is used to look up files on github if the main parser project has an Include.
func LoadProjectInto(ctx context.Context, data []byte, opts *GetProjectOpts, projectID string, project *Project) (*ParserProject, error) {
	attrs := []attribute.KeyValue{
		attribute.String(evergreen.ProjectIDOtelAttribute, projectID),
	}
	if opts != nil && opts.Ref != nil {
		attrs = append(attrs,
			attribute.String(evergreen.ProjectOrgOtelAttribute, opts.Ref.Owner),
			attribute.String(evergreen.ProjectRepoOtelAttribute, opts.Ref.Repo),
			attribute.String(evergreen.ProjectIdentifierOtelAttribute, opts.Ref.Identifier),
		)
	}
	ctx, span := tracer.Start(ctx, "LoadProjectInto", trace.WithAttributes(attrs...))
	defer span.End()

	unmarshalStrict := false
	if opts != nil {
		unmarshalStrict = opts.UnmarshalStrict
	}
	intermediateProject, err := createIntermediateProject(data, unmarshalStrict)
	if err != nil {
		return nil, errors.Wrapf(err, LoadProjectError)
	}

	if !slices.Contains(priorityBypassProjects, projectID) {
		capParserPriorities(intermediateProject)
	}

	if len(intermediateProject.Include) > 0 {
		if err := mergeIncludes(ctx, projectID, intermediateProject, opts); err != nil {
			return nil, errors.Wrap(err, "merging included files")
		}
	}

	// Return project even with errors.
	p, err := TranslateProject(intermediateProject)
	if p != nil {
		*project = *p
	}
	project.Identifier = projectID

	// Remove includes once the project is translated since translate project saves number of includes.
	// Intermediate project is used to save parser project as a YAML so removing the includes verifies that
	// they have been processed.
	intermediateProject.Include = nil

	return intermediateProject, errors.Wrapf(err, LoadProjectError)
}

// mergeIncludes merges all included files into the intermediateProject.
func mergeIncludes(ctx context.Context, projectID string, intermediateProject *ParserProject, opts *GetProjectOpts) error {
	ctx, span := tracer.Start(ctx, "mergeIncludes")
	defer span.End()

	if opts == nil {
		err := errors.New("trying to open include files with empty options")
		return errors.Wrapf(err, LoadProjectError)
	}

	// Be polite. Don't make more than 10 concurrent requests to GitHub.
	const maxWorkers = 10
	workers := util.Min(maxWorkers, len(intermediateProject.Include))

	dirs, err := setupParallelGitIncludeDirs(ctx, intermediateProject.Modules, intermediateProject.Include, workers, opts)
	if err != nil {
		msg := message.Fields{
			"message":    "could not set up git include directories for includes, will fall back to using GitHub API to retrieve include files",
			"project_id": projectID,
			"revision":   opts.Revision,
		}
		if opts.Ref != nil {
			msg["project_identifier"] = opts.Ref.Identifier
			msg["owner"] = opts.Ref.Owner
			msg["repo"] = opts.Ref.Repo
		}
		grip.Warning(message.WrapError(err, msg))
	}
	defer func() {
		// This is a best-effort attempt to clean up the temporary git
		// directories after includes are processed. However, if it errors, it's
		// not really an issue because the git directories don't use much disk
		// space and app servers are not long-lived enough for a few leftover
		// files to cause issues.
		if err := dirs.cleanup(); err != nil {
			msg := message.Fields{
				"message":    "could not clean up git directories after including files, may leave behind temporary git files in the file system",
				"project_id": projectID,
				"revision":   opts.Revision,
			}
			if opts.Ref != nil {
				msg["project_identifier"] = opts.Ref.Identifier
				msg["owner"] = opts.Ref.Owner
				msg["repo"] = opts.Ref.Repo
			}
			grip.Warning(message.WrapError(err, msg))
		}
	}()

	wg := sync.WaitGroup{}
	outputYAMLs := make(chan yamlTuple, len(intermediateProject.Include))
	includesToProcess := make(chan parserInclude, len(intermediateProject.Include))

	for _, path := range intermediateProject.Include {
		includesToProcess <- path
	}
	close(includesToProcess)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			for include := range includesToProcess {
				processIntermediateProjectIncludes(ctx, projectID, intermediateProject, include, outputYAMLs, opts, dirs, workerIdx)
			}
		}(i)
	}

	// This order is deliberate:
	// 1. Wait for the workers, since sending on a `nil` `outputYAMLs` would panic.
	// 2. Close `outputYAMLs` so that the later `range` statement over it will stop after it's drained.
	wg.Wait()
	close(outputYAMLs)

	yamlMap := map[string][]byte{}
	catcher := grip.NewBasicCatcher()
	for elem := range outputYAMLs {
		catcher.Add(elem.err)
		if thirdparty.IsFileNotFound(errors.Cause(elem.err)) {
			return errors.Wrap(elem.err, "getting includes")
		}
		if elem.yaml != nil {
			yamlMap[elem.name] = elem.yaml
		}
	}

	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "getting includes")
	}

	// We promise to iterate over includes in the order they are defined.
	for _, path := range intermediateProject.Include {
		if _, ok := yamlMap[path.FileName]; !ok {
			return errors.WithStack(errors.Errorf("yaml was nil in map for %s, but it never should be", path.FileName))
		}
		add, err := createIntermediateProject(yamlMap[path.FileName], opts.UnmarshalStrict)
		if err != nil {
			// Return intermediateProject even if we run into issues to show merge progress.
			return errors.Wrapf(err, "%s: loading file '%s'", LoadProjectError, path.FileName)
		}
		if err = intermediateProject.mergeMultipleParserProjects(add); err != nil {
			// Return intermediateProject even if we run into issues to show merge progress.
			return errors.Wrapf(err, "%s: merging file '%s'", LoadProjectError, path.FileName)
		}
	}

	return nil
}

// gitIncludeDirs contains information about git clone and worktree directories
// that can be used when including YAML files.
type gitIncludeDirs struct {
	// clonesForOwnerRepo maps a git owner/repo to the directory where its
	// git clone is located.
	clonesForOwnerRepo map[gitOwnerRepo]string
	// worktreesForOwnerRepo maps a git owner/repo to the list of available
	// worktree directories.
	worktreesForOwnerRepo map[gitOwnerRepo][]string
}

type gitOwnerRepo struct {
	owner string
	repo  string
}

func newGitOwnerRepo(owner, repo string) gitOwnerRepo {
	return gitOwnerRepo{
		owner: owner,
		repo:  repo,
	}
}

func (d *gitIncludeDirs) getWorktreeForOwnerRepoWorker(owner, repo string, workerNum int) string {
	if d == nil {
		return ""
	}
	if d.worktreesForOwnerRepo == nil {
		return ""
	}

	ownerRepo := newGitOwnerRepo(owner, repo)
	worktrees := d.worktreesForOwnerRepo[ownerRepo]
	if workerNum >= len(worktrees) {
		return ""
	}
	return worktrees[workerNum]
}

func (d *gitIncludeDirs) cleanup() error {
	if d == nil {
		return nil
	}
	catcher := grip.NewBasicCatcher()
	for ownerRepo, dir := range d.clonesForOwnerRepo {
		catcher.Wrapf(os.RemoveAll(dir), "cleaning up git clone directory '%s' for '%s/%s'", dir, ownerRepo.owner, ownerRepo.repo)
	}
	return catcher.Resolve()
}

// setupParallelGitIncludeDirs sets up git clones and worktrees in preparation
// for retrieving included YAML files using git. numWorkers determines how
// many worktrees are created for each included repo.
// This is primarily a performance optimization. If using git to retrieve
// included files from GitHub, repeating the same git setup for every single
// included file is expensive and slow.
func setupParallelGitIncludeDirs(ctx context.Context, modules ModuleList, includes []parserInclude, numWorkers int, opts *GetProjectOpts) (dirs *gitIncludeDirs, err error) {
	if !readFromRemoteSource(opts.ReadFileFrom) {
		return nil, nil
	}
	if opts.Ref == nil {
		// Ref could be nil when the CLI is loading the project for validation.
		return nil, nil
	}

	ctx, span := tracer.Start(ctx, "setupParallelGitIncludeDirs", trace.WithAttributes(
		attribute.String(evergreen.ProjectIDOtelAttribute, opts.Ref.Id),
		attribute.String(evergreen.ProjectIdentifierOtelAttribute, opts.Ref.Identifier),
		attribute.String(evergreen.ProjectOrgOtelAttribute, opts.Ref.Owner),
		attribute.String(evergreen.ProjectRepoOtelAttribute, opts.Ref.Repo),
	))
	defer span.End()

	dirs = &gitIncludeDirs{
		clonesForOwnerRepo:    make(map[gitOwnerRepo]string),
		worktreesForOwnerRepo: make(map[gitOwnerRepo][]string),
	}

	defer func() {
		if err == nil {
			return
		}
		// If this function errored, clean up any intermediate git clone
		// directories and worktrees that were created.
		grip.Warning(message.WrapError(dirs.cleanup(), message.Fields{
			"message":            "could not clean up git clone directory after failing to set up git directories",
			"project_id":         opts.Ref.Id,
			"project_identifier": opts.Ref.Identifier,
		}))
		// Once dirs has been cleaned up, it's no longer valid to use it, so do
		// not return it in the result.
		dirs = nil
	}()

	includedModuleNames := map[string]struct{}{}
	var includesProjectFiles bool
	for _, include := range includes {
		if include.Module != "" {
			includedModuleNames[include.Module] = struct{}{}
		} else {
			includesProjectFiles = true
		}
	}

	type gitInput struct {
		ownerRepo gitOwnerRepo
		revision  string
	}
	type gitOutput struct {
		ownerRepo    gitOwnerRepo
		cloneDir     string
		worktreeDirs []string
		err          error
	}

	// Creating git clones is slow, so for better performance, parallelize the
	// git clones.
	reposToProcess := make(chan gitInput, len(includedModuleNames)+1)
	output := make(chan gitOutput, len(includedModuleNames)+1)

	if includesProjectFiles {
		ownerRepo := newGitOwnerRepo(opts.Ref.Owner, opts.Ref.Repo)
		reposToProcess <- gitInput{
			ownerRepo: ownerRepo,
			revision:  opts.Revision,
		}
	}

	for modName := range includedModuleNames {
		mod, err := GetModuleByName(modules, modName)
		if err != nil {
			return dirs, errors.Wrapf(err, "getting module for module name '%s'", modName)
		}
		repoOwner, repoName, err := mod.GetOwnerAndRepo()
		if err != nil {
			return dirs, errors.Wrapf(err, "getting owner and repo for module '%s'", mod.Name)
		}
		revision, err := getRevisionForRemoteModule(ctx, *mod, modName, *opts)
		if err != nil {
			return dirs, errors.Wrapf(err, "getting revision for module '%s'", mod.Name)
		}

		ownerRepo := newGitOwnerRepo(repoOwner, repoName)
		if _, ok := dirs.clonesForOwnerRepo[ownerRepo]; ok {
			// When including files, there should not be any duplicate repos
			// defined in the modules, and the modules should not use the exact
			// same repo/branch as the project itself.
			grip.Warning(message.Fields{
				"message":            "trying to make multiple git clones of the same repo, skipping duplicate repo",
				"project_id":         opts.Ref.Id,
				"project_identifier": opts.Ref.Identifier,
				"owner":              repoOwner,
				"repo":               repoName,
				"revision":           revision,
				"module":             modName,
			})
			continue
		}

		reposToProcess <- gitInput{
			ownerRepo: ownerRepo,
			revision:  revision,
		}
	}

	close(reposToProcess)

	wg := sync.WaitGroup{}

	numGitWorkers := util.Min(numWorkers, len(reposToProcess))
	for range numGitWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repoData := range reposToProcess {
				cloneDir, worktreeDirs, err := gitCloneAndCreateWorktrees(ctx, repoData.ownerRepo, repoData.revision, numWorkers)
				output <- gitOutput{
					ownerRepo:    repoData.ownerRepo,
					cloneDir:     cloneDir,
					worktreeDirs: worktreeDirs,
					err:          err,
				}
			}
		}()
	}

	wg.Wait()
	close(output)

	catcher := grip.NewBasicCatcher()
	for out := range output {
		if out.cloneDir != "" {
			dirs.clonesForOwnerRepo[out.ownerRepo] = out.cloneDir
		}
		if out.worktreeDirs != nil {
			dirs.worktreesForOwnerRepo[out.ownerRepo] = out.worktreeDirs
		}
		if out.err != nil {
			catcher.Wrapf(out.err, "setting up git clone and worktrees for repo '%s/%s'", out.ownerRepo.owner, out.ownerRepo.repo)
		}
	}

	return dirs, catcher.Resolve()
}

// gitCloneAndCreateWorktrees performs a minimal git clone of the specified repo
// for the specific revision. Once the git clone is finished, it creates git
// worktrees under the clone directory based on numWorktrees.
func gitCloneAndCreateWorktrees(ctx context.Context, ownerRepo gitOwnerRepo, revision string, numWorktrees int) (cloneDir string, worktreeDirs []string, err error) {
	dir, err := thirdparty.GitCloneMinimal(ctx, ownerRepo.owner, ownerRepo.repo, revision)
	if err != nil {
		return "", []string{}, errors.Wrapf(err, "git cloning repo '%s/%s' at revision '%s'", ownerRepo.owner, ownerRepo.repo, revision)
	}

	for i := range numWorktrees {
		worktreeDir := filepath.Join(dir, fmt.Sprintf("worktree-%d", i))
		if err := thirdparty.GitCreateWorktree(ctx, dir, worktreeDir); err != nil {
			return dir, worktreeDirs, errors.Wrapf(err, "creating git worktree for repo '%s/%s'", ownerRepo.owner, ownerRepo.repo)
		}
		worktreeDirs = append(worktreeDirs, worktreeDir)
	}

	return dir, worktreeDirs, nil
}

const (
	ReadFromGithub    = "github"
	ReadFromLocal     = "local"
	ReadFromPatch     = "patch"
	ReadFromPatchDiff = "patch_diff"
)

// readFromRemoteSource returns true if the readFrom option requires retrieving
// a file from a remote source (i.e. GitHub). If ReadFileFrom is empty, the
// default behavior is that it reads from a remote source.
func readFromRemoteSource(readFrom string) bool {
	return utility.StringSliceContains([]string{"", ReadFromGithub, ReadFromPatch, ReadFromPatchDiff}, readFrom)
}

type GetProjectOpts struct {
	Ref          *ProjectRef
	PatchOpts    *PatchOpts
	LocalModules map[string]string
	RemotePath   string
	Revision     string
	// ReadFileFrom determines where the file should be fetched from. If
	// unspecified, the default is ReadFromGithub.
	ReadFileFrom              string
	Identifier                string
	UnmarshalStrict           bool
	LocalModuleIncludes       []patch.LocalModuleInclude
	ReferencePatchID          string
	ReferenceManifestID       string
	AutoUpdateModuleRevisions map[string]string
	// IsIncludedFile indicates whether the file being retrieved is an included
	// YAML file.
	IsIncludedFile bool
	// Worktree is the directory of the git worktree to use when retrieving
	// files via git. Only set if reading a remote file using git.
	Worktree string
}

type PatchOpts struct {
	patch *patch.Patch
	env   evergreen.Environment
}

// UpdateNewFile modifies ReadFileFrom to read from the patch diff
// if the included file has been modified.
func (opts *GetProjectOpts) UpdateReadFileFrom(path string) {
	if opts.ReadFileFrom == ReadFromPatch || opts.ReadFileFrom == ReadFromPatchDiff {
		if opts.PatchOpts.patch != nil && opts.PatchOpts.patch.ShouldPatchFileWithDiff(path) {
			opts.ReadFileFrom = ReadFromPatchDiff
		} else {
			opts.ReadFileFrom = ReadFromPatch
		}
	}
}

// retrieveFile retrieves a file from its source location. If no
// opts.ReadFileFrom is specified, it will default to retrieving the file from
// GitHub.
func retrieveFile(ctx context.Context, opts GetProjectOpts) ([]byte, error) {
	if opts.RemotePath == "" && opts.Ref != nil {
		opts.RemotePath = opts.Ref.RemotePath
	}

	switch opts.ReadFileFrom {
	case ReadFromLocal:
		fileContents, err := os.ReadFile(opts.RemotePath)
		if err != nil {
			return nil, errors.Wrap(err, "reading project config")
		}
		return fileContents, nil
	case ReadFromPatch:
		fileContents, err := getFileForPatchDiff(ctx, opts)
		if err != nil {
			return nil, errors.Wrap(err, "fetching remote configuration file")
		}
		return fileContents, nil
	case ReadFromPatchDiff:
		originalConfig, err := getFileForPatchDiff(ctx, opts)
		if err != nil {
			return nil, errors.Wrap(err, "fetching remote configuration file")
		}
		fileContents, err := MakePatchedConfig(ctx, opts, string(originalConfig))
		if err != nil {
			return nil, errors.Wrap(err, "patching remote configuration file")
		}
		return fileContents, nil
	default:
		ghAppAuth, err := opts.Ref.GetGitHubAppAuthForAPI(ctx)
		grip.Warning(message.WrapError(err, message.Fields{
			"message":    "errored while attempting to get GitHub app for API, will fall back to using Evergreen-internal app",
			"project_id": opts.Ref.Id,
		}))
		useGit := true
		if opts.IsIncludedFile && opts.Worktree == "" {
			// Include files that have a git worktree available should try to
			// use that because it's an optimization to reduce GitHub API calls
			// (includes use a lot of GitHub API calls). However, if it doesn't
			// have a git worktree, this should avoid using git entirely because
			// without a worktree, retrieving every include file without a
			// pre-created worktree is very slow.
			// This can still retrieve the file using the GitHub API even though
			// git is not an option.
			useGit = false
		}
		fileContents, err := thirdparty.GetGitHubFileContent(ctx, opts.Ref.Owner, opts.Ref.Repo, opts.Revision, opts.RemotePath, opts.Worktree, ghAppAuth, useGit)
		if err != nil {
			return nil, errors.Wrapf(err, "fetching project config file for project '%s' at revision '%s'", opts.Ref.Id, opts.Revision)
		}
		return fileContents, nil
	}
}

func retrieveFileForModule(ctx context.Context, opts GetProjectOpts, modules ModuleList, include parserInclude, dirs *gitIncludeDirs, workerIdx int) ([]byte, error) {
	// Check if the module has a local change passed in through the CLI or previous patch.
	if opts.ReferencePatchID != "" {
		p, err := patch.FindOneId(ctx, opts.ReferencePatchID)
		if err != nil {
			return nil, errors.Wrapf(err, "finding patch to repeat '%s'", opts.ReferencePatchID)
		}
		if p == nil {
			return nil, errors.Errorf("patch to repeat '%s' not found", opts.ReferencePatchID)
		}
		opts.LocalModuleIncludes = p.LocalModuleIncludes
	}
	for _, patchedInclude := range opts.LocalModuleIncludes {
		if patchedInclude.Module == include.Module && patchedInclude.FileName == include.FileName {
			return patchedInclude.FileContent, nil
		}
	}

	// Look through any given local modules first
	if path, ok := opts.LocalModules[include.Module]; ok {
		moduleOpts := GetProjectOpts{
			RemotePath:   fmt.Sprintf("%s/%s", path, opts.RemotePath),
			ReadFileFrom: ReadFromLocal,
		}
		return retrieveFile(ctx, moduleOpts)
	} else if opts.ReadFileFrom == ReadFromLocal {
		return nil, errors.Errorf("local path for module '%s' is unspecified", include.Module)
	}
	// Retrieve from github
	module, err := GetModuleByName(modules, include.Module)
	if err != nil {
		return nil, errors.Wrapf(err, "getting module for module name '%s'", include.Module)
	}
	repoOwner, repoName, err := module.GetOwnerAndRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "getting module owner and repo '%s'", module.Name)
	}
	pRef := *opts.Ref
	pRef.Owner = repoOwner
	pRef.Repo = repoName
	moduleOpts := GetProjectOpts{
		Ref:          &pRef,
		RemotePath:   opts.RemotePath,
		ReadFileFrom: ReadFromGithub,
		Identifier:   include.Module,
		Worktree:     dirs.getWorktreeForOwnerRepoWorker(repoOwner, repoName, workerIdx),
	}
	moduleOpts.Revision, err = getRevisionForRemoteModule(ctx, *module, include.Module, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "getting revision for module '%s'", include.Module)
	}

	return retrieveFile(ctx, moduleOpts)
}

// getRevisionForRemoteModule returns the revision or branch to use for the
// given module include. This only works if retrieving files from a remote
// source (e.g. GitHub); it will not handle includes from local modules.
func getRevisionForRemoteModule(ctx context.Context, mod Module, modName string, opts GetProjectOpts) (string, error) {
	if opts.AutoUpdateModuleRevisions != nil {
		if revision, ok := opts.AutoUpdateModuleRevisions[modName]; ok {
			return revision, nil
		}
	}

	if opts.ReferenceManifestID != "" {
		m, err := manifest.FindOne(ctx, manifest.ById(opts.ReferenceManifestID))
		if err != nil {
			return "", errors.Wrapf(err, "finding reference manifest '%s'", opts.ReferenceManifestID)
		}
		// Sometimes the manifest might be nil, in which case we don't want to set the revision.
		if m != nil {
			for name, mod := range m.Modules {
				if name == modName {
					return mod.Revision, nil
				}
			}
		}
	}

	return mod.Branch, nil
}

func getFileForPatchDiff(ctx context.Context, opts GetProjectOpts) ([]byte, error) {
	if opts.PatchOpts == nil {
		return nil, errors.New("patch not passed in")
	}
	if opts.Ref == nil {
		return nil, errors.New("project not passed in")
	}
	ghAppAuth, err := opts.Ref.GetGitHubAppAuthForAPI(ctx)
	grip.Warning(message.WrapError(err, message.Fields{
		"message":    "errored while attempting to get GitHub app for API, will fall back to using Evergreen-internal app",
		"project_id": opts.Ref.Id,
	}))
	useGit := true
	if opts.IsIncludedFile && opts.Worktree == "" {
		// Include files that have a git worktree available should try to
		// use that because it's an optimization to reduce GitHub API calls
		// (includes use a lot of GitHub API calls). However, if it doesn't
		// have a git worktree, this should avoid using git entirely because
		// without a worktree, retrieving every include file without a
		// pre-created worktree is very slow.
		// This can still retrieve the file using the GitHub API even though
		// git is not an option.
		useGit = false
	}
	projectFileBytes, err := thirdparty.GetGitHubFileContent(ctx, opts.Ref.Owner, opts.Ref.Repo, opts.Revision, opts.RemotePath, opts.Worktree, ghAppAuth, useGit)
	if err != nil {
		// if the project file doesn't exist, but our patch includes a project file,
		// we try to apply the diff and proceed.
		if !(opts.PatchOpts.patch.ConfigChanged(opts.RemotePath) && thirdparty.IsFileNotFound(err)) {
			// return an error if the github error is network/auth-related or we aren't patching the config
			return nil, errors.Wrapf(err, "getting GitHub file at '%s/%s'@%s: %s", opts.Ref.Owner,
				opts.Ref.Repo, opts.RemotePath, opts.Revision)
		}
	}
	return projectFileBytes, nil
}

// fetchProjectFilesTimeout is the maximum timeout to fetch project
// configuration files from its source.
const fetchProjectFilesTimeout = time.Minute

// GetProjectFromFile fetches project configuration files from its source (e.g.
// from a patch diff, GitHub, etc).
func GetProjectFromFile(ctx context.Context, opts GetProjectOpts) (ProjectInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchProjectFilesTimeout)
	defer cancel()

	fileContents, err := retrieveFile(ctx, opts)
	if err != nil {
		return ProjectInfo{}, err
	}

	config := Project{}
	pp, err := LoadProjectInto(ctx, fileContents, &opts, opts.Ref.Id, &config)
	if err != nil {
		return ProjectInfo{}, errors.Wrapf(err, "parsing config file for '%s'", opts.Ref.Id)
	}
	var pc *ProjectConfig
	if opts.Ref.IsVersionControlEnabled() {
		pc, err = CreateProjectConfig(fileContents, opts.Ref.Id)
		if err != nil {
			return ProjectInfo{}, errors.Wrapf(err, "parsing project config for '%s'", opts.Ref.Id)
		}
	}
	return ProjectInfo{
		Project:             &config,
		IntermediateProject: pp,
		Config:              pc,
	}, nil
}

// createIntermediateProject marshals the supplied YAML into our
// intermediate project representation (i.e. before selectors or
// matrix logic has been evaluated).
// If unmarshalStrict is true, use the strict version of unmarshalling.
func createIntermediateProject(yml []byte, unmarshalStrict bool) (*ParserProject, error) {
	p := ParserProject{}
	if unmarshalStrict {
		strictProjectWithVariables := struct {
			ParserProject       `yaml:"pp,inline"`
			ProjectConfigFields `yaml:"pc,inline"`
			// Variables is only used to suppress yaml unmarshalling errors related
			// to a non-existent variables field.
			Variables any `yaml:"variables,omitempty" bson:"-"`
		}{}
		if err := util.UnmarshalYAMLStrictWithFallback(yml, &strictProjectWithVariables); err != nil {
			return nil, err
		}
		p = strictProjectWithVariables.ParserProject
	} else {
		if err := util.UnmarshalYAMLWithFallback(yml, &p); err != nil {
			yamlErr := thirdparty.YAMLFormatError{Message: err.Error()}
			return nil, errors.Wrap(yamlErr, "unmarshalling parser project from YAML")
		}
	}

	if p.Functions == nil {
		p.Functions = map[string]*YAMLCommandSet{}
	}

	return &p, nil
}

// capParserPriorities caps all priority values in the parser project at MaxConfigSetPriority.
// It caps priorities for both tasks and build variant tasks.
func capParserPriorities(p *ParserProject) {
	for i := range p.Tasks {
		if p.Tasks[i].Priority > MaxConfigSetPriority {
			p.Tasks[i].Priority = MaxConfigSetPriority
		}
	}
	for i := range p.BuildVariants {
		for j := range p.BuildVariants[i].Tasks {
			if p.BuildVariants[i].Tasks[j].Priority > MaxConfigSetPriority {
				p.BuildVariants[i].Tasks[j].Priority = MaxConfigSetPriority
			}
		}
	}
}

// TranslateProject converts our intermediate project representation into
// the Project type that Evergreen actually uses.
func TranslateProject(pp *ParserProject) (*Project, error) {
	// Transfer top level fields
	proj := &Project{

		Stepback:                       utility.FromBoolPtr(pp.Stepback),
		PreTimeoutSecs:                 utility.FromIntPtr(pp.PreTimeoutSecs),
		PostTimeoutSecs:                utility.FromIntPtr(pp.PostTimeoutSecs),
		PreErrorFailsTask:              utility.FromBoolPtr(pp.PreErrorFailsTask),
		PostErrorFailsTask:             utility.FromBoolPtr(pp.PostErrorFailsTask),
		OomTracker:                     utility.FromBoolTPtr(pp.OomTracker), // oom tracker is true by default
		PS:                             utility.FromStringPtr(pp.Ps),
		Identifier:                     utility.FromStringPtr(pp.Identifier),
		DisplayName:                    utility.FromStringPtr(pp.DisplayName),
		CommandType:                    utility.FromStringPtr(pp.CommandType),
		DisableMergeQueuePathFiltering: utility.FromBoolPtr(pp.DisableMergeQueuePathFiltering),
		Ignore:                         pp.Ignore,
		Parameters:                     pp.Parameters,
		Pre:                            pp.Pre,
		Post:                           pp.Post,
		Timeout:                        pp.Timeout,
		CallbackTimeout:                utility.FromIntPtr(pp.CallbackTimeout),
		Modules:                        pp.Modules,
		Functions:                      pp.Functions,
		ExecTimeoutSecs:                utility.FromIntPtr(pp.ExecTimeoutSecs),
		TimeoutSecs:                    utility.FromIntPtr(pp.TimeoutSecs),
		NumIncludes:                    len(pp.Include),
	}
	catcher := grip.NewBasicCatcher()
	tse := NewParserTaskSelectorEvaluator(pp.Tasks)
	tgse := newTaskGroupSelectorEvaluator(pp.TaskGroups)
	ase := NewAxisSelectorEvaluator(pp.Axes)
	buildVariants, errs := GetVariantsWithMatrices(ase, pp.Axes, pp.BuildVariants)
	catcher.Extend(errs)
	vse := NewVariantSelectorEvaluator(buildVariants, ase)
	proj.Tasks, proj.TaskGroups, errs = evaluateTaskUnits(tse, tgse, vse, pp.Tasks, pp.TaskGroups)
	catcher.Extend(errs)

	proj.BuildVariants, errs = evaluateBuildVariants(tse, tgse, vse, buildVariants, pp.Tasks, proj.TaskGroups)
	catcher.Extend(errs)
	return proj, errors.Wrap(catcher.Resolve(), TranslateProjectError)
}

// Init initializes the parser project with the expected fields before it is
// persisted. It's assumed that the remaining parser project configuration is
// already populated, but these values to initialize come from an external
// source (i.e. the patch or version it's based on).
func (pp *ParserProject) Init(id string, createdAt time.Time) {
	pp.Id = id
	pp.CreateTime = createdAt
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

func (pp *ParserProject) GetParameters() []patch.Parameter {
	res := []patch.Parameter{}
	for _, param := range pp.Parameters {
		res = append(res, param.Parameter)
	}
	return res
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
			Disable:         pt.Disable,
			AllowForGitTag:  pt.AllowForGitTag,
			GitTagOnly:      pt.GitTagOnly,
			Stepback:        pt.Stepback,
			MustHaveResults: pt.MustHaveResults,
			PS:              pt.Ps,
		}
		if strings.Contains(strings.TrimSpace(pt.Name), " ") {
			evalErrs = append(evalErrs, errors.Errorf("spaces are not allowed in task names ('%s')", pt.Name))
		}
		t.AllowedRequesters = pt.AllowedRequesters
		t.DependsOn, errs = evaluateDependsOn(tse.tagEval, tgse, vse, pt.DependsOn)
		evalErrs = append(evalErrs, errs...)
		tasks = append(tasks, t)
	}
	for _, ptg := range tgs {
		tg := TaskGroup{
			Name:                     ptg.Name,
			SetupGroup:               ptg.SetupGroup,
			SetupGroupCanFailTask:    ptg.SetupGroupCanFailTask,
			SetupGroupTimeoutSecs:    ptg.SetupGroupTimeoutSecs,
			TeardownGroup:            ptg.TeardownGroup,
			TeardownGroupTimeoutSecs: ptg.TeardownGroupTimeoutSecs,
			SetupTask:                ptg.SetupTask,
			SetupTaskCanFailTask:     ptg.SetupTaskCanFailTask,
			SetupTaskTimeoutSecs:     ptg.SetupTaskTimeoutSecs,
			TeardownTask:             ptg.TeardownTask,
			TeardownTaskCanFailTask:  ptg.TeardownTaskCanFailTask,
			TeardownTaskTimeoutSecs:  ptg.TeardownTaskTimeoutSecs,
			Tags:                     ptg.Tags,
			MaxHosts:                 ptg.MaxHosts,
			Timeout:                  ptg.Timeout,
			ShareProcs:               ptg.ShareProcs,
		}
		if tg.MaxHosts == -1 {
			tg.MaxHosts = len(ptg.Tasks)
		} else if tg.MaxHosts < 1 {
			tg.MaxHosts = 1
		}
		// expand, validate that tasks defined in a group are listed in the project tasks
		var taskNames []string
		for _, taskName := range ptg.Tasks {
			names, unmatched, err := tse.evalSelector(ParseSelector(taskName))
			if err != nil {
				evalErrs = append(evalErrs, err)
			}
			if len(unmatched) > 0 {
				evalErrs = append(evalErrs, errors.Errorf("task group '%s' has unmatched selector: '%s'", ptg.Name, strings.Join(unmatched, "', '")))
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
	var unmatchedSelectors []string
	var unmatchedCriteria []string
	var evalErrs, errs []error
	for _, pbv := range pbvs {
		bv := BuildVariant{
			DisplayName:        pbv.DisplayName,
			Name:               pbv.Name,
			Expansions:         pbv.Expansions,
			Modules:            pbv.Modules,
			Disable:            pbv.Disable,
			BatchTime:          pbv.BatchTime,
			CronBatchTime:      pbv.CronBatchTime,
			Activate:           pbv.Activate,
			Patchable:          pbv.Patchable,
			PatchOnly:          pbv.PatchOnly,
			AllowForGitTag:     pbv.AllowForGitTag,
			GitTagOnly:         pbv.GitTagOnly,
			Stepback:           pbv.Stepback,
			DeactivatePrevious: pbv.DeactivatePrevious,
			RunOn:              pbv.RunOn,
			Tags:               pbv.Tags,
			Paths:              pbv.Paths,
		}
		bv.AllowedRequesters = pbv.AllowedRequesters
		bv.Tasks, unmatchedSelectors, unmatchedCriteria, errs = evaluateBVTasks(tse, tgse, vse, pbv, tasks)
		if len(unmatchedSelectors) > 0 {
			bv.TranslationWarnings = append(bv.TranslationWarnings, fmt.Sprintf("buildvariant '%s' has unmatched selector: '%s'", pbv.Name, strings.Join(unmatchedSelectors, "', '")))
		}
		if len(unmatchedCriteria) > 0 {
			bv.TranslationWarnings = append(bv.TranslationWarnings, fmt.Sprintf("buildvariant '%s' has unmatched criteria: '%s'", pbv.Name, strings.Join(unmatchedCriteria, "', '")))
		}

		// evaluate any rules passed in during matrix construction
		for _, r := range pbv.MatrixRules {
			// remove_tasks removes all tasks with matching names
			if len(r.RemoveTasks) > 0 {
				prunedTasks := []BuildVariantTaskUnit{}
				toRemove := []string{}
				for _, t := range r.RemoveTasks {
					removed, _, err := tse.evalSelector(ParseSelector(t))
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
				added, _, _, errs = evaluateBVTasks(tse, tgse, vse, pbv, tasks)
				evalErrs = append(evalErrs, errs...)
				// check for conflicting duplicates
				for _, t := range added {
					if old, ok := existing[t.Name]; ok {
						if !reflect.DeepEqual(t, *old) {
							evalErrs = append(evalErrs, errors.Errorf(
								"conflicting definitions of added tasks '%s': %#v != %#v", t.Name, t, old))
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
				errs = append(errs, errors.Errorf("display task '%s' cannot have the same name as an execution task", dt.Name))
				continue
			}

			//resolve tags for display tasks
			tasks := []string{}
			for _, et := range dt.ExecutionTasks {
				results, unmatched, err := dtse.evalSelector(ParseSelector(et))
				if err != nil {
					errs = append(errs, err)
				}
				if len(unmatched) > 0 {
					errs = append(errs, errors.Errorf("display task '%s' contains unmatched criteria: '%s'", dt.Name, strings.Join(unmatched, "', '")))
				}
				tasks = append(tasks, results...)
			}

			for _, et := range tasks {
				if _, exists := bvTasks[et]; !exists {
					errs = append(errs, errors.Errorf("display task '%s' contains execution task '%s' which does not exist in build variant", dt.Name, et))
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
				errs = append(errs, errors.Errorf("execution task '%s' is listed in more than 1 display task", taskId))
				bv.DisplayTasks = nil
			}
		}

		evalErrs = append(evalErrs, errs...)
		bvs = append(bvs, bv)
	}
	return bvs, evalErrs
}

// evaluateBVTasks translates intermediate tasks listed under build variants
// into true BuildVariantTaskUnit types, evaluating any selectors referencing
// tasks, and further evaluating any selectors in the DependsOn field of those
// tasks.
// For task units that represent task groups, the resulting BuildVariantTaskUnit
// represents the task group itself, not the individual tasks in the task group.
// Returns the list of BuildVariantTaskUnits, the list of selectors that did not
// match anything, the list of criteria that did not match any tasks, and
// any errors encountered during evaluation.
func evaluateBVTasks(tse *taskSelectorEvaluator, tgse *tagSelectorEvaluator, vse *variantSelectorEvaluator,
	pbv parserBV, tasks []parserTask) ([]BuildVariantTaskUnit, []string, []string, []error) {
	var evalErrs, errs []error
	ts := []BuildVariantTaskUnit{}
	unmatchedSelectors := []string{}
	unmatchedCriteria := []string{}
	taskUnitsByName := map[string]BuildVariantTaskUnit{}
	tasksByName := map[string]parserTask{}
	for _, t := range tasks {
		tasksByName[t.Name] = t
	}
	for _, pbvt := range pbv.Tasks {
		// Evaluate each task against both the task and task group selectors
		// only error if both selectors error because each task should only be found
		// in one or the other.
		var names, unmatchedCriteriaFromTasks, unmatchedCriteriaFromTaskGroups, temp []string
		isGroup := false

		var taskSelectorErr, taskGroupSelectorErr error
		if tse != nil {
			temp, unmatchedCriteriaFromTasks, taskSelectorErr = tse.evalSelector(ParseSelector(pbvt.Name))
			names = append(names, temp...)
		}
		if tgse != nil {
			temp, unmatchedCriteriaFromTaskGroups, taskGroupSelectorErr = tgse.evalSelector(ParseSelector(pbvt.Name))
			if len(temp) > 0 {
				names = append(names, temp...)
				isGroup = true
			}
		}
		if taskSelectorErr != nil && taskGroupSelectorErr != nil {
			evalErrs = append(evalErrs, taskSelectorErr, taskGroupSelectorErr)
			unmatchedSelectors = append(unmatchedSelectors, pbvt.Name)
			continue
		}

		initialUnmatchedCriteriaLen := len(unmatchedCriteriaFromTasks)
		for _, unmatchedTask := range unmatchedCriteriaFromTasks {
			if utility.StringSliceContains(unmatchedCriteriaFromTaskGroups, unmatchedTask) {
				unmatchedCriteria = append(unmatchedCriteria, unmatchedTask)
			}
		}
		if len(names) == 0 {
			unmatchedSelectors = append(unmatchedSelectors, pbvt.Name)
			continue
		}
		// If any were unmatched, do not add any matched tasks to the list.
		if len(unmatchedCriteriaFromTasks) > initialUnmatchedCriteriaLen {
			continue
		}
		// create new task definitions--duplicates must have the same status requirements
		for _, name := range names {
			parserTask := tasksByName[name]
			// create a new task by copying the task that selected it,
			// so we can preserve the "Variant" and "Status" field.
			t := getParserBuildVariantTaskUnit(name, parserTask, pbvt, pbv)

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
			// IsGroup indicates here that this build variant task unit is a
			// task group.
			t.IsGroup = isGroup

			// add the new task if it doesn't already exists (we must avoid conflicting status fields)
			if old, ok := taskUnitsByName[t.Name]; !ok {
				ts = append(ts, t)
				taskUnitsByName[t.Name] = t
			} else {
				// it's already in the new list, so we check to make sure the status definitions match.
				if !reflect.DeepEqual(t, old) {
					evalErrs = append(evalErrs, errors.Errorf(
						"conflicting definitions of task '%s' listed under build variant '%s': %#v != %#v", name, pbv.Name, t, old))
					continue
				}
			}
		}
	}
	// No tasks selected should result in an error if there are any tasks defined in the build variant.
	if len(ts) == 0 && len(pbv.Tasks) > 0 {
		evalErrs = append(evalErrs, errors.Errorf("task selectors for build variant '%s' did not match any tasks", pbv.Name))

	}
	return ts, unmatchedSelectors, unmatchedCriteria, evalErrs
}

// getParserBuildVariantTaskUnit combines the parser project's build variant
// task definition with its task definition and build variant definition.
// DependsOn is handled separately. When there are conflicting settings defined
// at different levels, the priority of settings are (from highest to lowest):
// * Task settings within a build variant's list of tasks
// * Task settings within a task group's list of tasks
// * Project task's settings
// * Build variant's settings
func getParserBuildVariantTaskUnit(name string, pt parserTask, bvt parserBVTaskUnit, bv parserBV) BuildVariantTaskUnit {
	res := BuildVariantTaskUnit{
		Name:           name,
		Variant:        bv.Name,
		Patchable:      bvt.Patchable,
		PatchOnly:      bvt.PatchOnly,
		Disable:        bvt.Disable,
		AllowForGitTag: bvt.AllowForGitTag,
		GitTagOnly:     bvt.GitTagOnly,
		Priority:       bvt.Priority,
		Stepback:       bvt.Stepback,
		RunOn:          bvt.RunOn,
		CronBatchTime:  bvt.CronBatchTime,
		BatchTime:      bvt.BatchTime,
		Activate:       bvt.Activate,
		PS:             bvt.PS,
		CreateCheckRun: bvt.CreateCheckRun,
	}
	res.AllowedRequesters = bvt.AllowedRequesters
	if res.Priority == 0 {
		res.Priority = pt.Priority
	}
	if res.Patchable == nil {
		res.Patchable = pt.Patchable
	}
	if res.PatchOnly == nil {
		res.PatchOnly = pt.PatchOnly
	}
	if res.Disable == nil {
		res.Disable = pt.Disable
	}
	if res.AllowForGitTag == nil {
		res.AllowForGitTag = pt.AllowForGitTag
	}
	if res.GitTagOnly == nil {
		res.GitTagOnly = pt.GitTagOnly
	}
	if len(res.AllowedRequesters) == 0 {
		res.AllowedRequesters = pt.AllowedRequesters
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
	if res.PS == nil {
		res.PS = pt.Ps
	}

	// Build variant level settings are lower priority than project task level
	// settings.
	if res.Patchable == nil {
		res.Patchable = bv.Patchable
	}
	if res.PatchOnly == nil {
		res.PatchOnly = bv.PatchOnly
	}
	if res.AllowForGitTag == nil {
		res.AllowForGitTag = bv.AllowForGitTag
	}
	if res.GitTagOnly == nil {
		res.GitTagOnly = bv.GitTagOnly
	}
	if len(res.AllowedRequesters) == 0 {
		res.AllowedRequesters = bv.AllowedRequesters
	}

	if res.Disable == nil {
		res.Disable = bv.Disable
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
		var names, unmatched []string

		if d.TaskSelector.Name == AllDependencies {
			// * is a special case for dependencies, so don't eval it
			names = []string{AllDependencies}
		} else {
			var temp []string
			var err1, err2 error
			if tse != nil {
				temp, unmatched, err1 = tse.evalSelector(ParseSelector(d.TaskSelector.Name))
				names = append(names, temp...)
				if err1 == nil && len(unmatched) > 0 {
					err1 = errors.Errorf("unmatched criteria: '%s' from selector '%s'", strings.Join(unmatched, ", "), d.TaskSelector.Name)
				}
			}
			if tgse != nil {
				temp, unmatched, err2 = tgse.evalSelector(ParseSelector(d.TaskSelector.Name))
				names = append(names, temp...)
				if err2 == nil && len(unmatched) > 0 {
					err2 = errors.Errorf("unmatched criteria: '%s' from selector '%s'", strings.Join(unmatched, ", "), d.TaskSelector.Name)
				}
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
					Name:               name,
					Variant:            variant,
					Status:             d.Status,
					PatchOptional:      d.PatchOptional,
					OmitGeneratedTasks: d.OmitGeneratedTasks,
				}
				// add the new dep if it doesn't already exist (we must avoid conflicting status fields)
				if oldDep, ok := newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}]; !ok {
					newDeps = append(newDeps, newDep)
					newDepsByNameAndVariant[TVPair{newDep.Variant, newDep.Name}] = newDep
				} else {
					// it's already in the new list, so we check to make sure the status definitions match.
					if !reflect.DeepEqual(newDep, oldDep) {
						evalErrs = append(evalErrs, errors.Errorf(
							"conflicting definitions of dependency '%s': %#v != %#v", name, newDep, oldDep))
						continue
					}
				}
			}
		}
	}
	return newDeps, evalErrs
}

// evaluateRequesters translates user requesters into internal requesters.
func evaluateRequesters(userRequesters []evergreen.UserRequester) []string {
	requesters := make([]string, 0, len(userRequesters))
	for _, userRequester := range userRequesters {
		requester := evergreen.UserRequesterToInternalRequester(userRequester)
		if !utility.StringSliceContains(evergreen.AllRequesterTypes, requester) {
			continue
		}
		requesters = append(requesters, requester)
	}
	return requesters
}

func preGeneratedParserProjectId(originalId string) string {
	return fmt.Sprintf("%s_%s", "pre_generation", originalId)
}

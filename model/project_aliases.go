package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	idKey          = bsonutil.MustHaveTag(ProjectAlias{}, "ID")
	projectIDKey   = bsonutil.MustHaveTag(ProjectAlias{}, "ProjectID")
	aliasKey       = bsonutil.MustHaveTag(ProjectAlias{}, "Alias")
	gitTagKey      = bsonutil.MustHaveTag(ProjectAlias{}, "GitTag")
	remotePathKey  = bsonutil.MustHaveTag(ProjectAlias{}, "RemotePath")
	variantKey     = bsonutil.MustHaveTag(ProjectAlias{}, "Variant")
	descriptionKey = bsonutil.MustHaveTag(ProjectAlias{}, "Description")
	taskKey        = bsonutil.MustHaveTag(ProjectAlias{}, "Task")
	parametersKey  = bsonutil.MustHaveTag(ProjectAlias{}, "Parameters")
	variantTagsKey = bsonutil.MustHaveTag(ProjectAlias{}, "VariantTags")
	taskTagsKey    = bsonutil.MustHaveTag(ProjectAlias{}, "TaskTags")
)

const (
	ProjectAliasCollection = "project_aliases"
)

// ProjectAlias defines a single alias mapping an alias name to two regexes which
// define the variants and tasks for the alias. Users can use these aliases for
// operations within the system.
//
// For example, a user can specify that alias with the CLI tool so that a project
// admin can define a set of default builders for patch builds. Pull request
// testing uses a special alias, "__github" to determine the default
// variants and tasks to run in a patch build.
//
// An alias can be specified multiple times. The resulting variant/task
// combinations are the union of the aliases. For example, a user might set the
// following:
//
// ALIAS                  VARIANTS          TASKS
// __github               .*linux.*         .*test.*
// __github               ^ubuntu1604.*$    ^compile.*$
//
// This will cause a GitHub pull request to create and finalize a patch which runs
// all tasks containing the string "test" on all variants containing the string
// "linux"; and to run all tasks beginning with the string "compile" to run on all
// variants beginning with the string "ubuntu1604".

// For regular patch aliases, the Alias field is required to be a custom string defined by the user.
// For all other special alias types (commit queue, github PR, etc) the Alias field must match its associated
// constant in globals.go, i.e. evergreen.GithubPRAlias. For aliases defined within a project's config YAML
// the Alias field for non-patch aliases is not-required since it will be inferred and assigned at runtime.

// Git tags use a special alias "__git_tag" and create a new version for the matching
// variants/tasks, assuming the tag matches the defined git_tag regex.
// In this way, users can define different behavior for different kind of tags.
type ProjectAlias struct {
	ID          mgobson.ObjectId  `bson:"_id,omitempty" json:"_id" yaml:"id"`
	ProjectID   string            `bson:"project_id" json:"project_id" yaml:"project_id"`
	Alias       string            `bson:"alias" json:"alias" yaml:"alias"`
	Variant     string            `bson:"variant,omitempty" json:"variant" yaml:"variant"`
	Description string            `bson:"description" json:"description" yaml:"description"`
	GitTag      string            `bson:"git_tag" json:"git_tag" yaml:"git_tag"`
	RemotePath  string            `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	VariantTags []string          `bson:"variant_tags,omitempty" json:"variant_tags" yaml:"variant_tags"`
	Task        string            `bson:"task,omitempty" json:"task" yaml:"task"`
	TaskTags    []string          `bson:"tags,omitempty" json:"tags" yaml:"task_tags"`
	Parameters  []patch.Parameter `bson:"parameters,omitempty" json:"parameters" yaml:"parameters"`

	// Source is not stored; indicates where the alias is stored for the project.
	Source string `bson:"-" json:"-" yaml:"-"`
}

const (
	AliasSourceProject = "project"
	AliasSourceConfig  = "config"
	AliasSourceRepo    = "repo"
)

type ProjectAliases []ProjectAlias

// FindAliasesForProjectFromDb fetches all aliases for a given project without merging with aliases from the parser project
func FindAliasesForProjectFromDb(projectID string) ([]ProjectAlias, error) {
	var out []ProjectAlias
	q := db.Query(bson.M{
		projectIDKey: projectID,
	})
	err := db.FindAllQ(ProjectAliasCollection, q, &out)
	if err != nil {
		return nil, errors.Wrap(err, "finding project aliases")
	}
	return out, nil
}

// GetAliasesMergedWithProjectConfig returns a merged list of project aliases that includes the merged result of aliases defined
// on the project ref and aliases defined in the project YAML.  Aliases defined on the project ref will take precedence over the
// project YAML in the case that both are defined.
func GetAliasesMergedWithProjectConfig(projectID string, dbAliases []ProjectAlias) ([]ProjectAlias, error) {
	projectConfig, err := FindLastKnownGoodProjectConfig(projectID)
	if err != nil {
		return nil, errors.Wrap(err, "finding project config")
	}
	return mergeProjectConfigAndAliases(projectConfig, dbAliases), nil
}

func mergeProjectConfigAndAliases(projectConfig *ProjectConfig, dbAliases []ProjectAlias) []ProjectAlias {
	if projectConfig == nil {
		return dbAliases
	}
	dbAliasMap := aliasesToMap(dbAliases)

	patchAliases := []ProjectAlias{}
	for alias, aliases := range dbAliasMap {
		if IsPatchAlias(alias) {
			patchAliases = append(patchAliases, aliases...)
		}
	}
	mergedAliases := []ProjectAlias{}
	if projectConfig != nil {
		if len(dbAliasMap[evergreen.CommitQueueAlias]) == 0 {
			dbAliasMap[evergreen.CommitQueueAlias] = projectConfig.CommitQueueAliases
		}
		if len(dbAliasMap[evergreen.GithubPRAlias]) == 0 {
			dbAliasMap[evergreen.GithubPRAlias] = projectConfig.GitHubPRAliases
		}
		if len(dbAliasMap[evergreen.GithubChecksAlias]) == 0 {
			dbAliasMap[evergreen.GithubChecksAlias] = projectConfig.GitHubChecksAliases
		}
		if len(dbAliasMap[evergreen.GitTagAlias]) == 0 {
			dbAliasMap[evergreen.GitTagAlias] = projectConfig.GitTagAliases
		}
		if len(patchAliases) == 0 {
			patchAliases = projectConfig.PatchAliases
		}
	}
	mergedAliases = append(mergedAliases, dbAliasMap[evergreen.CommitQueueAlias]...)
	mergedAliases = append(mergedAliases, dbAliasMap[evergreen.GithubChecksAlias]...)
	mergedAliases = append(mergedAliases, dbAliasMap[evergreen.GitTagAlias]...)
	mergedAliases = append(mergedAliases, dbAliasMap[evergreen.GithubPRAlias]...)
	mergedAliases = append(mergedAliases, patchAliases...)
	return mergedAliases
}

// FindAliasesForRepo fetches all aliases for a given project
func FindAliasesForRepo(repoId string) ([]ProjectAlias, error) {
	out := []ProjectAlias{}
	q := db.Query(bson.M{
		projectIDKey: repoId,
	})
	err := db.FindAllQ(ProjectAliasCollection, q, &out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// findMatchingAliasForRepo finds all aliases with a given name for a repo.
// Typically FindAliasInProjectRepoOrConfig should be used.
func findMatchingAliasForRepo(repoID, alias string) ([]ProjectAlias, error) {
	var out []ProjectAlias
	q := db.Query(bson.M{
		projectIDKey: repoID,
		aliasKey:     alias,
	})
	err := db.FindAllQ(ProjectAliasCollection, q, &out)
	if err != nil {
		return nil, errors.Wrap(err, "finding project aliases for repo")
	}
	return out, nil
}

// findMatchingAliasForProjectRef finds all aliases with a given name for a project.
// Typically FindAliasInProjectRepoOrConfig should be used.
func findMatchingAliasForProjectRef(projectID, alias string) ([]ProjectAlias, error) {
	var out []ProjectAlias
	q := db.Query(bson.M{
		projectIDKey: projectID,
		aliasKey:     alias,
	})
	err := db.FindAllQ(ProjectAliasCollection, q, &out)
	if err != nil {
		return nil, errors.Wrap(err, "finding project aliases")
	}

	return out, nil
}

// getMatchingAliasesForProjectConfig finds any aliases matching the alias input in the project config.
func getMatchingAliasesForProjectConfig(projectID, versionID, alias string) ([]ProjectAlias, error) {
	projectConfig, err := FindProjectConfigForProjectOrVersion(projectID, versionID)
	if err != nil {
		return nil, errors.Wrap(err, "finding project config")
	}
	if projectConfig == nil {
		return nil, nil
	}

	return findAliasFromProjectConfig(projectConfig, alias)
}

func findAliasFromProjectConfig(projectConfig *ProjectConfig, alias string) ([]ProjectAlias, error) {
	projectConfig.SetInternalAliases()
	projectConfigAliases := aliasesToMap(projectConfig.AllAliases())
	return projectConfigAliases[alias], nil
}

func aliasesToMap(aliases []ProjectAlias) map[string][]ProjectAlias {
	output := make(map[string][]ProjectAlias)
	for _, alias := range aliases {
		output[alias.Alias] = append(output[alias.Alias], alias)
	}
	return output
}

func aliasesFromMap(input map[string]ProjectAliases) []ProjectAlias {
	output := []ProjectAlias{}
	for _, aliases := range input {
		output = append(output, aliases...)
	}
	return output
}

// FindAliasInProjectRepoOrConfig finds all aliases with a given name for a project.
// If the project has no aliases, the repo is checked for aliases.
func FindAliasInProjectRepoOrConfig(projectID, alias string) ([]ProjectAlias, error) {
	aliases, err := findAliasInProjectOrRepoFromDb(projectID, alias)
	if err != nil {
		return nil, errors.Wrap(err, "checking for existing aliases")
	}
	// If nothing is defined in the DB, check the project config.
	if len(aliases) > 0 {
		return aliases, nil
	}
	return getMatchingAliasesForProjectConfig(projectID, "", alias)
}

// patchAliasKey is used internally to group patch aliases together.
const patchAliasKey = "patch_alias"

// ConstructMergedAliasesByPrecedence will construct a merged list of aliases based on what aliases
// are found at the project level, repo level, and project config level.
func ConstructMergedAliasesByPrecedence(projectRef *ProjectRef, projectConfig *ProjectConfig, repoId string) ([]ProjectAlias, error) {
	var projectAliases []ProjectAlias
	var err error
	if projectRef != nil {
		projectAliases, err = FindAliasesForProjectFromDb(projectRef.Id)
		if err != nil {
			return nil, errors.Wrap(err, "finding project aliases")
		}
	}
	aliasesToReturn := map[string]ProjectAliases{}
	for _, alias := range projectAliases {
		aliasName := alias.Alias
		if IsPatchAlias(aliasName) {
			aliasName = patchAliasKey
		}
		alias.Source = AliasSourceProject
		aliasesToReturn[aliasName] = append(aliasesToReturn[aliasName], alias)
	}
	// If all aliases are covered in the project, so there's no reason to look at other sources
	uncoveredAliases := uncoveredAliasTypes(aliasesToReturn)
	if len(uncoveredAliases) > 0 && repoId != "" {
		// Get repo aliases and merge with project aliases
		repoAliases, err := FindAliasesForRepo(repoId)
		if err != nil {
			return nil, errors.Wrap(err, "finding repo aliases")
		}
		for _, alias := range repoAliases {
			aliasName := alias.Alias
			if IsPatchAlias(aliasName) {
				aliasName = patchAliasKey
			}
			if !utility.StringSliceContains(uncoveredAliases, aliasName) { // Only add alias if there aren't project aliases
				continue
			}
			alias.Source = AliasSourceRepo
			aliasesToReturn[aliasName] = append(aliasesToReturn[aliasName], alias)
		}
		// If all aliases covered in project/repo, then no reason to look at config
		uncoveredAliases = uncoveredAliasTypes(aliasesToReturn)
	}
	res := aliasesFromMap(aliasesToReturn)
	if len(uncoveredAliases) > 0 && projectRef != nil && projectRef.IsVersionControlEnabled() {
		mergedAliases := mergeProjectConfigAndAliases(projectConfig, res)
		// If we've added any new aliases, ensure they're given the config source
		if len(mergedAliases) > len(res) {
			for i, a := range mergedAliases {
				if a.Source == "" {
					mergedAliases[i].Source = AliasSourceConfig
				}
			}
		}
		return mergedAliases, nil
	}
	return res, nil
}

// uncoveredAliasTypes returns a list of alias types that aren't populated keys in the given map.
func uncoveredAliasTypes(aliases map[string]ProjectAliases) []string {
	res := []string{}
	aliasesToCheck := append(evergreen.InternalAliases, patchAliasKey)
	for _, name := range aliasesToCheck {
		if len(aliases[name]) == 0 {
			res = append(res, name)
		}
	}
	return res
}

// FindAliasInProjectRepoOrProjectConfig finds all aliases with a given name for a project.
// If the project has no aliases, the patched config string is checked for the alias as well.
func FindAliasInProjectRepoOrProjectConfig(projectID, alias string, projectConfig *ProjectConfig) ([]ProjectAlias, error) {
	aliases, err := findAliasInProjectOrRepoFromDb(projectID, alias)
	if err != nil {
		return nil, errors.Wrap(err, "checking for existing aliases")
	}
	if len(aliases) > 0 || projectConfig == nil {
		return aliases, nil
	}

	return findAliasFromProjectConfig(projectConfig, alias)
}

// findAliasInProjectOrRepoFromDb finds all aliases with a given name for a project without merging with parser project.
// If the project has no aliases, the repo is checked for aliases.
func findAliasInProjectOrRepoFromDb(projectID, alias string) ([]ProjectAlias, error) {
	aliases, err := findMatchingAliasForProjectRef(projectID, alias)
	if err != nil {
		return aliases, errors.Wrapf(err, "finding aliases for project ref '%s'", projectID)
	}
	if len(aliases) > 0 {
		return aliases, nil
	}
	return tryGetRepoAliases(projectID, alias, aliases)
}

func tryGetRepoAliases(projectID string, alias string, aliases []ProjectAlias) ([]ProjectAlias, error) {
	project, err := FindBranchProjectRef(projectID)
	if err != nil {
		return aliases, errors.Wrapf(err, "finding project '%s'", projectID)
	}
	if project == nil {
		return aliases, errors.Errorf("project '%s' does not exist", projectID)
	}
	if !project.UseRepoSettings() {
		return aliases, nil
	}

	aliases, err = findMatchingAliasForRepo(project.RepoRefId, alias)
	if err != nil {
		return aliases, errors.Wrapf(err, "finding aliases for repo '%s'", project.RepoRefId)
	}
	return aliases, nil
}

// HasMatchingGitTagAliasAndRemotePath returns matching git tag aliases that match the given git tag
func HasMatchingGitTagAliasAndRemotePath(projectId, tag string) (bool, string, error) {
	aliases, err := FindMatchingGitTagAliasesInProject(projectId, tag)
	if err != nil {
		return false, "", err
	}

	if len(aliases) == 1 && aliases[0].RemotePath != "" {
		return true, aliases[0].RemotePath, nil
	}
	return len(aliases) > 0, "", nil
}

// CopyProjectAliases finds the aliases for a given project and inserts them for the new project.
func CopyProjectAliases(oldProjectId, newProjectId string) error {
	aliases, err := FindAliasesForProjectFromDb(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "finding aliases for project '%s'", oldProjectId)
	}
	if aliases != nil {
		if err = UpsertAliasesForProject(aliases, newProjectId); err != nil {
			return errors.Wrapf(err, "inserting aliases for project '%s'", newProjectId)
		}
	}
	return nil
}

func FindMatchingGitTagAliasesInProject(projectID, tag string) ([]ProjectAlias, error) {
	aliases, err := FindAliasInProjectRepoOrConfig(projectID, evergreen.GitTagAlias)
	if err != nil {
		return nil, err
	}
	matchingAliases, err := aliasesMatchingGitTag(aliases, tag)
	if err != nil {
		return nil, err
	}
	for _, alias := range matchingAliases {
		if alias.RemotePath != "" && len(matchingAliases) > 1 {
			return matchingAliases, errors.Errorf("git tag '%s' matches multiple aliases but a remote path is defined", tag)
		}
	}
	return matchingAliases, nil
}

// IsValidId returns whether the supplied Id is a valid patch doc id (BSON ObjectId).
func IsValidId(id string) bool {
	return mgobson.IsObjectIdHex(id)
}

// NewId constructs a valid patch Id from the given hex string.
func NewId(id string) mgobson.ObjectId { return mgobson.ObjectIdHex(id) }

func (p *ProjectAlias) Upsert() error {
	if len(p.ProjectID) == 0 {
		return errors.New("empty project ID")
	}
	if p.ID.Hex() == "" {
		p.ID = mgobson.NewObjectId()
	}
	update := bson.M{
		aliasKey:       p.Alias,
		gitTagKey:      p.GitTag,
		remotePathKey:  p.RemotePath,
		projectIDKey:   p.ProjectID,
		variantKey:     p.Variant,
		descriptionKey: p.Description,
		variantTagsKey: p.VariantTags,
		taskTagsKey:    p.TaskTags,
		taskKey:        p.Task,
		parametersKey:  p.Parameters,
	}

	_, err := db.Upsert(ProjectAliasCollection, bson.M{
		idKey: p.ID,
	}, bson.M{"$set": update})
	if err != nil {
		return errors.Wrapf(err, "inserting project alias '%s'", p.ID)
	}
	return nil
}

func UpsertAliasesForProject(aliases []ProjectAlias, projectId string) error {
	catcher := grip.NewBasicCatcher()
	for i := range aliases {
		if aliases[i].ProjectID != projectId { // new project, so we need a new document (new ID)
			aliases[i].ProjectID = projectId
			aliases[i].ID = ""
		}
		catcher.Add(aliases[i].Upsert())
	}
	grip.Debug(message.WrapError(catcher.Resolve(), message.Fields{
		"message":    "problem getting aliases",
		"project_id": projectId,
	}))
	return catcher.Resolve()
}

// RemoveProjectAlias removes a project alias with the given document ID from the
// database.
func RemoveProjectAlias(id string) error {
	if id == "" {
		return errors.New("can't remove project alias with empty id")
	}
	err := db.Remove(ProjectAliasCollection, bson.M{idKey: mgobson.ObjectIdHex(id)})
	if err != nil {
		return errors.Wrapf(err, "removing project alias '%s'", id)
	}
	return nil
}

func IsPatchAlias(alias string) bool {
	return !utility.StringSliceContains(evergreen.InternalAliases, alias)
}

// HasMatchingGitTag determines whether or not the given git tag name matches
// any of the project aliases' git tag regexp.
func (a ProjectAliases) HasMatchingGitTag(tag string) (bool, error) {
	matchingAliases, err := aliasesMatchingGitTag(a, tag)
	if err != nil {
		return false, err
	}
	return len(matchingAliases) > 0, nil
}

func aliasesMatchingGitTag(a ProjectAliases, tag string) (ProjectAliases, error) {
	res := []ProjectAlias{}
	for _, alias := range a {
		gitTagRegex, err := alias.getGitTagRegex()
		if err != nil {
			return nil, err
		}
		if isValidRegexOrTag(tag, nil, nil, gitTagRegex) {
			res = append(res, alias)
		}
	}
	return res, nil
}

// AliasesMatchingVariant returns the filtered set of project aliases for which
// the alias' variant regexp matches the variant's name, or the alias' variant
// tags match the variant's tags.
func (a ProjectAliases) AliasesMatchingVariant(variant string, variantTags []string) (ProjectAliases, error) {
	res := []ProjectAlias{}
	for _, alias := range a {
		hasMatch, err := alias.HasMatchingVariant(variant, variantTags)
		if err != nil {
			return nil, err
		}
		if hasMatch {
			res = append(res, alias)
		}
	}
	return res, nil
}

// HasMatchingVariant returns whether or not one the alias variant regexp
// matches the variant's name, or the alias' variant tags match the variant's
// tags. Note that this does not check for matching tasks.
func (a ProjectAlias) HasMatchingVariant(variant string, variantTags []string) (bool, error) {
	variantRegex, err := a.getVariantRegex()
	if err != nil {
		return false, err
	}
	return isValidRegexOrTag(variant, variantTags, a.VariantTags, variantRegex), nil
}

func (a *ProjectAlias) getVariantRegex() (*regexp.Regexp, error) {
	if a.Variant == "" {
		return nil, nil
	}
	variantRegex, err := regexp.Compile(a.Variant)
	if err != nil {
		return nil, errors.Wrapf(err, "compiling variant regex '%s'", a.Variant)
	}
	return variantRegex, nil
}

func (a *ProjectAlias) getTaskRegex() (*regexp.Regexp, error) {
	if a.Task == "" {
		return nil, nil
	}
	taskRegex, err := regexp.Compile(a.Task)
	if err != nil {
		return nil, errors.Wrapf(err, "compiling task regex '%s'", a.Task)
	}
	return taskRegex, nil
}

func (a *ProjectAlias) getGitTagRegex() (*regexp.Regexp, error) {
	if a.GitTag == "" {
		return nil, nil
	}
	gitTagRegex, err := regexp.Compile(a.GitTag)
	if err != nil {
		return nil, errors.Wrapf(err, "compiling git tag regex '%s'", a.GitTag)
	}
	return gitTagRegex, nil
}

// HasMatchingTask returns whether or not one the alias task regexp matches the
// variant's name, or the alias' task tags match the variant's tags. Note that
// this does not check for matching variant.
func (a ProjectAliases) HasMatchingTask(taskName string, taskTags []string) (bool, error) {
	for _, alias := range a {
		hasMatch, err := alias.HasMatchingTask(taskName, taskTags)
		if err != nil {
			return false, err
		}
		if hasMatch {
			return true, nil
		}
	}
	return false, nil
}

func (a ProjectAlias) HasMatchingTask(taskName string, taskTags []string) (bool, error) {
	taskRegex, err := a.getTaskRegex()
	if err != nil {
		return false, err
	}

	return isValidRegexOrTag(taskName, taskTags, a.TaskTags, taskRegex), nil
}

// isValidRegexOrTag returns true if the item/tag matches the alias tag/regex.
func isValidRegexOrTag(curItem string, curTags, aliasTags []string, aliasRegex *regexp.Regexp) bool {
	if aliasRegex != nil && aliasRegex.MatchString(curItem) {
		return true
	}
	for _, tag := range aliasTags {
		// For this tag selector to match the current item's tags, it has to
		// satisfy all the space-separated criteria defined in the selector.
		// Note that this is functionally equivalent to project tag selector
		// syntax, except that alias tags are not prefixed by a period.

		criteria := strings.Fields(tag)
		if len(criteria) == 0 {
			continue
		}

		allCriteriaSatisfied := true
		for _, criterion := range criteria {
			if len(criterion) == 0 {
				continue
			}

			tagName := criterion
			var negated bool
			if criterion[0] == '!' {
				negated = true
				tagName = criterion[1:]
			}
			if negated && utility.StringSliceContains(curTags, tagName) {
				// Negation - tag matches the item's tags.
				allCriteriaSatisfied = false
				break
			}
			if !negated && !utility.StringSliceContains(curTags, tagName) {
				// Intersection - tag does not match the item's tags.
				allCriteriaSatisfied = false
				break
			}
		}

		if allCriteriaSatisfied {
			return true
		}
	}

	return false
}

func ValidateProjectAliases(aliases []ProjectAlias, aliasType string) []string {
	errs := []string{}
	for i, pd := range aliases {
		if strings.TrimSpace(pd.Alias) == "" {
			errs = append(errs, fmt.Sprintf("%s: alias name #%d can't be empty string", aliasType, i+1))
		}
		if pd.Alias == evergreen.GitTagAlias {
			errs = append(errs, validateGitTagAlias(pd, aliasType, i+1)...)
			continue
		}
		if strings.TrimSpace(pd.GitTag) != "" || strings.TrimSpace(pd.RemotePath) != "" {
			errs = append(errs, fmt.Sprintf("%s: cannot define git tag or remote path on line #%d", aliasType, i+1))
		}
		errs = append(errs, validateAliasPatchDefinition(pd, aliasType, i+1)...)
	}

	return errs
}

func validateAliasPatchDefinition(pd ProjectAlias, aliasType string, lineNum int) []string {
	errs := []string{}
	if (strings.TrimSpace(pd.Variant) == "") == (len(pd.VariantTags) == 0) {
		errs = append(errs, fmt.Sprintf("%s: must specify exactly one of variant regex or variant tags on line #%d", aliasType, lineNum))
	}
	if (strings.TrimSpace(pd.Task) == "") == (len(pd.TaskTags) == 0) {
		errs = append(errs, fmt.Sprintf("%s: must specify exactly one of task regex or task tags on line #%d", aliasType, lineNum))
	}

	if _, err := pd.getVariantRegex(); err != nil {
		errs = append(errs, fmt.Sprintf("%s: variant regex #%d is invalid", aliasType, lineNum))
	}
	if _, err := pd.getTaskRegex(); err != nil {
		errs = append(errs, fmt.Sprintf("%s: task regex #%d is invalid", aliasType, lineNum))
	}
	return errs
}

func validateGitTagAlias(pd ProjectAlias, aliasType string, lineNum int) []string {
	errs := []string{}
	if strings.TrimSpace(pd.GitTag) == "" {
		errs = append(errs, fmt.Sprintf("%s: must define valid git tag regex on line #%d", aliasType, lineNum))
	}
	if _, err := pd.getGitTagRegex(); err != nil {
		errs = append(errs, fmt.Sprintf("%s: git tag regex #%d is invalid", aliasType, lineNum))
	}
	// if path is defined then no patch definition can be given
	if strings.TrimSpace(pd.RemotePath) != "" && populatedPatchDefinition(pd) {
		errs = append(errs, fmt.Sprintf("%s: cannot define remote path and task/variant constraints on line #%d", aliasType, lineNum))
	}
	if strings.TrimSpace(pd.RemotePath) == "" {
		errs = append(errs, validateAliasPatchDefinition(pd, aliasType, lineNum)...)
	}
	return errs
}

func populatedPatchDefinition(pd ProjectAlias) bool {
	return strings.TrimSpace(pd.Variant) != "" || strings.TrimSpace(pd.Task) != "" ||
		len(pd.VariantTags) != 0 || len(pd.Task) != 0
}

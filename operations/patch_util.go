package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Above this size, the user must explicitly use --large to submit the patch (or confirm)
const largePatchThreshold = 1024 * 1024 * 16

// Above this number of tasks to finalize, the user must confirm their intention via prompt
const largeNumFinalizedTasksThreshold = 1000

// This is the template used to render a patch's summary in a human-readable output format.
var patchDisplayTemplate = template.Must(template.New("patch").Parse(`
         ID : {{.Patch.Id.Hex}}
    Project : {{.ProjectIdentifier}}
    Created : {{.Patch.CreateTime}}
Description : {{if .Patch.Description}}{{.Patch.Description}}{{else}}<none>{{end}}
      Build : {{.Link}}
     Status : {{.Patch.Status}}
{{if .ShowFinalized}}      Finalized : {{if .Patch.Activated}}Yes{{else}}No{{end}}{{end}}
{{if .ShowSummary}}
    Summary :
{{range .Patch.Patches}}{{if not (eq .ModuleName "") }}Module:{{.ModuleName}}{{end}}
Base Commit : {{.Githash}}
    {{range .PatchSet.Summary}}+{{.Additions}} -{{.Deletions}} {{.Name}}
    {{end}}
{{end}}
{{end}}
`))

type localDiff struct {
	fullPatch    string
	patchSummary string
	log          string
	base         string
	gitMetadata  patch.GitMetadata
}

type patchParams struct {
	Project string
	Path    string
	Alias   string
	// isUsingLocalAlias indicates that the user-specified alias matches a local
	// alias.
	isUsingLocalAlias   bool
	Variants            []string
	Tasks               []string
	RegexVariants       []string
	RegexTasks          []string
	SyncBuildVariants   []string
	SyncTasks           []string
	SyncStatuses        []string
	SyncTimeout         time.Duration
	Description         string
	SkipConfirm         bool
	Finalize            bool
	Browse              bool
	Large               bool
	ShowSummary         bool
	AutoDescription     bool
	Uncommitted         bool
	PreserveCommits     bool
	Ref                 string
	BackportOf          patch.BackportInfo
	TriggerAliases      []string
	Parameters          []patch.Parameter
	RepeatDefinition    bool
	RepeatFailed        bool
	RepeatPatchId       string
	GithubAuthor        string
	PatchAuthor         string
	IncludeModules      bool
	LocalModuleIncludes []patch.LocalModuleInclude
}

type patchSubmission struct {
	projectName         string
	patchData           string
	description         string
	base                string
	alias               string
	path                string
	variants            []string
	tasks               []string
	regexVariants       []string
	regexTasks          []string
	syncBuildVariants   []string
	syncTasks           []string
	syncStatuses        []string
	syncTimeout         time.Duration
	finalize            bool
	parameters          []patch.Parameter
	triggerAliases      []string
	backportOf          patch.BackportInfo
	gitMetadata         patch.GitMetadata
	repeatDefinition    bool
	repeatFailed        bool
	repeatPatchId       string
	githubAuthor        string
	patchAuthor         string
	localModuleIncludes []patch.LocalModuleInclude
}

func (p *patchParams) createPatch(ac *legacyClient, diffData *localDiff) (*patch.Patch, error) {
	patchSub := patchSubmission{
		projectName:         p.Project,
		patchData:           diffData.fullPatch,
		description:         p.Description,
		base:                diffData.base,
		variants:            p.Variants,
		tasks:               p.Tasks,
		regexVariants:       p.RegexVariants,
		regexTasks:          p.RegexTasks,
		alias:               p.Alias,
		syncBuildVariants:   p.SyncBuildVariants,
		syncTasks:           p.SyncTasks,
		syncStatuses:        p.SyncStatuses,
		syncTimeout:         p.SyncTimeout,
		finalize:            p.Finalize,
		backportOf:          p.BackportOf,
		parameters:          p.Parameters,
		triggerAliases:      p.TriggerAliases,
		gitMetadata:         diffData.gitMetadata,
		repeatDefinition:    p.RepeatDefinition,
		repeatFailed:        p.RepeatFailed,
		repeatPatchId:       p.RepeatPatchId,
		path:                p.Path,
		githubAuthor:        p.GithubAuthor,
		patchAuthor:         p.PatchAuthor,
		localModuleIncludes: p.LocalModuleIncludes,
	}

	newPatch, err := ac.PutPatch(patchSub)
	if err != nil {
		return nil, err
	}

	return newPatch, nil
}

func (p *patchParams) validateSubmission(diffData *localDiff) error {
	if err := validatePatchSize(diffData, p.Large); err != nil {
		return err
	}
	if !p.SkipConfirm && len(diffData.fullPatch) == 0 {
		if !confirm("Patch submission is empty. Continue?", true) {
			return errors.New("patch aborted")
		}
	} else if !p.SkipConfirm && diffData.patchSummary != "" {
		grip.Info(diffData.patchSummary)
		if diffData.log != "" {
			grip.Info(diffData.log)
		}

		if !confirm("This is a summary of the patch to be submitted. Continue?", true) {
			return errors.New("patch aborted")
		}
	}

	return nil
}

// displayPatch outputs the given patch(es) to the user. If there is only one patch,
// and browse is true, it will open the patch in the user's default web browser.
func (p *patchParams) displayPatch(ac *legacyClient, params outputPatchParams) error {
	patchDisp, err := getPatchDisplay(ac, params)
	if err != nil {
		return err
	}

	grip.InfoWhen(!params.outputJSON, "Patch successfully created.")
	grip.Info(patchDisp)

	if len(params.patches) == 1 && p.Browse {
		browserCmd, err := findBrowserCommand()
		if err != nil {
			grip.Warning(errors.Wrap(err, "finding browser command"))
			return nil
		}
		url := params.patches[0].GetURL(params.uiHost)
		if params.patches[0].IsCommitQueuePatch() {
			url = params.patches[0].GetCommitQueueURL(params.uiHost)
		}
		browserCmd = append(browserCmd, url)
		cmd := exec.Command(browserCmd[0], browserCmd[1:]...)
		return cmd.Run()
	}

	return nil
}

func findBrowserCommand() ([]string, error) {
	browser := os.Getenv("BROWSER")
	if browser != "" {
		return []string{browser}, nil
	}

	switch runtime.GOOS {
	case "darwin":
		return []string{"open"}, nil
	case "windows":
		return []string{"cmd", "/c", "start"}, nil
	default:
		candidates := []string{"xdg-open", "gnome-open", "x-www-browser", "firefox",
			"opera", "mozilla", "netscape"}
		for _, b := range candidates {
			path, err := exec.LookPath(b)
			if err == nil {
				return []string{path}, nil
			}
		}
	}

	return nil, errors.New("unable to find a web browser, try setting $BROWSER")
}

// Performs validation for patch or patch-file
func (p *patchParams) validatePatchCommand(ctx context.Context, conf *ClientSettings, ac *legacyClient, comm client.Communicator) (*model.ProjectRef, error) {
	if err := p.loadProject(conf); err != nil {
		grip.Warningf("warning - failed to set default project: %v\n", err)
	}

	// If reusing a previous definition, ignore defaults.
	if !p.RepeatFailed && !p.RepeatDefinition {
		p.setNonRepeatedDefaults(conf)
	}

	if err := p.loadParameters(conf); err != nil {
		grip.Warningf("warning - failed to set default parameters: %v\n", err)
	}

	if p.Uncommitted || conf.UncommittedChanges {
		p.Ref = ""
	}

	// Validate the project exists
	ref, err := ac.GetProjectRef(p.Project)
	if err != nil {
		if apiErr, ok := err.(APIError); ok && apiErr.code == http.StatusNotFound {
			err = errors.Errorf("%s \nRun `evergreen list --projects` to see all valid projects", err)
		}
		return nil, err
	}

	// Validate trigger aliases exist
	if len(p.TriggerAliases) > 0 {
		validTriggerAliases, err := comm.ListPatchTriggerAliases(ctx, p.Project)
		if err != nil {
			return nil, errors.Wrap(err, "fetching trigger aliases")
		}
		for _, alias := range p.TriggerAliases {
			if !utility.StringSliceContains(validTriggerAliases, alias) {
				return nil, errors.Errorf("Trigger alias '%s' is not defined for project '%s'. Valid aliases are %v", alias, p.Project, validTriggerAliases)
			}
		}
	}

	if p.Finalize && p.Alias == "" && !p.RepeatFailed && !p.RepeatDefinition {
		if len(p.Variants)+len(p.RegexVariants) == 0 || len(p.Tasks)+len(p.RegexTasks) == 0 {
			return ref, errors.Errorf("Need to specify at least one task/variant or alias when finalizing")
		}
	}

	return ref, nil
}

func (p *patchParams) setNonRepeatedDefaults(conf *ClientSettings) {
	if err := p.setLocalAliases(conf); err != nil {
		grip.Warningf("warning - setting local aliases: %s\n", err)
	}

	if err := p.loadAlias(conf); err != nil {
		grip.Warningf("warning - failed to set default alias: %s\n", err)
	}

	if err := p.loadVariants(conf); err != nil {
		grip.Warningf("warning - failed to set default variants: %s\n", err)
	}

	if err := p.loadTasks(conf); err != nil {
		grip.Warningf("warning - failed to set default tasks: %s\n", err)
	}

	if err := p.loadTriggerAliases(conf); err != nil {
		grip.Warningf("warning - failed to set default trigger aliases: %s\n", err)
	}
}

func (p *patchParams) loadProject(conf *ClientSettings) error {
	if p.Project == "" {
		cwd, err := os.Getwd()
		grip.Error(errors.Wrap(err, "getting current working directory"))
		cwd, err = filepath.EvalSymlinks(cwd)
		grip.Error(errors.Wrap(err, "resolving current working directory symlinks"))
		p.Project = conf.FindDefaultProject(cwd, true)
	}
	if p.Project == "" {
		return errors.New("Need to specify a project")
	}

	return nil
}

func (p *patchParams) setLocalAliases(conf *ClientSettings) error {
	if p.Alias != "" {
		for i := range conf.Projects {
			if conf.Projects[i].Name == p.Project {
				if errStrs := model.ValidateProjectAliases(conf.Projects[i].LocalAliases, "Local Aliases"); len(errStrs) != 0 {
					return errors.Errorf("validating local aliases: '%s'", errStrs)
				}
				for _, alias := range conf.Projects[i].LocalAliases {
					if alias.Alias == p.Alias {
						p.addAliasToPatchParams(alias)
					}
				}
			}
		}
	}
	return nil
}

// addAliasToPatchParams add the matching local alias to patch params.
func (p *patchParams) addAliasToPatchParams(alias model.ProjectAlias) {
	if alias.Variant != "" {
		p.RegexVariants = append(p.RegexVariants, strings.Split(alias.Variant, ",")...)
	}
	if alias.VariantTags != nil {
		var formattedTags []string
		for _, tag := range alias.VariantTags {
			formattedTags = append(formattedTags, fmt.Sprintf(".%s", tag))
		}
		p.Variants = append(p.Variants, formattedTags...)
	}
	if alias.Task != "" {
		p.RegexTasks = append(p.RegexTasks, strings.Split(alias.Task, ",")...)
	}
	if alias.TaskTags != nil {
		var formattedTags []string
		for _, tag := range alias.TaskTags {
			formattedTags = append(formattedTags, fmt.Sprintf(".%s", tag))
		}
		p.Tasks = append(p.Tasks, formattedTags...)
	}
	p.Alias = ""
	p.isUsingLocalAlias = true
}

func (p *patchParams) setDefaultProject(conf *ClientSettings) {
	cwd, err := os.Getwd()
	grip.Error(errors.Wrap(err, "getting current working directory"))
	cwd, err = filepath.EvalSymlinks(cwd)
	grip.Error(errors.Wrapf(err, "resolving symlinks for the current working directory '%s'", cwd))

	if conf.FindDefaultProject(cwd, false) == "" {
		conf.SetDefaultProject(cwd, p.Project)
		if err := conf.Write(""); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "failed to set default project",
				"project": p.Project,
			}))
		}
	}
}

// Sets the patch's alias to either the passed in option or the default
func (p *patchParams) loadAlias(conf *ClientSettings) error {
	// If somebody passed an --alias
	if p.Alias != "" {
		// Check if there's an alias as the default, and if not, ask to save the cl one
		defaultAlias := conf.FindDefaultAlias(p.Project)
		if defaultAlias == "" && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %s as the default alias for project '%s'?",
				p.Alias, p.Project), false) {
			conf.SetDefaultAlias(p.Project, p.Alias)
			if err := conf.Write(""); err != nil {
				return errors.Wrap(err, "setting default alias")
			}
		}
	} else if len(p.Variants) == 0 || len(p.Tasks) == 0 {
		// No --alias or variant/task pair was passed, use the default
		p.Alias = conf.FindDefaultAlias(p.Project)
		grip.InfoWhen(p.Alias != "", "Using default alias set in local config")
	}

	return nil
}

func (p *patchParams) loadVariants(conf *ClientSettings) error {
	if len(p.Variants) != 0 {
		defaultVariants := conf.FindDefaultVariants(p.Project)
		if len(defaultVariants) == 0 && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %s as the default variants for project '%s'?",
				p.Variants, p.Project), false) {
			conf.SetDefaultVariants(p.Project, p.Variants...)
			if err := conf.Write(""); err != nil {
				return errors.Wrap(err, "setting default variants")
			}
		}
	} else if p.Alias == "" && !p.isUsingLocalAlias {
		p.Variants = conf.FindDefaultVariants(p.Project)
		grip.InfoWhen(len(p.Variants) > 0, "Using default variants set in local config")
	}

	return nil
}

// Option to set default parameters no longer supported to prevent unwanted parameters from persisting within future patches
func (p *patchParams) loadParameters(conf *ClientSettings) error {
	if len(p.Parameters) == 0 {
		p.Parameters = conf.FindDefaultParameters(p.Project)
	}
	return nil
}

func (p *patchParams) loadTriggerAliases(conf *ClientSettings) error {
	defaultAliases := conf.FindDefaultTriggerAliases(p.Project)
	if len(p.TriggerAliases) != 0 {
		if len(defaultAliases) == 0 && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %s as the default trigger aliases for project '%s'?",
				p.TriggerAliases, p.Project), false) {
			conf.SetDefaultTriggerAliases(p.Project, p.TriggerAliases)
			if err := conf.Write(""); err != nil {
				return errors.Wrap(err, "setting default trigger aliases")
			}
		}
	} else {
		p.TriggerAliases = defaultAliases
	}
	return nil
}

func (p *patchParams) loadTasks(conf *ClientSettings) error {
	if len(p.Tasks) != 0 {
		defaultTasks := conf.FindDefaultTasks(p.Project)
		if len(defaultTasks) == 0 && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %s as the default tasks for project '%v'?",
				p.Tasks, p.Project), false) {
			conf.SetDefaultTasks(p.Project, p.Tasks...)
			if err := conf.Write(""); err != nil {
				return errors.Wrap(err, "setting default tasks")
			}
		}
	} else if p.Alias == "" && !p.isUsingLocalAlias {
		p.Tasks = conf.FindDefaultTasks(p.Project)
		grip.InfoWhen(len(p.Tasks) > 0, "Using default tasks set in local config")

	}

	return nil
}

func (p *patchParams) getDescription() string {
	if p.Description != "" {
		return p.Description
	} else if p.AutoDescription {
		description, err := getDefaultDescription()
		if err != nil {
			grip.Error(err)
		}
		return description
	}

	description := ""
	if !p.SkipConfirm {
		description = prompt("Enter a description for this patch (optional):")
	}

	if description == "" {
		var err error
		description, err = getDefaultDescription()
		if err != nil {
			grip.Error(err)
		}
	}

	return description
}

func (p *patchParams) getModulePath(conf *ClientSettings, module string) (string, error) {
	modulePath := conf.getModulePath(p.Project, module)
	if modulePath != "" || p.SkipConfirm {
		return modulePath, nil
	}

	modulePath = prompt(fmt.Sprintf("Enter absolute path to module '%s' to include changes (optional):", module))
	if modulePath == "" {
		return "", errors.Errorf("no module path given")
	}

	if !conf.DisableAutoDefaulting {
		// Verify that the path is correct before auto defaulting
		if _, err := gitUncommittedChanges(modulePath); err != nil {
			return "", errors.Wrapf(err, "verifying module '%s''", module)
		}
		conf.setModulePath(p.Project, module, modulePath)
		if err := conf.Write(""); err != nil {
			grip.Errorf("problem setting module '%s' path in config: %s", module, err.Error())
		}
	}

	return modulePath, nil
}

// Returns an error if the diff is greater than the system limit, or if it's above the large
// patch threhsold and allowLarge is not set.
func validatePatchSize(diff *localDiff, allowLarge bool) error {
	patchLen := len(diff.fullPatch)
	if patchLen > patch.SizeLimit {
		return errors.Errorf("Patch is greater than the system limit (%v > %v bytes).", patchLen, patch.SizeLimit)
	} else if patchLen > largePatchThreshold && !allowLarge {
		return errors.Errorf("Patch is larger than the default threshold (%v > %v bytes).\n"+
			"To allow submitting this patch, use the --large flag.", patchLen, largePatchThreshold)
	}

	// Patch is small enough and/or allowLarge is true, so no error
	return nil
}

type outputPatchParams struct {
	patches []patch.Patch
	uiHost  string

	summarize  bool
	outputJSON bool
}

// getPatchDisplay returns a string representation of the given patches
// according to the outputPatchParams.
func getPatchDisplay(ac *legacyClient, params outputPatchParams) (string, error) {
	if params.outputJSON {
		return getJSONPatchDisplay(params)
	}
	return getGenericPatchDisplay(ac, params)
}

// getGenericPatchDisplay returns a string representation of the given patches
// using our custom template.
func getGenericPatchDisplay(ac *legacyClient, params outputPatchParams) (string, error) {
	var out bytes.Buffer
	for _, p := range params.patches {
		var link string
		if p.IsCommitQueuePatch() {
			link = p.GetCommitQueueURL(params.uiHost)
		} else {
			link = p.GetURL(params.uiHost)
		}

		proj, err := ac.GetProjectRef(p.Project)
		if err != nil {
			return "", errors.Wrapf(err, "getting project ref for '%s'", p.Project)
		}
		if proj == nil {
			return "", errors.Errorf("project ref not found for '%s'", p.Project)
		}

		err = patchDisplayTemplate.Execute(&out, struct {
			Patch             *patch.Patch
			ShowSummary       bool
			ShowFinalized     bool
			Link              string
			ProjectIdentifier string
		}{
			Patch:             &p,
			ShowSummary:       params.summarize,
			ShowFinalized:     p.IsCommitQueuePatch(),
			Link:              link,
			ProjectIdentifier: proj.Identifier,
		})

		if err != nil {
			return "", errors.Wrapf(err, "executing patch display template for id %s", p.Id.Hex())
		}
	}

	return out.String(), nil
}

// getJSONPatchDisplay returns a string representation of the given patches
// using the JSON format. If there is only one patch, it will be displayed
// as a single JSON object. If there are multiple patches, they will be
// displayed as a JSON array.
func getJSONPatchDisplay(params outputPatchParams) (string, error) {
	display := []restModel.APIPatch{}
	for _, p := range params.patches {
		api := restModel.APIPatch{}
		err := api.BuildFromService(p, nil)
		if err != nil {
			return "", errors.Wrap(err, "converting patch to API model")
		}
		display = append(display, api)
	}
	var b []byte
	var err error
	if len(display) == 1 {
		b, err = json.MarshalIndent(display[0], "", "\t")
	} else {
		b, err = json.MarshalIndent(display, "", "\t")
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func isCommitRange(commits string) bool {
	return strings.Contains(commits, "..")
}

func formatCommitRange(commits string) string {
	if commits == "" {
		return commits
	}
	if isCommitRange(commits) {
		return commits
	}
	return fmt.Sprintf("%s^!", commits)
}

func getFeatureBranch(ref, commits string) string {
	if ref != "" {
		return ref
	}
	if commits != "" {
		// if one commit, this returns just that commit, else the first commit in the range
		return strings.Split(commits, "..")[0]
	}
	return "HEAD"
}

func confirmUncommittedChanges(dir string, preserveCommits, includeUncommitedChanges bool) (bool, error) {
	uncommittedChanges, err := gitUncommittedChanges(dir)
	if err != nil {
		return false, errors.Wrap(err, "getting uncommitted changes")
	}
	if !uncommittedChanges {
		return true, nil
	}

	if preserveCommits {
		return confirm("Uncommitted changes are omitted from patches when commits are preserved. Continue?", false), nil
	}

	if !includeUncommitedChanges {
		return confirm(fmt.Sprintf(`Uncommitted changes are omitted from patches by default.
Use the '--%s, -u' flag or set 'patch_uncommitted_changes: true' in your ~/.evergreen.yml file to include uncommitted changes.
Continue?`, uncommittedChangesFlag), true), nil
	}

	return true, nil
}

// loadGitData inspects the given git working directory and returns a patch and its summary.
// If no dir is provided, we use the current working directory.
// The branch argument is used to determine where to generate the merge base from, and any extra
// arguments supplied are passed directly in as additional args to git diff.
func loadGitData(dir, remote, branch, ref, commits string, format bool, extraArgs ...string) (*localDiff, error) {
	// branch@{upstream} refers to the branch that the branch specified by branchname is set to
	// build on top of. For example, if a user's repo is a fork, this allows automatic detection
	// of a branch based on the correct remote. This also works with a commit hash, if given.
	// In the case a range is passed, we only need one commit to determine the base, so we use the first commit.
	// For details see: https://git-scm.com/docs/gitrevisions

	if remote == "" {
		remote = "upstream"
	}

	mergeBase, err := gitMergeBase(dir, fmt.Sprintf("%s/%s", remote, branch), ref, commits)
	if err != nil {
		mergeBase, err = gitMergeBase(dir, branch+"@{upstream}", ref, commits)
		if err != nil {
			return nil, errors.Wrapf(err, "Error getting merge base, "+
				"may need to create local branch '%s' and have it track your Evergreen project", branch)
		}
	}
	statArgs := []string{"--stat"}
	if len(extraArgs) > 0 {
		statArgs = append(statArgs, extraArgs...)
	}
	stat, err := gitDiff(dir, mergeBase, ref, commits, statArgs...)
	if err != nil {
		return nil, errors.Wrap(err, "getting diff summary")
	}
	log, err := gitLog(dir, mergeBase, ref, commits)
	if err != nil {
		return nil, errors.Wrap(err, "git log")
	}

	var fullPatch string
	if format {
		fullPatch, err = gitFormatPatch(dir, mergeBase, ref, commits)
		if err != nil {
			return nil, errors.Wrap(err, "getting git formatted patch")
		}
	} else {
		if !utility.StringSliceContains(extraArgs, "--binary") {
			extraArgs = append(extraArgs, "--binary")
		}
		fullPatch, err = gitDiff(dir, mergeBase, ref, commits, extraArgs...)
		if err != nil {
			return nil, errors.Wrap(err, "getting git diff")
		}
	}

	gitMetadata, err := getGitConfigMetadata()
	if err != nil {
		return nil, errors.Wrap(err, "getting git metadata")
	}

	return &localDiff{
		fullPatch:    fullPatch,
		patchSummary: stat,
		log:          log,
		base:         mergeBase,
		gitMetadata:  gitMetadata,
	}, nil
}

func gitGetRemote(dir, owner, repo string) (string, error) {
	out, err := gitCmdWithDir("remote", dir, "-v")
	if err != nil {
		return "", errors.Wrap(err, "getting git remotes")
	}

	return getRemoteFromOutput(out, owner, repo)
}

func getRemoteFromOutput(output, owner, repo string) (string, error) {
	lines := strings.Split(output, "\n")
	partial := strings.ToLower(fmt.Sprintf("%s/%s", owner, repo))

	// git remote -v has a format of
	// <remote_name> <url> [fetch|pull]
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) > 1 && strings.Contains(strings.ToLower(f[1]), partial) {
			return f[0], nil
		}
	}

	return "", errors.New("no git remote found")
}

// gitMergeBase runs "git merge-base <branch1> <branch2>" (where branch2 can optionally be a githash)
// and returns the resulting githash as string
func gitMergeBase(dir, branch1, ref, commits string) (string, error) {
	branch2 := getFeatureBranch(ref, commits)
	out, err := gitCmdWithDir("merge-base", dir, branch1, branch2)

	return strings.TrimSpace(out), err
}

// gitDiff runs "git diff <base> <ref> <commits> <diffargs ...>" and returns the output of the command as a string,
// where ref and commits are mutually exclusive (and not required). If dir is specified, runs the command
// in the specified directory.
func gitDiff(dir, base, ref, commits string, diffArgs ...string) (string, error) {
	args := []string{base}
	if commits != "" {
		args = []string{formatCommitRange(commits)}
	}
	if ref != "" {
		args = append(args, ref)
	}
	args = append(args, "--no-ext-diff")
	args = append(args, diffArgs...)
	return gitCmdWithDir("diff", dir, args...)
}

func gitFormatPatch(dir, base string, ref, commits string) (string, error) {
	revisionRange := fmt.Sprintf("%s..%s", base, ref)
	if commits != "" {
		revisionRange = formatCommitRange(commits)
	}
	return gitCmdWithDir("format-patch", dir, "--keep-subject", "--no-signature", "--stdout", "--no-ext-diff", "--binary", revisionRange)
}

// getLog runs "git log <base>...<ref> or uses the commit range given
func gitLog(dir, base, ref, commits string) (string, error) {
	revisionRange := fmt.Sprintf("%s...%s", base, ref)
	if commits != "" {
		revisionRange = formatCommitRange(commits)
	}
	return gitCmdWithDir("log", dir, revisionRange, "--oneline")
}

// assumes base includes @{upstream}
func gitLastCommitMessage() (string, error) {
	args := []string{"HEAD", "--no-show-signature", "--pretty=format:%s", "-n 1"}
	return gitCmd("log", args...)
}

func gitBranch() (string, error) {
	args := []string{"--abbrev-ref", "HEAD"}
	return gitCmd("rev-parse", args...)
}

func getDefaultDescription() (string, error) {
	desc, err := gitLastCommitMessage()
	if err != nil {
		return "", errors.Wrap(err, "getting last git commit message")
	}
	branch, err := gitBranch()
	if err != nil {
		return "", errors.Wrap(err, "getting git branch name")
	}

	branch = strings.TrimSpace(branch)
	if strings.HasPrefix(desc, branch) {
		return desc, nil
	}
	return fmt.Sprintf("%s: %s", branch, desc), nil
}

func gitUncommittedChanges(dir string) (bool, error) {
	args := "--porcelain"
	out, err := gitCmdWithDir("status", dir, args)
	if err != nil {
		return false, errors.Wrap(err, "running git status")
	}
	return len(out) != 0, nil
}

func getGitConfigMetadata() (patch.GitMetadata, error) {
	var err error
	metadata := patch.GitMetadata{}
	username, err := gitCmd("config", "user.name")
	if err != nil {
		return metadata, errors.Wrap(err, "getting git user.name")
	}
	metadata.Username = strings.TrimSpace(username)

	email, err := gitCmd("config", "user.email")
	if err != nil {
		return metadata, errors.Wrap(err, "getting git user.email")
	}
	metadata.Email = strings.TrimSpace(email)

	version, err := getGitVersion()
	if err == nil {
		metadata.GitVersion = version
	}

	return metadata, nil
}

func getGitVersion() (string, error) {
	// We need just the version number, but git gives it as part of a larger string.
	// Parse the version number out of the version string.
	versionString, err := gitCmd("version")
	if err != nil {
		return "", errors.Wrap(err, "getting git version")
	}

	return parseGitVersion(strings.TrimSpace(versionString))
}

func parseGitVersion(version string) (string, error) {
	matches := regexp.MustCompile(`^git version ` +
		// capture the version major.minor(.patch(.build(.etc...)))
		`(\w+(?:\.\w+)+)` +
		// match and discard Apple git's addition to the version string
		`(?: \(Apple Git-[\w\.]+\))?$`,
	).FindStringSubmatch(version)
	if len(matches) != 2 {
		return "", errors.Errorf("could not parse git version number from version string '%s'", version)
	}

	return matches[1], nil
}

func gitCmd(cmdName string, gitArgs ...string) (string, error) {
	args := make([]string, 0, 1+len(gitArgs))
	args = append(args, cmdName)
	args = append(args, gitArgs...)
	return gitExecCmd(args)
}

func gitCmdWithDir(cmdName, dir string, gitArgs ...string) (string, error) {
	args := []string{}
	if dir != "" {
		args = append(args, moduleDirArgs(dir)...)
	}
	args = append(args, cmdName)
	args = append(args, gitArgs...)

	return gitExecCmd(args)
}

// gitExecCmd assumes that the command name has already been added to args, and executes the command.
func gitExecCmd(args []string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "command 'git %s' failed", strings.Join(args, " "))
	}
	return string(out), nil
}

func moduleDirArgs(path string) []string {
	str1 := fmt.Sprintf("--git-dir=%s/.git", path)
	str2 := fmt.Sprintf("--work-tree=%s", path)
	return []string{str1, str2}
}

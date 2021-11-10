package operations

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Above this size, the user must explicitly use --large to submit the patch (or confirm)
const largePatchThreshold = 1024 * 1024 * 16

// This is the template used to render a patch's summary in a human-readable output format.
var patchDisplayTemplate = template.Must(template.New("patch").Parse(`
	     ID : {{.Patch.Id.Hex}}
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
	Project           string
	Path              string
	Alias             string
	Variants          []string
	Tasks             []string
	SyncBuildVariants []string
	SyncTasks         []string
	SyncStatuses      []string
	SyncTimeout       time.Duration
	Description       string
	SkipConfirm       bool
	Finalize          bool
	Browse            bool
	Large             bool
	ShowSummary       bool
	Uncommitted       bool
	PreserveCommits   bool
	Ref               string
	BackportOf        patch.BackportInfo
	TriggerAliases    []string
	Parameters        []patch.Parameter
	ReuseDefinition   bool
}

type patchSubmission struct {
	projectName       string
	patchData         string
	description       string
	base              string
	alias             string
	path              string
	variants          []string
	tasks             []string
	syncBuildVariants []string
	syncTasks         []string
	syncStatuses      []string
	syncTimeout       time.Duration
	finalize          bool
	parameters        []patch.Parameter
	triggerAliases    []string
	backportOf        patch.BackportInfo
	gitMetadata       patch.GitMetadata
	reuseDefinition   bool
}

func (p *patchParams) createPatch(ac *legacyClient, diffData *localDiff) (*patch.Patch, error) {
	patchSub := patchSubmission{
		projectName:       p.Project,
		patchData:         diffData.fullPatch,
		description:       p.Description,
		base:              diffData.base,
		variants:          p.Variants,
		tasks:             p.Tasks,
		alias:             p.Alias,
		syncBuildVariants: p.SyncBuildVariants,
		syncTasks:         p.SyncTasks,
		syncStatuses:      p.SyncStatuses,
		syncTimeout:       p.SyncTimeout,
		finalize:          p.Finalize,
		backportOf:        p.BackportOf,
		parameters:        p.Parameters,
		triggerAliases:    p.TriggerAliases,
		gitMetadata:       diffData.gitMetadata,
		reuseDefinition:   p.ReuseDefinition,
		path:              p.Path,
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
		if !confirm("Patch submission is empty. Continue? (Y/n)", true) {
			return errors.New("patch aborted")
		}
	} else if !p.SkipConfirm && diffData.patchSummary != "" {
		grip.Info(diffData.patchSummary)
		if diffData.log != "" {
			grip.Info(diffData.log)
		}

		if !confirm("This is a summary of the patch to be submitted. Continue? (Y/n):", true) {
			return errors.New("patch aborted")
		}
	}

	return nil
}

func (p *patchParams) displayPatch(newPatch *patch.Patch, uiHost string, isCommitQueuePatch bool) error {
	patchDisp, err := getPatchDisplay(newPatch, p.ShowSummary, uiHost, isCommitQueuePatch)
	if err != nil {
		return err
	}

	grip.Info("Patch successfully created.")
	grip.Info(patchDisp)

	if p.Browse {
		browserCmd, err := findBrowserCommand()
		if err != nil || len(browserCmd) == 0 {
			grip.Warningf("cannot find browser command: %s", err)
			return nil
		}
		url := newPatch.GetURL(uiHost)
		if isCommitQueuePatch {
			url = newPatch.GetCommitQueueURL(uiHost)
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

	if err := p.loadAlias(conf); err != nil {
		grip.Warningf("warning - failed to set default alias: %v\n", err)
	}

	if err := p.loadVariants(conf); err != nil {
		grip.Warningf("warning - failed to set default variants: %v\n", err)
	}

	if err := p.loadTasks(conf); err != nil {
		grip.Warningf("warning - failed to set default tasks: %v\n", err)
	}

	if err := p.loadParameters(conf); err != nil {
		grip.Warningf("warning - failed to set default parameters: %v\n", err)
	}

	if err := p.loadTriggerAliases(conf); err != nil {
		grip.Warningf("warning - failed to set default trigger aliases: %v\n", err)
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

	// Validate the alias exists
	if p.Alias != "" {
		validAlias := false
		aliases, err := comm.ListAliases(ctx, ref.Id)
		if err != nil {
			return nil, errors.Wrap(err, "error contacting API server")
		}
		for _, alias := range aliases {
			if alias.Alias == p.Alias {
				validAlias = true
				break
			}
		}
		if !validAlias {
			return nil, errors.Errorf("%s is not a valid alias", p.Alias)
		}
	}

	// Validate trigger aliases exist
	if len(p.TriggerAliases) > 0 {
		validTriggerAliases, err := comm.ListPatchTriggerAliases(ctx, p.Project)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching trigger aliases")
		}
		for _, alias := range p.TriggerAliases {
			if !utility.StringSliceContains(validTriggerAliases, alias) {
				return nil, errors.Errorf("Trigger alias '%s' is not defined for project '%s'. Valid aliases are %v", alias, p.Project, validTriggerAliases)
			}
		}
	}

	if (len(p.Tasks) == 0 || len(p.Variants) == 0) && p.Alias == "" && p.Finalize {
		return ref, errors.Errorf("Need to specify at least one task/variant or alias when finalizing.")
	}

	return ref, nil
}

func (p *patchParams) loadProject(conf *ClientSettings) error {
	if p.Project == "" {
		cwd, err := os.Getwd()
		grip.Error(errors.Wrap(err, "unable to get current working directory"))
		cwd, err = filepath.EvalSymlinks(cwd)
		grip.Error(errors.Wrap(err, "unable to resolve symlinks"))
		p.Project = conf.FindDefaultProject(cwd, true)
	}
	if p.Project == "" {
		return errors.New("Need to specify a project")
	}

	return nil
}

func (p *patchParams) setDefaultProject(conf *ClientSettings) {
	cwd, err := os.Getwd()
	grip.Error(errors.Wrap(err, "unable to get current working directory"))
	cwd, err = filepath.EvalSymlinks(cwd)
	grip.Error(errors.Wrap(err, "unable to resolve symlinks"))

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
			confirm(fmt.Sprintf("Set %v as the default alias for project '%v'?",
				p.Alias, p.Project), false) {
			conf.SetDefaultAlias(p.Project, p.Alias)
			if err := conf.Write(""); err != nil {
				return err
			}
		}
	} else if len(p.Variants) == 0 || len(p.Tasks) == 0 {
		// No --alias or variant/task pair was passed, use the default
		p.Alias = conf.FindDefaultAlias(p.Project)
	}

	return nil
}

func (p *patchParams) loadVariants(conf *ClientSettings) error {
	if len(p.Variants) != 0 {
		defaultVariants := conf.FindDefaultVariants(p.Project)
		if len(defaultVariants) == 0 && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default variants for project '%v'?",
				p.Variants, p.Project), false) {
			conf.SetDefaultVariants(p.Project, p.Variants...)
			if err := conf.Write(""); err != nil {
				return err
			}
		}
	} else if p.Alias == "" {
		p.Variants = conf.FindDefaultVariants(p.Project)
	}

	return nil
}

//Option to set default parameters no longer supported to prevent unwanted parameters from persisting within future patches
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
			confirm(fmt.Sprintf("Set %v as the default trigger aliases for project '%v'?",
				p.TriggerAliases, p.Project), false) {
			conf.SetDefaultTriggerAliases(p.Project, p.TriggerAliases)
			if err := conf.Write(""); err != nil {
				return err
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
			confirm(fmt.Sprintf("Set %v as the default tasks for project '%v'?",
				p.Tasks, p.Project), false) {
			conf.SetDefaultTasks(p.Project, p.Tasks...)
			if err := conf.Write(""); err != nil {
				return err
			}
		}
	} else if p.Alias == "" {
		p.Tasks = conf.FindDefaultTasks(p.Project)
	}

	return nil
}

func (p *patchParams) getDescription() string {
	if p.Description != "" {
		return p.Description
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

// getPatchDisplay returns a human-readable summary representation of a patch object
// which can be written to the terminal.
func getPatchDisplay(p *patch.Patch, summarize bool, uiHost string, isCommitQueuePatch bool) (string, error) {
	var out bytes.Buffer
	var link string
	if isCommitQueuePatch {
		link = p.GetCommitQueueURL(uiHost)
	} else {
		link = p.GetURL(uiHost)
	}

	err := patchDisplayTemplate.Execute(&out, struct {
		Patch         *patch.Patch
		ShowSummary   bool
		ShowFinalized bool
		Link          string
	}{
		Patch:         p,
		ShowSummary:   summarize,
		ShowFinalized: p.IsCommitQueuePatch(),
		Link:          link,
	})
	if err != nil {
		return "", err
	}
	return out.String(), nil
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

func isValidCommitsFormat(commits string) error {
	errToReturn := errors.New("Invalid commit format: verify input is of the form `<hash1> OR `<hash1>..<hash2>` (where hash1 is an ancestor of hash2)")
	if commits == "" || !isCommitRange(commits) {
		return nil
	}

	commitsList := strings.Split(commits, "..")
	if len(commitsList) != 2 { // extra check
		return errToReturn
	}

	if _, err := gitIsAncestor(commitsList[0], strings.Trim(commitsList[1], ".")); err != nil {
		// suppressing given error bc it's not helpful
		return errToReturn
	}

	return nil
}

func confirmUncommittedChanges(preserveCommits, includeUncommitedChanges bool) (bool, error) {
	uncommittedChanges, err := gitUncommittedChanges()
	if err != nil {
		return false, errors.Wrap(err, "can't test for uncommitted changes")
	}
	if !uncommittedChanges {
		return true, nil
	}

	if preserveCommits {
		return confirm("Uncommitted changes are omitted from patches when commits are preserved. Continue? (y/N)", false), nil
	}

	if !includeUncommitedChanges {
		return confirm(fmt.Sprintf(`Uncommitted changes are omitted from patches by default.
Use the '--%s, -u' flag or set 'patch_uncommitted_changes: true' in your ~/.evergreen.yml file to include uncommitted changes.
Continue? (Y/n)`, uncommittedChangesFlag), true), nil
	}

	return true, nil
}

// loadGitData inspects the current git working directory and returns a patch and its summary.
// The branch argument is used to determine where to generate the merge base from, and any extra
// arguments supplied are passed directly in as additional args to git diff.
func loadGitData(branch, ref, commits string, format bool, extraArgs ...string) (*localDiff, error) {
	// branch@{upstream} refers to the branch that the branch specified by branchname is set to
	// build on top of. This allows automatically detecting a branch based on the correct remote,
	// if the user's repo is a fork, for example. This also works with a commit hash, if given.
	// In the case a range is passed, we only need one commit to determine the base, so we use the first commit.
	// For details see: https://git-scm.com/docs/gitrevisions

	mergeBase, err := gitMergeBase(branch+"@{upstream}", ref, commits)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting merge base")
	}
	statArgs := []string{"--stat"}
	if len(extraArgs) > 0 {
		statArgs = append(statArgs, extraArgs...)
	}
	stat, err := gitDiff(mergeBase, ref, commits, statArgs...)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting diff summary")
	}
	log, err := gitLog(mergeBase, ref, commits)
	if err != nil {
		return nil, errors.Wrap(err, "git log")
	}

	var fullPatch string
	if format {
		fullPatch, err = gitFormatPatch(mergeBase, ref, commits)
		if err != nil {
			return nil, errors.Wrap(err, "Error getting formatted patch")
		}
	} else {
		if !utility.StringSliceContains(extraArgs, "--binary") {
			extraArgs = append(extraArgs, "--binary")
		}
		fullPatch, err = gitDiff(mergeBase, ref, commits, extraArgs...)
		if err != nil {
			return nil, errors.Wrap(err, "Error getting patch")
		}
	}

	gitMetadata, err := getGitConfigMetadata()
	if err != nil {
		return nil, errors.Wrap(err, "Error getting git metadata")
	}

	return &localDiff{
		fullPatch:    fullPatch,
		patchSummary: stat,
		log:          log,
		base:         mergeBase,
		gitMetadata:  gitMetadata,
	}, nil
}

// gitMergeBase runs "git merge-base <branch1> <branch2>" (where branch2 can optionally be a githash)
// and returns the resulting githash as string
func gitMergeBase(branch1, ref, commits string) (string, error) {
	branch2 := getFeatureBranch(ref, commits)
	cmd := exec.Command("git", "merge-base", branch1, branch2)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "'git merge-base %s %s' failed: %s (%s)", branch1, branch2, out, err)
	}
	return strings.TrimSpace(string(out)), err
}

func gitIsAncestor(commit1, commit2 string) (string, error) {
	args := []string{"--is-ancestor", commit1, commit2}
	return gitCmd("merge-base", args...)
}

// gitDiff runs "git diff <base> <ref> <commits> <diffargs ...>" and returns the output of the command as a string,
// where ref and commits are mutually exclusive (and not required)
func gitDiff(base string, ref, commits string, diffArgs ...string) (string, error) {
	args := []string{base}
	if commits != "" {
		args = []string{formatCommitRange(commits)}
	}
	if ref != "" {
		args = append(args, ref)
	}
	args = append(args, "--no-ext-diff")
	args = append(args, diffArgs...)
	return gitCmd("diff", args...)
}

func gitFormatPatch(base string, ref, commits string) (string, error) {
	revisionRange := fmt.Sprintf("%s..%s", base, ref)
	if commits != "" {
		revisionRange = formatCommitRange(commits)
	}
	return gitCmd("format-patch", "--keep-subject", "--no-signature", "--stdout", "--no-ext-diff", "--binary", revisionRange)
}

// getLog runs "git log <base>...<ref> or uses the commit range given
func gitLog(base, ref, commits string) (string, error) {
	revisionRange := fmt.Sprintf("%s...%s", base, ref)
	if commits != "" {
		revisionRange = formatCommitRange(commits)
	}
	return gitCmd("log", revisionRange, "--oneline")
}

func gitCommitMessages(base, ref, commits string) (string, error) {
	input := fmt.Sprintf("%s@{upstream}..%s", base, ref)
	if commits != "" {
		input = formatCommitRange(commits)
	}
	args := []string{"--no-show-signature", "--pretty=format:%s", "--reverse", input}
	msg, err := gitCmd("log", args...)
	if err != nil {
		return "", errors.Wrap(err, "can't get messages")
	}
	// separate multiple commits with <-
	msg = strings.TrimSpace(msg)
	msg = strings.Replace(msg, "\n", " <- ", -1)

	return msg, nil
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
		return "", errors.Wrap(err, "Couldn't get last commit message")
	}
	branch, err := gitBranch()
	if err != nil {
		return "", errors.Wrap(err, "Couldn't get branch name")
	}

	branch = strings.TrimSpace(branch)
	if strings.HasPrefix(desc, branch) {
		return desc, nil
	}
	return fmt.Sprintf("%s: %s", branch, desc), nil
}

func gitCommitCount(base, ref, commits string) (int, error) {
	input := fmt.Sprintf("%s@{upstream}..%s", base, ref)
	if commits != "" {
		input = formatCommitRange(commits)
	}
	out, err := gitCmd("rev-list", input, "--count")
	if err != nil {
		return 0, errors.Wrap(err, "can't get commit count")
	}

	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, errors.Wrapf(err, "'%s' is not an integer", out)
	}

	return count, nil
}

func gitUncommittedChanges() (bool, error) {
	args := "--porcelain"
	out, err := gitCmd("status", args)
	if err != nil {
		return false, errors.Wrap(err, "can't run git status")
	}
	return len(out) != 0, nil
}

func getGitConfigMetadata() (patch.GitMetadata, error) {
	var err error
	metadata := patch.GitMetadata{}
	username, err := gitCmd("config", "user.name")
	if err != nil {
		return metadata, errors.Wrap(err, "can't get git user.name")
	}
	metadata.Username = strings.TrimSpace(username)

	email, err := gitCmd("config", "user.email")
	if err != nil {
		return metadata, errors.Wrap(err, "can't get git user.email")
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
		return "", errors.Wrap(err, "can't run git version")
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
		return "", errors.Errorf("can't parse git version number from version string '%s'", version)
	}

	return matches[1], nil
}

func gitCmd(cmdName string, gitArgs ...string) (string, error) {
	args := make([]string, 0, 1+len(gitArgs))
	args = append(args, cmdName)
	args = append(args, gitArgs...)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Errorf("'git %s' failed with err %s", strings.Join(args, " "), err)
	}
	return string(out), nil
}

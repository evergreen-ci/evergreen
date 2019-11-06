package operations

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
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
      Finalized : {{if .Patch.Activated}}Yes{{else}}No{{end}}
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
}

type patchParams struct {
	Project     string
	Variants    []string
	Tasks       []string
	Description string
	Alias       string
	SkipConfirm bool
	Finalize    bool
	Browse      bool
	Large       bool
	ShowSummary bool
	Uncommitted bool
	Ref         string
}

type patchSubmission struct {
	projectId   string
	patchData   string
	description string
	base        string
	alias       string
	variants    []string
	tasks       []string
	finalize    bool
}

func (p *patchParams) createPatch(ac *legacyClient, conf *ClientSettings, diffData *localDiff) (*patch.Patch, error) {
	if err := validatePatchSize(diffData, p.Large); err != nil {
		return nil, err
	}
	if !p.SkipConfirm && len(diffData.fullPatch) == 0 {
		if !confirm("Patch submission is empty. Continue?(y/n)", true) {
			return nil, errors.New("patch aborted")
		}
	} else if !p.SkipConfirm && diffData.patchSummary != "" {
		grip.Info(diffData.patchSummary)
		if diffData.log != "" {
			grip.Info(diffData.log)
		}

		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil, errors.New("patch aborted")
		}
	}

	var err error
	if p.Description == "" {
		p.Description, err = gitLastCommitMessage()
		if err != nil {
			grip.Debug("Couldn't create patch description using commit messages.")
		}
	}

	patchSub := patchSubmission{
		projectId:   p.Project,
		patchData:   diffData.fullPatch,
		description: p.Description,
		base:        diffData.base,
		variants:    p.Variants,
		tasks:       p.Tasks,
		finalize:    p.Finalize,
		alias:       p.Alias,
	}

	newPatch, err := ac.PutPatch(patchSub)
	if err != nil {
		return nil, err
	}
	patchDisp, err := getPatchDisplay(newPatch, p.ShowSummary, conf.UIServerHost)
	if err != nil {
		return nil, err
	}

	grip.Info("Patch successfully created.")
	grip.Info(patchDisp)

	if p.Browse {
		browserCmd, err := findBrowserCommand()
		if err != nil || len(browserCmd) == 0 {
			grip.Warningf("cannot find browser command: %s", err)
			return newPatch, nil
		}

		var url string
		if newPatch.Activated {
			url = conf.UIServerHost + "/version/" + newPatch.Id.Hex()
		} else {
			url = conf.UIServerHost + "/patch/" + newPatch.Id.Hex()
		}

		browserCmd = append(browserCmd, url)
		cmd := exec.Command(browserCmd[0], browserCmd[1:]...)
		return newPatch, cmd.Run()
	}

	return newPatch, nil
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
		aliases, err := comm.ListAliases(ctx, p.Project)
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

	if (len(p.Tasks) == 0 || len(p.Variants) == 0) && p.Alias == "" && p.Finalize {
		return ref, errors.Errorf("Need to specify at least one task/variant or alias when finalizing.")
	}

	if p.Description == "" && !p.SkipConfirm {
		p.Description = prompt("Enter a description for this patch (optional):")
	}

	return ref, nil
}

func (p *patchParams) loadProject(conf *ClientSettings) error {
	if p.Project == "" {
		p.Project = conf.FindDefaultProject()
	} else {
		if conf.FindDefaultProject() == "" &&
			!p.SkipConfirm && confirm(fmt.Sprintf("Make %s your default project?", p.Project), true) {
			conf.SetDefaultProject(p.Project)
			if err := conf.Write(""); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"message": "failed to set default project",
					"project": p.Project,
				}))
			}
		}
	}

	if p.Project == "" {
		return errors.New("Need to specify a project")
	}

	return nil
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
func getPatchDisplay(p *patch.Patch, summarize bool, uiHost string) (string, error) {
	var out bytes.Buffer
	var url string
	if p.Activated {
		url = uiHost + "/version/" + p.Id.Hex()
	} else {
		url = uiHost + "/patch/" + p.Id.Hex()
	}

	err := patchDisplayTemplate.Execute(&out, struct {
		Patch       *patch.Patch
		ShowSummary bool
		Link        string
	}{
		Patch:       p,
		ShowSummary: summarize,
		Link:        url,
	})
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// loadGitData inspects the current git working directory and returns a patch and its summary.
// The branch argument is used to determine where to generate the merge base from, and any extra
// arguments supplied are passed directly in as additional args to git diff.
func loadGitData(branch string, ref string, extraArgs ...string) (*localDiff, error) {
	// branch@{upstream} refers to the branch that the branch specified by branchname is set to
	// build on top of. This allows automatically detecting a branch based on the correct remote,
	// if the user's repo is a fork, for example.
	// For details see: https://git-scm.com/docs/gitrevisions
	var featureBranch string
	if ref == "" {
		featureBranch = "HEAD"
	} else {
		featureBranch = ref
	}
	mergeBase, err := gitMergeBase(branch+"@{upstream}", featureBranch)
	if err != nil {
		return nil, errors.Errorf("Error getting merge base: %v", err)
	}
	statArgs := []string{"--stat"}
	if len(extraArgs) > 0 {
		statArgs = append(statArgs, extraArgs...)
	}
	stat, err := gitDiff(mergeBase, ref, statArgs...)
	if err != nil {
		return nil, errors.Errorf("Error getting diff summary: %v", err)
	}
	log, err := gitLog(mergeBase, ref)
	if err != nil {
		return nil, errors.Errorf("git log: %v", err)
	}

	if !util.StringSliceContains(extraArgs, "--binary") {
		extraArgs = append(extraArgs, "--binary")
	}

	patch, err := gitDiff(mergeBase, ref, extraArgs...)
	if err != nil {
		return nil, errors.Errorf("Error getting patch: %v", err)
	}
	return &localDiff{patch, stat, log, mergeBase}, nil
}

// gitMergeBase runs "git merge-base <branch1> <branch2>" and returns the
// resulting githash as string
func gitMergeBase(branch1, branch2 string) (string, error) {
	cmd := exec.Command("git", "merge-base", branch1, branch2)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "'git merge-base %s %s' failed: %s (%s)", branch1, branch2, out, err)
	}
	return strings.TrimSpace(string(out)), err
}

// gitDiff runs "git diff <base> <diffargs ...>" and returns the output of the command as a string
func gitDiff(base string, ref string, diffArgs ...string) (string, error) {
	args := []string{base}
	if ref != "" {
		args = append(args, ref)
	}
	args = append(args, "--no-ext-diff")
	args = append(args, diffArgs...)
	return gitCmd("diff", args...)
}

// getLog runs "git log <base>...<ref>
func gitLog(base, ref string) (string, error) {
	args := []string{fmt.Sprintf("%s...%s", base, ref), "--oneline"}
	return gitCmd("log", args...)
}

func gitCommitMessages(base, ref string) (string, error) {
	args := []string{"--no-show-signature", "--pretty=format:%B", fmt.Sprintf("%s@{upstream}..%s", base, ref)}
	return gitCmd("log", args...)
}

// assumes base includes @{upstream}
func gitLastCommitMessage() (string, error) {
	args := []string{"HEAD", "--no-show-signature", "--pretty=format:%s", "-n 1"}
	return gitCmd("log", args...)
}

func gitCommitCount(base, ref string) (int, error) {
	args := []string{fmt.Sprintf("%s@{upstream}..%s", base, ref), "--count"}
	out, err := gitCmd("rev-list", args...)
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

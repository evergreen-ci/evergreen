package operations

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// Above this size, the user must explicitly use --large to submit the patch (or confirm)
const largePatchThreshold = 1024 * 1024 * 16

// This is the template used to render a patch's summary in a human-readable output format.
var patchDisplayTemplate = template.Must(template.New("patch").Parse(`
	     ID : {{.Patch.Id.Hex}}
	Created : {{.Now.Sub .Patch.CreateTime}} ago
    Description : {{if .Patch.Description}}{{.Patch.Description}}{{else}}<none>{{end}}
	  Build : {{.Link}}
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
	Large       bool
	ShowSummary bool
}

type patchSubmission struct {
	projectId   string
	patchData   string
	description string
	base        string
	alias       string
	variants    string
	tasks       []string
	finalize    bool
}

func (p *patchParams) createPatch(ac *legacyClient, conf *ClientSettings, diffData *localDiff) error {
	if err := validatePatchSize(diffData, p.Large); err != nil {
		return err
	}
	if !p.SkipConfirm && len(diffData.fullPatch) == 0 {
		if !confirm("Patch submission is empty. Continue?(y/n)", true) {
			return nil
		}
	} else if !p.SkipConfirm && diffData.patchSummary != "" {
		fmt.Println(diffData.patchSummary)
		if diffData.log != "" {
			fmt.Println(diffData.log)
		}

		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil
		}
	}

	variantsStr := strings.Join(p.Variants, ",")
	patchSub := patchSubmission{
		projectId:   p.Project,
		patchData:   diffData.fullPatch,
		description: p.Description,
		base:        diffData.base,
		variants:    variantsStr,
		tasks:       p.Tasks,
		finalize:    p.Finalize,
		alias:       p.Alias,
	}

	newPatch, err := ac.PutPatch(patchSub)
	if err != nil {
		return err
	}
	patchDisp, err := getPatchDisplay(newPatch, p.ShowSummary, conf.UIServerHost)
	if err != nil {
		return err
	}

	if p.Alias != "" {
		fmt.Printf("activated tasks on %d variants...\n", len(newPatch.VariantsTasks))
		for _, v := range newPatch.VariantsTasks {
			fmt.Printf("\ntasks for variant %s:\n", v.Variant)
			for _, t := range v.Tasks {
				fmt.Println(t)
			}
		}
		fmt.Printf("\n")
	}

	fmt.Println("Patch successfully created.")
	fmt.Print(patchDisp)
	return nil
}

// Performs validation for patch or patch-file
func (p *patchParams) validatePatchCommand(ctx context.Context, conf *ClientSettings, ac *legacyClient, comm client.Communicator) (ref *model.ProjectRef, err error) {
	if p.Project == "" {
		p.Project = conf.FindDefaultProject()
	} else {
		if conf.FindDefaultProject() == "" &&
			!p.SkipConfirm && confirm(fmt.Sprintf("Make %v your default project?", p.Project), true) {
			conf.SetDefaultProject(p.Project)
			if err = conf.Write(""); err != nil {
				fmt.Printf("warning - failed to set default project: %v\n", err)
			}
		}
	}

	if p.Project == "" {
		err = errors.Errorf("Need to specify a project.")
		return
	}

	if p.Alias != "" {
		validAlias := false
		var aliases []model.PatchDefinition
		aliases, err = comm.ListAliases(ctx, p.Project)
		if err != nil {
			err = errors.Wrap(err, "error contacting API server")
			return
		}
		for _, alias := range aliases {
			if alias.Alias == p.Alias {
				validAlias = true
				break
			}
		}
		if !validAlias {
			err = errors.Errorf("%s is not a valid alias", params.Alias)
			return
		}
	}

	ref, err = ac.GetProjectRef(p.Project)
	if err != nil {
		if apiErr, ok := err.(APIError); ok && apiErr.code == http.StatusNotFound {
			err = errors.Errorf("%v \nRun `evergreen list --projects` to see all valid projects", err)
		}
		return
	}

	// update variants
	if len(p.Variants) == 0 && p.Alias == "" {
		p.Variants = conf.FindDefaultVariants(p.Project)
		if len(p.Variants) == 0 && p.Finalize {
			err = errors.Errorf("Need to specify at least one buildvariant with -v when finalizing." +
				" Run with `-v all` to finalize against all variants.")
			return
		}
	} else if p.Alias == "" {
		defaultVariants := conf.FindDefaultVariants(p.Project)
		if len(defaultVariants) == 0 && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default variants for project '%v'?",
				p.Variants, p.Project), false) {
			conf.SetDefaultVariants(p.Project, p.Variants...)
			if err = conf.Write(""); err != nil {
				fmt.Printf("warning - failed to set default variants: %v\n", err)
			}
		}
	}

	// update tasks
	if len(p.Tasks) == 0 {
		p.Tasks = conf.FindDefaultTasks(p.Project)
		if len(p.Tasks) == 0 && p.Finalize {
			err = errors.Errorf("Need to specify at least one task with -t when finalizing." +
				" Run with `-t all` to finalize against all tasks.")
			return
		}
	} else if p.Alias == "" {
		defaultTasks := conf.FindDefaultTasks(p.Project)
		if len(defaultTasks) == 0 && !p.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default tasks for project '%v'?",
				p.Tasks, p.Project), false) {
			conf.SetDefaultTasks(p.Project, p.Tasks...)
			if err := conf.Write(""); err != nil {
				fmt.Printf("warning - failed to set default tasks: %v\n", err)
			}
		}
	}

	if p.Description == "" && !p.SkipConfirm {
		p.Description = prompt("Enter a description for this patch (optional):")
	}

	return
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
		Now         time.Time
	}{
		Patch:       p,
		ShowSummary: summarize,
		Link:        url,
		Now:         time.Now(),
	})
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// loadGitData inspects the current git working directory and returns a patch and its summary.
// The branch argument is used to determine where to generate the merge base from, and any extra
// arguments supplied are passed directly in as additional args to git diff.
func loadGitData(branch string, extraArgs ...string) (*localDiff, error) {
	// branch@{upstream} refers to the branch that the branch specified by branchname is set to
	// build on top of. This allows automatically detecting a branch based on the correct remote,
	// if the user's repo is a fork, for example.
	// For details see: https://git-scm.com/docs/gitrevisions
	mergeBase, err := gitMergeBase(branch+"@{upstream}", "HEAD")
	if err != nil {
		return nil, errors.Errorf("Error getting merge base: %v", err)
	}
	statArgs := []string{"--stat"}
	if len(extraArgs) > 0 {
		statArgs = append(statArgs, extraArgs...)
	}
	stat, err := gitDiff(mergeBase, statArgs...)
	if err != nil {
		return nil, errors.Errorf("Error getting diff summary: %v", err)
	}
	log, err := gitLog(mergeBase)
	if err != nil {
		return nil, errors.Errorf("git log: %v", err)
	}

	if !util.StringSliceContains(extraArgs, "--binary") {
		extraArgs = append(extraArgs, "--binary")
	}

	patch, err := gitDiff(mergeBase, extraArgs...)
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
func gitDiff(base string, diffArgs ...string) (string, error) {
	args := append([]string{
		"--no-ext-diff",
	}, diffArgs...)
	return gitCmd("diff", base, args...)
}

// getLog runs "git log <base>
func gitLog(base string, logArgs ...string) (string, error) {
	args := append(logArgs, "--oneline")
	return gitCmd("log", fmt.Sprintf("...%v", base), args...)
}

func gitCmd(cmdName, base string, gitArgs ...string) (string, error) {
	args := make([]string, 0, 1+len(gitArgs))
	args = append(args, cmdName)
	if base != "" {
		args = append(args, base)
	}
	args = append(args, gitArgs...)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Errorf("'git %v %v' failed with err %v", base, strings.Join(args, " "), err)
	}
	return string(out), err
}

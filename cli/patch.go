package cli

import (
	"10gen.com/mci/model/patch"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

// This is the template used to render a patch's summary in a human-readable output format.
var patchDisplayTemplate = template.Must(template.New("patch").Parse(`
             ID : {{.Patch.Id.Hex}}
        Created : {{.Now.Sub .Patch.CreateTime}} ago
    Description : {{if .Patch.Description}}{{.Patch.Description}}{{else}}<none>{{end}}
           Link : {{.Link}}
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
	base         string
}

// ListPatchesCommand is used to list a user's existing patches.
type ListPatchesCommand struct {
	GlobalOpts  Options  `no-flag:"true"`
	Variants    []string `short:"v" long:"variants"`
	PatchId     string   `short:"i" description:"show details for only the patch with this ID"`
	ShowSummary bool     `short:"s" long:"show-summary" description:"show a summary of the diff for each patch"`
}

type CancelPatchCommand struct {
	GlobalOpts Options `no-flag:"true"`
	PatchId    string  `short:"i" required description:"id of the patch to modify"`
}

type FinalizePatchCommand struct {
	GlobalOpts Options `no-flag:"true"`
	PatchId    string  `short:"i" required description:"id of the patch to modify"`
}

// PatchCommand is used to submit a new patch to the API server.
type PatchCommand struct {
	GlobalOpts  Options  `no-flag:"true"`
	Project     string   `short:"p" long:"project" description:"project to submit patch for"`
	Variants    []string `short:"v" long:"variants"`
	SkipConfirm bool     `short:"y" long:"yes" description:"skip confirmation text"`
	Description string   `short:"d" long:"description" description:"description of patch (optional)"`
	Finalize    bool     `short:"f" long:"finalize" description:"schedule tasks immediately"`
}

// SetModuleCommand adds or updates a module in an existing patch.
type SetModuleCommand struct {
	GlobalOpts  Options `no-flag:"true"`
	Module      string  `short:"m" long:"module" description:"name of the module to remove from patch"`
	PatchId     string  `short:"i" required description:"id of the patch to modify"`
	SkipConfirm bool    `short:"y" long:"yes" description:"skip confirmation text"`
}

// RemoveModuleCommand removes module information from an existing patch.
type RemoveModuleCommand struct {
	GlobalOpts Options `no-flag:"true"`
	Module     string  `short:"m" required long:"module" description:"name of the module to remove from patch"`
	PatchId    string  `short:"i" required description:"name of the module to remove from patch"`
}

// getAPIClient loads the user settings and creates an APIClient configured for the API/UI
// endpoints defined in the settings file.
func getAPIClient(o Options) (*APIClient, *Settings, error) {
	settings, err := loadSettings(o)
	if err != nil {
		return nil, nil, err
	}
	ac := &APIClient{APIRoot: settings.APIServerHost, User: settings.User, APIKey: settings.APIKey}
	return ac, settings, nil
}

func (lpc *ListPatchesCommand) Execute(args []string) error {
	ac, settings, err := getAPIClient(lpc.GlobalOpts)
	if err != nil {
		return err
	}
	patches, err := ac.GetPatches()
	if err != nil {
		return err
	}
	for _, p := range patches {
		disp, err := getPatchDisplay(&p, lpc.ShowSummary, settings.UIServerHost)
		if err != nil {
			return err
		}
		fmt.Println(disp)
	}
	return nil
}

// getPatchDisplay returns a human-readable summary representation of a patch object
// which can be written to the terminal.
func getPatchDisplay(p *patch.Patch, summarize bool, uiHost string) (string, error) {
	var out bytes.Buffer

	err := patchDisplayTemplate.Execute(&out, struct {
		Patch       *patch.Patch
		ShowSummary bool
		Link        string
		Now         time.Time
	}{p, summarize, uiHost + "patch/" + p.Id.Hex(), time.Now()})
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func (rmc *RemoveModuleCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(rmc.GlobalOpts)
	if err != nil {
		return err
	}
	err = ac.DeletePatchModule(rmc.PatchId, rmc.Module)
	if err != nil {
		return err
	}
	fmt.Println("Module removed.")
	return nil
}

func (smc *SetModuleCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(smc.GlobalOpts)
	if err != nil {
		return err
	}
	diffData, err := loadGitData("master", args...) // modules always diff relative to master. this is not great.
	if err != nil {
		return err
	}
	if !smc.SkipConfirm {
		fmt.Println(diffData.patchSummary)
		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil
		}
	}

	err = ac.UpdatePatchModule(smc.PatchId, smc.Module, diffData.fullPatch, diffData.base)
	if err != nil {
		return err
	}
	fmt.Println("Module updated.")
	return nil
}

func (pc *PatchCommand) Execute(args []string) error {
	ac, settings, err := getAPIClient(pc.GlobalOpts)
	if err != nil {
		return err
	}
	if pc.Project == "" {
		pc.Project = settings.FindDefaultProject()
	} else {
		if settings.FindDefaultProject() == "" &&
			!pc.SkipConfirm && confirm(fmt.Sprintf("Make %v your default project?", pc.Project), true) {
			settings.SetDefaultProject(pc.Project)
			if err = settings.Write(pc.GlobalOpts); err != nil {
				fmt.Println("warning - failed to set default project: %v", err)
			}
		}
	}
	if pc.Project == "" {
		return fmt.Errorf("Need to specify a project.")
	}
	ref, err := ac.GetProjectRef(pc.Project)
	if err != nil {
		return err
	}
	if pc.Description == "" && !pc.SkipConfirm {
		pc.Description = prompt("Enter a description for this patch (optional):")
	}
	diffData, err := loadGitData(ref.Branch, args...)
	if err != nil {
		return err
	}
	if !pc.SkipConfirm {
		fmt.Println(diffData.patchSummary)
		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil
		}
	}

	newPatch, err := ac.PutPatch(pc.Project, diffData.fullPatch, pc.Description, diffData.base, "all", pc.Finalize)
	if err != nil {
		return err
	}
	patchDisp, err := getPatchDisplay(newPatch, true, settings.UIServerHost)
	if err != nil {
		return err
	}
	fmt.Println("Patch successfully created.")
	fmt.Print(patchDisp)
	return nil
}

func (cpc *CancelPatchCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(cpc.GlobalOpts)
	if err != nil {
		return err
	}
	err = ac.CancelPatch(cpc.PatchId)
	if err != nil {
		return err
	}
	fmt.Println("Patch canceled.")
	return nil
}

func (fpc *FinalizePatchCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(fpc.GlobalOpts)
	if err != nil {
		return err
	}
	err = ac.FinalizePatch(fpc.PatchId)
	if err != nil {
		return err
	}
	fmt.Println("Patch finalized.")
	return nil
}

// loadGitData inspects the current git working directory and returns a patch and its summary.
// The branch argument is used to determine where to generate the merge base from, and any extra
// arguments supplied are passed directly in as additional args to git diff.
func loadGitData(branch string, extraArgs ...string) (*localDiff, error) {
	mergeBase, err := gitMergeBase("origin/"+branch, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("Error getting merge base: %v", err)
	}
	statArgs := []string{"--stat"}
	if len(extraArgs) > 0 {
		statArgs = append(statArgs, extraArgs...)
	}
	stat, err := gitDiff("", statArgs...)
	if err != nil {
		return nil, fmt.Errorf("Error getting diff summary: %v", err)
	}

	patch, err := gitDiff(mergeBase, extraArgs...)
	if err != nil {
		return nil, fmt.Errorf("Error getting patch: %v", err)
	}
	return &localDiff{patch, stat, mergeBase}, nil
}

// gitMergeBase runs "git merge-base <branch1> <branch2>" and returns the
// resulting githash as string
func gitMergeBase(branch1, branch2 string) (string, error) {
	cmd := exec.Command("git", "merge-base", branch1, branch2)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("'git merge-base %v %v' failed: %v", branch1, branch2, err)
	}
	return strings.TrimSpace(string(out)), err
}

// gitDiff runs "git diff <base> <diffargs ...>" and returns the output of the command as a string
func gitDiff(base string, diffArgs ...string) (string, error) {
	args := make([]string, 0, 2+len(diffArgs))
	args = append(args, "diff")
	if base != "" {
		args = append(args, base)
	}
	args = append(args, diffArgs...)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("'git %v' failed: %v", base, strings.Join(args, " "), err)
	}
	return string(out), err
}

package cli

import (
	"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"
)

// Above this size, the user must explicitly use --large to submit the patch (or confirm)
const largePatchThreshold = 1024 * 1024 * 16

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

type patchSubmission struct {
	projectId   string
	patchData   string
	description string
	base        string
	variants    string
	tasks       []string
	finalize    bool
}

// ListPatchesCommand is used to list a user's existing patches.
type ListPatchesCommand struct {
	GlobalOpts  *Options `no-flag:"true"`
	Variants    []string `short:"v" long:"variants" description:"variants to run the patch on. may be specified multiple times, or use the value 'all'"`
	PatchId     string   `short:"i" description:"show details for only the patch with this ID"`
	ShowSummary bool     `short:"s" long:"show-summary" description:"show a summary of the diff for each patch"`
}

type ListProjectsCommand struct {
	GlobalOpts *Options `no-flag:"true"`
}

// ValidateCommand is used to verify that a config file is valid.
type ValidateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
}

// CancelPatchCommand is used to cancel a patch.
type CancelPatchCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	PatchId    string   `short:"i" description:"id of the patch to modify" required:"true"`
}

// FinalizePatchCommand is used to finalize a patch, allowing it to be scheduled.
type FinalizePatchCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	PatchId    string   `short:"i" description:"id of the patch to modify" required:"true"`
}

// PatchCommand is used to submit a new patch to the API server.
type PatchCommand struct {
	PatchCommandParams
}

// PatchFileCommand is used to submit a new patch to the API server using a diff file.
type PatchFileCommand struct {
	PatchCommandParams
	DiffFile string `long:"diff-file" description:"file containing the diff for the patch"`
	Base     string `short:"b" long:"base" description:"githash of base"`
}

// PatchCommandParams contains parameters common to PatchCommand and PatchFileCommand
type PatchCommandParams struct {
	GlobalOpts  *Options `no-flag:"true"`
	Project     string   `short:"p" long:"project" description:"project to submit patch for"`
	Variants    []string `short:"v" long:"variants"`
	Tasks       []string `short:"t" long:"tasks"`
	SkipConfirm bool     `short:"y" long:"yes" description:"skip confirmation text"`
	Description string   `short:"d" long:"description" description:"description of patch (optional)"`
	Finalize    bool     `short:"f" long:"finalize" description:"schedule tasks immediately"`
	Large       bool     `long:"large" description:"enable submitting larger patches (>16MB)"`
}

// SetModuleCommand adds or updates a module in an existing patch.
type SetModuleCommand struct {
	GlobalOpts  *Options `no-flag:"true"`
	Module      string   `short:"m" long:"module" description:"name of the module to set patch for"`
	PatchId     string   `short:"i" description:"id of the patch to modify" required:"true" `
	SkipConfirm bool     `short:"y" long:"yes" description:"skip confirmation text"`
	Large       bool     `long:"large" description:"enable submitting larger patches (>16MB)"`
}

// RemoveModuleCommand removes module information from an existing patch.
type RemoveModuleCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Module     string   `short:"m" long:"module" description:"name of the module to remove from patch" required:"true" `
	PatchId    string   `short:"i" description:"name of the module to remove from patch" required:"true" `
}

// getAPIClient loads the user settings and creates an APIClient configured for the API/UI
// endpoints defined in the settings file.
func getAPIClient(o *Options) (*APIClient, *Settings, error) {
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
	notifyUserUpdate(ac)

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
	}{p, summarize, uiHost + "/patch/" + p.Id.Hex(), time.Now()})
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
	notifyUserUpdate(ac)

	err = ac.DeletePatchModule(rmc.PatchId, rmc.Module)
	if err != nil {
		return err
	}
	fmt.Println("Module removed.")
	return nil
}

func (vc *ValidateCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("must supply path to a file to validate.")
	}
	ac, _, err := getAPIClient(vc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

	confFile, err := ioutil.ReadFile(args[0])
	if err != nil {
		return err
	}
	projErrors, err := ac.ValidateLocalConfig(confFile)
	if err != nil {
		return nil
	}
	if len(projErrors) > 0 {
		fmt.Println("Project has", len(projErrors), "error(s)")
		for i, e := range projErrors {
			fmt.Printf("%v) %v\n\n", i+1, e.Message)
		}
		return fmt.Errorf("Invalid project file!")
	}
	fmt.Println("Valid!")
	return nil
}

func (smc *SetModuleCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(smc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

	diffData, err := loadGitData("master", args...) // modules always diff relative to master. this is not great.
	if err != nil {
		return err
	}

	if err := validatePatchSize(diffData, smc.Large); err != nil {
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
	ac, settings, ref, err := validatePatchCommand(&pc.PatchCommandParams)
	if err != nil {
		return err
	}

	diffData, err := loadGitData(ref.Branch, args...)
	if err != nil {
		return err
	}

	return createPatch(pc.PatchCommandParams, ac, settings, diffData)
}

func (pfc *PatchFileCommand) Execute(args []string) error {
	ac, settings, _, err := validatePatchCommand(&pfc.PatchCommandParams)
	if err != nil {
		return err
	}

	fullPatch, err := ioutil.ReadFile(pfc.DiffFile)
	if err != nil {
		return fmt.Errorf("Error reading diff file: %v", err)
	}
	diffData := &localDiff{string(fullPatch), "", pfc.Base}

	return createPatch(pfc.PatchCommandParams, ac, settings, diffData)
}

func (cpc *CancelPatchCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(cpc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

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
	notifyUserUpdate(ac)

	err = ac.FinalizePatch(fpc.PatchId)
	if err != nil {
		return err
	}
	fmt.Println("Patch finalized.")
	return nil
}

func (lp *ListProjectsCommand) Execute(args []string) error {
	ac, _, err := getAPIClient(lp.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

	projs, err := ac.ListProjects()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(projs))
	names := make(map[string]string)
	for _, proj := range projs {
		// Only list projects that are enabled
		if proj.Enabled {
			ids = append(ids, proj.Identifier)
			names[proj.Identifier] = proj.DisplayName
		}
	}
	sort.Strings(ids)
	fmt.Println(len(ids), "projects:")
	w := new(tabwriter.Writer)
	// Format in tab-separated columns with a tab stop of 8.
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	for _, id := range ids {
		line := fmt.Sprintf("\t%v\t", id)
		if len(names[id]) > 0 && names[id] != id {
			line = line + fmt.Sprintf("(%v)", names[id])
		}
		fmt.Fprintln(w, line)
	}
	w.Flush()
	return nil
}

// Performs validation for patch or patch-file
func validatePatchCommand(params *PatchCommandParams) (ac *APIClient, settings *Settings, ref *model.ProjectRef, err error) {
	ac, settings, err = getAPIClient(params.GlobalOpts)
	if err != nil {
		return
	}
	notifyUserUpdate(ac)

	if params.Project == "" {
		params.Project = settings.FindDefaultProject()
	} else {
		if settings.FindDefaultProject() == "" &&
			!params.SkipConfirm && confirm(fmt.Sprintf("Make %v your default project?", params.Project), true) {
			settings.SetDefaultProject(params.Project)
			if err := settings.Write(params.GlobalOpts); err != nil {
				fmt.Println("warning - failed to set default project: %v", err)
			}
		}
	}

	if params.Project == "" {
		err = fmt.Errorf("Need to specify a project.")
		return
	}

	ref, err = ac.GetProjectRef(params.Project)
	if err != nil {
		return
	}

	// update variants
	if len(params.Variants) == 0 {
		params.Variants = settings.FindDefaultVariants(params.Project)
		if len(params.Variants) == 0 && params.Finalize {
			err = fmt.Errorf("Need to specify at least one buildvariant with -v when finalizing." +
				" Run with `-v all` to finalize against all variants.")
			return
		}
	} else {
		defaultVariants := settings.FindDefaultVariants(params.Project)
		if len(defaultVariants) == 0 && !params.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default variants for project '%v'?",
				params.Variants, params.Project), false) {
			settings.SetDefaultVariants(params.Project, params.Variants...)
			if err := settings.Write(params.GlobalOpts); err != nil {
				fmt.Println("warning - failed to set default variants: %v", err)
			}
		}
	}

	// update tasks
	if len(params.Tasks) == 0 {
		params.Tasks = settings.FindDefaultTasks(params.Project)
		if len(params.Tasks) == 0 && params.Finalize {
			err = fmt.Errorf("Need to specify at least one task with -t when finalizing." +
				" Run with `-t all` to finalize against all tasks.")
			return
		}
	} else {
		defaultTasks := settings.FindDefaultTasks(params.Project)
		if len(defaultTasks) == 0 && !params.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default tasks for project '%v'?",
				params.Tasks, params.Project), false) {
			settings.SetDefaultTasks(params.Project, params.Tasks...)
			if err := settings.Write(params.GlobalOpts); err != nil {
				fmt.Println("warning - failed to set default tasks: %v", err)
			}
		}
	}

	if params.Description == "" && !params.SkipConfirm {
		params.Description = prompt("Enter a description for this patch (optional):")
	}

	return
}

// Creates a patch using diffData
func createPatch(params PatchCommandParams, ac *APIClient, settings *Settings, diffData *localDiff) error {
	if err := validatePatchSize(diffData, params.Large); err != nil {
		return err
	}
	if !params.SkipConfirm && diffData.patchSummary != "" {
		fmt.Println(diffData.patchSummary)
		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil
		}
	}

	variantsStr := strings.Join(params.Variants, ",")
	patchSub := patchSubmission{
		params.Project, diffData.fullPatch, params.Description,
		diffData.base, variantsStr, params.Tasks, params.Finalize,
	}

	newPatch, err := ac.PutPatch(patchSub)
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

// Returns an error if the diff is greater than the system limit, or if it's above the large
// patch threhsold and allowLarge is not set.
func validatePatchSize(diff *localDiff, allowLarge bool) error {
	patchLen := len(diff.fullPatch)
	if patchLen > patch.SizeLimit {
		return fmt.Errorf("Patch is greater than the system limit (%v > %v bytes).", patchLen, patch.SizeLimit)
	} else if patchLen > largePatchThreshold && !allowLarge {
		return fmt.Errorf("Patch is larger than the default threshold (%v > %v bytes).\n"+
			"To allow submitting this patch, use the --large flag.", patchLen, largePatchThreshold)
	}

	// Patch is small enough and/or allowLarge is true, so no error
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
	stat, err := gitDiff(mergeBase, statArgs...)
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

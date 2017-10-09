package cli

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var noProjectError = errors.New("must specify a project with -p/--project or a path to a config file with -f/--file")

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

// lastGreenTemplate helps return readable information for the last-green command.
var lastGreenTemplate = template.Must(template.New("last_green").Parse(`
   Revision : {{.Version.Revision}}
    Message : {{.Version.Message}}
       Link : {{.UIURL}}/version/{{.Version.Id}}

`))

var defaultPatchesReturned = 5

type localDiff struct {
	fullPatch    string
	patchSummary string
	log          string
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
	Number      *int     `short:"n" long:"number" description:"number of patches to show (0 for all patches)"`
}

type ListCommand struct {
	GlobalOpts           *Options `no-flag:"true"`
	Project              string   `short:"p" long:"project" description:"project whose variants or tasks should be listed (use with --variants/--tasks)"`
	File                 string   `short:"f" long:"file" description:"path to config file whose variants or tasks should be listed (use with --variants/--tasks)"`
	Projects             bool     `long:"projects" description:"list all available projects"`
	Variants             bool     `long:"variants" description:"list all variants for a project"`
	Tasks                bool     `long:"tasks" description:"list all tasks for a project"`
	Distros              bool     `long:"distros" description:"list all distros for a project"`
	UserSpawnableDistros bool     `long:"spawnable" description:"list all spawnable distros for a project"`
}

// ValidateCommand is used to verify that a config file is valid.
type ValidateCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Positional struct {
		FileName string `positional-arg-name:"filename" description:"path to an evergreen project file"`
	} `positional-args:"1" required:"yes"`
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
	Large       bool     `short:"l" long:"large" description:"enable submitting larger patches (>16MB)"`
	ShowSummary bool     `long:"verbose" description:"hide patch summary"`
}

// LastGreenCommand contains parameters for the finding a project's most recent passing version.
type LastGreenCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Project    string   `short:"p" long:"project" description:"project to search" required:"true"`
	Variants   []string `short:"v" long:"variants" description:"variant that must be passing" required:"true"`
}

// SetModuleCommand adds or updates a module in an existing patch.
type SetModuleCommand struct {
	GlobalOpts  *Options `no-flag:"true"`
	Module      string   `short:"m" long:"module" description:"name of the module to set patch for"`
	PatchId     string   `short:"i" description:"id of the patch to modify" required:"true" `
	Project     string   `short:"p" long:"project" description:"project name"`
	SkipConfirm bool     `short:"y" long:"yes" description:"skip confirmation text"`
	Large       bool     `long:"large" description:"enable submitting larger patches (>16MB)"`
}

// RemoveModuleCommand removes module information from an existing patch.
type RemoveModuleCommand struct {
	GlobalOpts *Options `no-flag:"true"`
	Module     string   `short:"m" long:"module" description:"name of the module to remove from patch" required:"true" `
	PatchId    string   `short:"i" description:"name of the module to remove from patch" required:"true" `
}

func (lpc *ListPatchesCommand) Execute(_ []string) error {
	ctx := context.Background()
	ac, _, settings, err := getAPIClients(ctx, lpc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)
	if lpc.Number == nil {
		lpc.Number = &defaultPatchesReturned
	}
	patches, err := ac.GetPatches(*lpc.Number)
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

func (rmc *RemoveModuleCommand) Execute(_ []string) error {
	ctx := context.Background()
	ac, _, _, err := getAPIClients(ctx, rmc.GlobalOpts)
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

func (vc *ValidateCommand) Execute(_ []string) error {
	if vc.Positional.FileName == "" {
		return errors.New("must supply path to a file to validate.")
	}

	ctx := context.Background()
	ac, _, _, err := getAPIClients(ctx, vc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

	confFile, err := ioutil.ReadFile(vc.Positional.FileName)
	if err != nil {
		return err
	}
	projErrors, err := ac.ValidateLocalConfig(confFile)
	if err != nil {
		return nil
	}
	numErrors, numWarnings := 0, 0
	if len(projErrors) > 0 {
		for i, e := range projErrors {
			if e.Level == validator.Warning {
				numWarnings++
			} else if e.Level == validator.Error {
				numErrors++
			}
			fmt.Printf("%v) %v: %v\n\n", i+1, e.Level, e.Message)
		}

		return errors.Errorf("Project file has %d warnings, %d errors.", numWarnings, numErrors)
	}
	fmt.Println("Valid!")
	return nil
}

// getModuleBranch returns the branch for the config.
func getModuleBranch(moduleName string, proj *model.Project) (string, error) {
	// find the module of the patch
	for _, module := range proj.Modules {
		if module.Name == moduleName {
			return module.Branch, nil
		}
	}
	return "", errors.Errorf("module '%s' unknown or not found", moduleName)
}

func (smc *SetModuleCommand) Execute(args []string) error {
	ctx := context.Background()
	ac, rc, _, err := getAPIClients(ctx, smc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

	proj, err := rc.GetPatchedConfig(smc.PatchId)
	if err != nil {
		return err
	}

	moduleBranch, err := getModuleBranch(smc.Module, proj)
	if err != nil {
		grip.Error(err)
		mods, merr := ac.GetPatchModules(smc.PatchId, proj.Identifier)
		if merr != nil {
			return errors.Wrap(merr, "errors fetching list of available modules")
		}

		if len(mods) != 0 {
			grip.Noticef("known modules includes:\n\t%s", strings.Join(mods, "\n\t"))
		}

		return errors.Errorf("could not set specified module: \"%s\"", smc.Module)
	}

	// diff against the module branch.
	diffData, err := loadGitData(moduleBranch, args...)
	if err != nil {
		return err
	}
	if err = validatePatchSize(diffData, smc.Large); err != nil {
		return err
	}

	if !smc.SkipConfirm {
		fmt.Printf("Using branch %v for module %v \n", moduleBranch, smc.Module)
		if diffData.patchSummary != "" {
			fmt.Println(diffData.patchSummary)
		}

		if !confirm("This is a summary of the patch to be submitted. Continue? (y/n):", true) {
			return nil
		}
	}

	err = ac.UpdatePatchModule(smc.PatchId, smc.Module, diffData.fullPatch, diffData.base)
	if err != nil {
		mods, err := ac.GetPatchModules(smc.PatchId, smc.Project)
		var msg string
		if err != nil {
			msg = fmt.Sprintf("could not find module named %s or retrieve list of modules",
				smc.Module)
		} else if len(mods) == 0 {
			msg = fmt.Sprintf("could not find modules for this project. %s is not a module. "+
				"see the evergreen configuration file for module configuration.",
				smc.Module)
		} else {
			msg = fmt.Sprintf("could not find module named '%s', select correct module from:\n\t%s",
				smc.Module, strings.Join(mods, "\n\t"))
		}
		grip.Error(msg)
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

func (pfc *PatchFileCommand) Execute(_ []string) error {
	ac, settings, _, err := validatePatchCommand(&pfc.PatchCommandParams)
	if err != nil {
		return err
	}

	fullPatch, err := ioutil.ReadFile(pfc.DiffFile)
	if err != nil {
		return errors.Errorf("Error reading diff file: %v", err)
	}
	diffData := &localDiff{string(fullPatch), "", "", pfc.Base}

	return createPatch(pfc.PatchCommandParams, ac, settings, diffData)
}

func (cpc *CancelPatchCommand) Execute(_ []string) error {
	ctx := context.Background()
	ac, _, _, err := getAPIClients(ctx, cpc.GlobalOpts)
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

func (fpc *FinalizePatchCommand) Execute(_ []string) error {
	ctx := context.Background()
	ac, _, _, err := getAPIClients(ctx, fpc.GlobalOpts)
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

func (lgc *LastGreenCommand) Execute(_ []string) error {
	ctx := context.Background()
	ac, rc, settings, err := getAPIClients(ctx, lgc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)
	v, err := rc.GetLastGreen(lgc.Project, lgc.Variants)
	if err != nil {
		return err
	}
	return lastGreenTemplate.Execute(os.Stdout, struct {
		Version *version.Version
		UIURL   string
	}{v, settings.UIServerHost})
}

func (lc *ListCommand) Execute(_ []string) error {
	// stop the user from using > 1 type flag
	opts := []bool{lc.Projects, lc.Variants, lc.Tasks, lc.Distros, lc.UserSpawnableDistros}
	var numOpts int
	for _, opt := range opts {
		if opt {
			numOpts++
		}
	}
	if numOpts != 1 {
		return errors.Errorf("must specify one and only one of --projects, --variants, --tasks, --distros, or --spawnable")
	}

	if lc.Projects {
		return lc.listProjects()
	}
	if lc.Tasks {
		return lc.listTasks()
	}
	if lc.Variants {
		return lc.listVariants()
	}
	if lc.Distros || lc.UserSpawnableDistros {
		return lc.listDistros(lc.UserSpawnableDistros)
	}
	return errors.Errorf("this code should not be reachable")
}

func (lc *ListCommand) listProjects() error {
	ctx := context.Background()
	ac, _, _, err := getAPIClients(ctx, lc.GlobalOpts)
	if err != nil {
		return errors.WithStack(err)
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
			line = line + fmt.Sprintf("%v", names[id])
		}
		fmt.Fprintln(w, line)
	}
	return errors.WithStack(w.Flush())
}

func (lc *ListCommand) listDistros(onlyUserSpawnable bool) error {
	ctx := context.Background()
	client, _, client_err := getAPIV2Client(ctx, lc.GlobalOpts)
	if client_err != nil {
		return errors.WithStack(client_err)
	}

	distros, req_err := client.GetDistrosList(ctx)
	if req_err != nil {
		return errors.WithStack(req_err)
	}

	if onlyUserSpawnable {
		spawnableDistros := []restmodel.APIDistro{}
		for _, distro := range distros {
			if distro.UserSpawnAllowed {
				spawnableDistros = append(spawnableDistros, distro)
			}
		}

		fmt.Println(len(spawnableDistros), "spawnable distros:")
		for _, distro := range spawnableDistros {
			fmt.Println(distro.Name)
		}

	} else {
		fmt.Println(len(distros), "distros:")
		for _, distro := range distros {
			fmt.Println(distro.Name)
		}
	}

	return nil
}

// LoadLocalConfig loads the local project config into a project
func loadLocalConfig(filepath string) (*model.Project, error) {
	configBytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading project config")
	}

	project := &model.Project{}
	err = model.LoadProjectInto(configBytes, "", project)
	if err != nil {
		return nil, errors.Wrap(err, "error loading project")
	}

	return project, nil
}

func (lc *ListCommand) listTasks() error {
	var tasks []model.ProjectTask
	ctx := context.Background()
	if lc.Project != "" {
		ac, _, _, err := getAPIClients(ctx, lc.GlobalOpts)
		if err != nil {
			return err
		}
		notifyUserUpdate(ac)
		tasks, err = ac.ListTasks(lc.Project)
		if err != nil {
			return err
		}
	} else if lc.File != "" {
		project, err := loadLocalConfig(lc.File)
		if err != nil {
			return err
		}
		tasks = project.Tasks
	} else {
		return noProjectError
	}
	fmt.Println(len(tasks), "tasks:")
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	for _, t := range tasks {
		line := fmt.Sprintf("\t%v\t", t.Name)
		fmt.Fprintln(w, line)
	}

	return w.Flush()
}

func (lc *ListCommand) listVariants() error {
	var variants []model.BuildVariant
	ctx := context.Background()
	if lc.Project != "" {
		ac, _, _, err := getAPIClients(ctx, lc.GlobalOpts)
		if err != nil {
			return err
		}
		notifyUserUpdate(ac)
		variants, err = ac.ListVariants(lc.Project)
		if err != nil {
			return err
		}
	} else if lc.File != "" {
		project, err := loadLocalConfig(lc.File)
		if err != nil {
			return err
		}
		variants = project.BuildVariants
	} else {
		return noProjectError
	}

	names := make([]string, 0, len(variants))
	displayNames := make(map[string]string)
	for _, variant := range variants {
		names = append(names, variant.Name)
		displayNames[variant.Name] = variant.DisplayName
	}
	sort.Strings(names)
	fmt.Println(len(names), "variants:")
	w := new(tabwriter.Writer)
	// Format in tab-separated columns with a tab stop of 8.
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	for _, name := range names {
		line := fmt.Sprintf("\t%v\t", name)
		if len(displayNames[name]) > 0 && displayNames[name] != name {
			line = line + fmt.Sprintf("%v", displayNames[name])
		}
		fmt.Fprintln(w, line)
	}

	return w.Flush()
}

// Performs validation for patch or patch-file
func validatePatchCommand(params *PatchCommandParams) (ac *APIClient, settings *model.CLISettings, ref *model.ProjectRef, err error) {
	ctx := context.Background()
	ac, _, settings, err = getAPIClients(ctx, params.GlobalOpts)
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
			if err = WriteSettings(settings, params.GlobalOpts); err != nil {
				fmt.Printf("warning - failed to set default project: %v\n", err)
			}
		}
	}

	if params.Project == "" {
		err = errors.Errorf("Need to specify a project.")
		return
	}

	ref, err = ac.GetProjectRef(params.Project)
	if err != nil {
		if apiErr, ok := err.(APIError); ok && apiErr.code == http.StatusNotFound {
			err = errors.Errorf("%v \nRun `evergreen list --projects` to see all valid projects", err)
		}
		return
	}

	// update variants
	if len(params.Variants) == 0 {
		params.Variants = settings.FindDefaultVariants(params.Project)
		if len(params.Variants) == 0 && params.Finalize {
			err = errors.Errorf("Need to specify at least one buildvariant with -v when finalizing." +
				" Run with `-v all` to finalize against all variants.")
			return
		}
	} else {
		defaultVariants := settings.FindDefaultVariants(params.Project)
		if len(defaultVariants) == 0 && !params.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default variants for project '%v'?",
				params.Variants, params.Project), false) {
			settings.SetDefaultVariants(params.Project, params.Variants...)
			if err = WriteSettings(settings, params.GlobalOpts); err != nil {
				fmt.Printf("warning - failed to set default variants: %v\n", err)
			}
		}
	}

	// update tasks
	if len(params.Tasks) == 0 {
		params.Tasks = settings.FindDefaultTasks(params.Project)
		if len(params.Tasks) == 0 && params.Finalize {
			err = errors.Errorf("Need to specify at least one task with -t when finalizing." +
				" Run with `-t all` to finalize against all tasks.")
			return
		}
	} else {
		defaultTasks := settings.FindDefaultTasks(params.Project)
		if len(defaultTasks) == 0 && !params.SkipConfirm &&
			confirm(fmt.Sprintf("Set %v as the default tasks for project '%v'?",
				params.Tasks, params.Project), false) {
			settings.SetDefaultTasks(params.Project, params.Tasks...)
			if err := WriteSettings(settings, params.GlobalOpts); err != nil {
				fmt.Printf("warning - failed to set default tasks: %v\n", err)
			}
		}
	}

	if params.Description == "" && !params.SkipConfirm {
		params.Description = prompt("Enter a description for this patch (optional):")
	}

	return
}

// Creates a patch using diffData
func createPatch(params PatchCommandParams, ac *APIClient, settings *model.CLISettings, diffData *localDiff) error {
	if err := validatePatchSize(diffData, params.Large); err != nil {
		return err
	}
	if !params.SkipConfirm && len(diffData.fullPatch) == 0 {
		if !confirm("Patch submission is empty. Continue?(y/n)", true) {
			return nil
		}
	} else if !params.SkipConfirm && diffData.patchSummary != "" {
		fmt.Println(diffData.patchSummary)
		if diffData.log != "" {
			fmt.Println(diffData.log)
		}

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
	patchDisp, err := getPatchDisplay(newPatch, params.ShowSummary, settings.UIServerHost)
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
		return errors.Errorf("Patch is greater than the system limit (%v > %v bytes).", patchLen, patch.SizeLimit)
	} else if patchLen > largePatchThreshold && !allowLarge {
		return errors.Errorf("Patch is larger than the default threshold (%v > %v bytes).\n"+
			"To allow submitting this patch, use the --large flag.", patchLen, largePatchThreshold)
	}

	// Patch is small enough and/or allowLarge is true, so no error
	return nil
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
	args := make([]string, 0, 1+len(diffArgs))
	args = append(args, "--no-ext-diff")
	args = append(args, diffArgs...)
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

package model

import (
	"context"
	"slices"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/attribute"
)

const (
	maxGeneratedBuildVariants = 200
	maxGeneratedTasks         = 25000

	numGenerateTaskBVAttribute           = "evergreen.generate_tasks.num_build_variants"
	numGenerateTasksAttribute            = "evergreen.generate_tasks.num_created"
	numActivatedGenerateTasksAttribute   = "evergreen.generate_tasks.num_activated"
	skippingGenerateTasksAttribute       = "evergreen.generate_tasks.skipping"
	firstGenerateTasksOnVersionAttribute = "evergreen.generate_tasks.first_generate_tasks_on_version"
)

var DependencyCycleError = errors.New("adding dependencies creates a dependency cycle")

// GeneratedProject is a subset of the Project type, and is generated from the
// JSON from a `generate.tasks` command.
type GeneratedProject struct {
	BuildVariants []parserBV                 `yaml:"buildvariants"`
	Tasks         []parserTask               `yaml:"tasks"`
	Functions     map[string]*YAMLCommandSet `yaml:"functions"`
	TaskGroups    []parserTaskGroup          `yaml:"task_groups"`

	// Task is the task that is running generate.tasks.
	Task           *task.Task
	ActivationInfo *specificActivationInfo
	NewTVPairs     *TaskVariantPairs
}

// MergeGeneratedProjects takes a slice of generated projects and returns a single, deduplicated project.
func MergeGeneratedProjects(ctx context.Context, projects []GeneratedProject) (*GeneratedProject, error) {
	_, span := tracer.Start(ctx, "merge-generated-projects")
	defer span.End()
	catcher := grip.NewBasicCatcher()

	bvs := map[string]*parserBV{}
	tasks := map[string]*parserTask{}
	functions := map[string]*YAMLCommandSet{}
	taskGroups := map[string]*parserTaskGroup{}

	for _, p := range projects {
	mergeBuildVariants:
		for i, bv := range p.BuildVariants {
			if len(bv.Tasks) == 0 {
				if _, ok := bvs[bv.Name]; ok {
					catcher.Errorf("found duplicate buildvariant '%s'", bv.Name)
				} else {
					bvs[bv.Name] = &p.BuildVariants[i]
					continue mergeBuildVariants
				}
			}
			if _, ok := bvs[bv.Name]; ok {
				bvs[bv.Name].Tasks = append(bvs[bv.Name].Tasks, bv.Tasks...)
				bvs[bv.Name].DisplayTasks = append(bvs[bv.Name].DisplayTasks, bv.DisplayTasks...)
			}
			bvs[bv.Name] = &p.BuildVariants[i]
		}
		for i, t := range p.Tasks {
			if _, ok := tasks[t.Name]; ok {
				catcher.Errorf("found duplicate task '%s'", t.Name)
			} else {
				tasks[t.Name] = &p.Tasks[i]
			}
		}
		for f, val := range p.Functions {
			if _, ok := functions[f]; ok {
				catcher.Errorf("found duplicate function '%s'", f)
			}
			functions[f] = val
		}
		for i, tg := range p.TaskGroups {
			if _, ok := taskGroups[tg.Name]; ok {
				catcher.Errorf("found duplicate task group '%s'", tg.Name)
			} else {
				taskGroups[tg.Name] = &p.TaskGroups[i]
			}
		}
	}

	g := &GeneratedProject{}
	for i := range bvs {
		g.BuildVariants = append(g.BuildVariants, *bvs[i])
	}
	for i := range tasks {
		g.Tasks = append(g.Tasks, *tasks[i])
	}
	g.Functions = functions
	for i := range taskGroups {
		g.TaskGroups = append(g.TaskGroups, *taskGroups[i])
	}
	return g, catcher.Resolve()
}

// ParseProjectFromJSON returns a GeneratedTasks type from JSON. We use the
// YAML parser instead of the JSON parser because the JSON parser will not
// properly unmarshal into a struct with multiple fields as options, like the YAMLCommandSet.
func ParseProjectFromJSONString(data string) (GeneratedProject, error) {
	g := GeneratedProject{}
	dataAsJSON := []byte(data)
	if err := util.UnmarshalYAMLWithFallback(dataAsJSON, &g); err != nil {
		return g, errors.Wrap(err, "unmarshalling generated project from YAML data")
	}
	return g, nil
}

// NewVersion adds the buildvariants, tasks, and functions
// from a generated project config to a project, and returns the previous config number.
func (g *GeneratedProject) NewVersion(ctx context.Context, p *Project, pp *ParserProject, v *Version) (*Project, *ParserProject, *Version, error) {
	_, span := tracer.Start(ctx, "create-generated-version")
	defer span.End()
	// Cache project data in maps for quick lookup
	cachedProject := cacheProjectData(p)

	// We've updated the parser project in a previous iteration of the generator job, so we don't try to update.
	if utility.StringSliceContains(pp.UpdatedByGenerators, g.Task.Id) {
		return p, pp, v, nil
	}
	// Validate generated project against original project.
	if err := g.validateGeneratedProject(cachedProject); err != nil {
		// Return version in this error case for handleError, which checks for a race. We only need to do this in cases where there is a validation check.
		return nil, pp, v, errors.Wrap(err, "generated project is invalid")
	}

	newPP, err := g.addGeneratedProjectToConfig(pp, cachedProject)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "creating config from generated config")
	}
	newPP.Id = v.Id
	p, err = TranslateProject(newPP)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, TranslateProjectError)
	}
	return p, newPP, v, nil
}

func (g *GeneratedProject) Save(ctx context.Context, settings *evergreen.Settings, p *Project, pp *ParserProject, v *Version) error {
	ctx, span := tracer.Start(ctx, "save-generated-project")
	defer span.End()
	// Get task again, to exit early if another generator finished early.
	t, err := task.FindOneId(ctx, g.Task.Id)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", g.Task.Id)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", g.Task.Id)
	}
	g.Task = t

	if g.Task.GeneratedTasks {
		grip.Debug(message.Fields{
			"message": "skipping attempting to update parser project because another generator marked the task complete",
			"task":    g.Task.Id,
			"version": g.Task.Version,
		})
		return mongo.ErrNoDocuments
	}

	if err := updateParserProject(ctx, settings, v, pp, t.Id); err != nil {
		return errors.WithStack(err)
	}

	if err := g.saveNewBuildsAndTasks(ctx, settings, v, p); err != nil {
		return errors.Wrap(err, "saving new builds and tasks")
	}
	return nil
}

// updateParserProject updates the parser project along with generated task ID
// and updated config number.
func updateParserProject(ctx context.Context, settings *evergreen.Settings, v *Version, pp *ParserProject, taskId string) error {
	ctx, span := tracer.Start(ctx, "update-parser-project")
	defer span.End()

	if utility.StringSliceContains(pp.UpdatedByGenerators, taskId) {
		// This generator has already updated the parser project so continue.
		span.SetAttributes(attribute.Bool(skippingGenerateTasksAttribute, true))
		return nil
	}

	// If this is the first time the parser project has been updated by a generator, cache a copy of
	// the parser project representing its state before any generators updated it. This is required for
	// a special case that enables child patches to reference a pre-generated copy of the parser project
	// so that they can call generate.tasks again without getting a "redefining tasks" error.
	if len(pp.UpdatedByGenerators) == 0 {
		span.SetAttributes(attribute.Bool(firstGenerateTasksOnVersionAttribute, true))
		oldPP, err := ParserProjectFindOneByID(ctx, settings, v.ProjectStorageMethod, v.Id)
		if err != nil {
			return errors.Wrapf(err, "finding parser project '%s' from before task generation", v.Id)
		}
		if oldPP == nil {
			return errors.Errorf("parser project '%s' not found", v.Id)
		}
		oldPP.Id = preGeneratedParserProjectId(oldPP.Id)
		preGenerationStorageMethod, err := ParserProjectUpsertOneWithS3Fallback(ctx, settings, evergreen.ProjectStorageMethodDB, oldPP)
		if err != nil {
			return errors.Wrapf(err, "upserting pre-generation parser project '%s'", oldPP.Id)
		}
		if err = v.UpdatePreGenerationProjectStorageMethod(ctx, preGenerationStorageMethod); err != nil {
			return errors.Wrapf(err, "updating version's parser project pre-generation storage method from '%s' to '%s'", v.ProjectStorageMethod, preGenerationStorageMethod)
		}
	}

	pp.UpdatedByGenerators = append(pp.UpdatedByGenerators, taskId)

	ppStorageMethod, err := ParserProjectUpsertOneWithS3Fallback(ctx, settings, v.ProjectStorageMethod, pp)
	if err != nil {
		return errors.Wrapf(err, "upserting parser project '%s'", pp.Id)
	}
	if err := v.UpdateProjectStorageMethod(ctx, ppStorageMethod); err != nil {
		return errors.Wrapf(err, "updating version's parser project storage method from '%s' to '%s'", v.ProjectStorageMethod, ppStorageMethod)
	}

	return nil
}

func cacheProjectData(p *Project) projectMaps {
	cachedProject := projectMaps{
		buildVariants: map[string]struct{}{},
		tasks:         map[string]*ProjectTask{},
		functions:     map[string]*YAMLCommandSet{},
	}
	// use a set because we never need to look up buildvariants
	for _, bv := range p.BuildVariants {
		cachedProject.buildVariants[bv.Name] = struct{}{}
	}
	for i, t := range p.Tasks {
		cachedProject.tasks[t.Name] = &p.Tasks[i]
	}
	// functions is already a map, cache it anyway for convenience
	cachedProject.functions = p.Functions
	return cachedProject
}

// saveNewBuildsAndTasks saves new builds and tasks to the db.
func (g *GeneratedProject) saveNewBuildsAndTasks(ctx context.Context, settings *evergreen.Settings, v *Version, p *Project) error {
	ctx, span := tracer.Start(ctx, "save-builds-and-tasks")
	defer span.End()

	span.SetAttributes(attribute.Int(numGenerateTaskBVAttribute, len(g.BuildVariants)))
	// Inherit priority from the parent generator task.
	for i, projBv := range p.BuildVariants {
		for j := range projBv.Tasks {
			p.BuildVariants[i].Tasks[j].Priority = g.Task.Priority
		}
	}

	existingBuilds, err := build.Find(ctx, build.ByVersion(v.Id))
	if err != nil {
		return errors.Wrap(err, "finding builds for version")
	}
	buildSet := map[string]struct{}{}
	for _, b := range existingBuilds {
		buildSet[b.BuildVariant] = struct{}{}
	}

	newTVPairs, activationInfo := g.GetNewTasksAndActivationInfo(ctx, v, p)

	if v.Requester == evergreen.GithubPRRequester {
		numCheckRuns := p.GetNumCheckRunsFromTaskVariantPairs(newTVPairs)
		checkRunLimit := settings.GitHubCheckRun.CheckRunLimit
		if numCheckRuns > checkRunLimit {
			return errors.Errorf("total number of checkRuns (%d) exceeds maximum limit (%d)", numCheckRuns, checkRunLimit)
		}
	}

	// Group into new builds and new tasks for existing builds.
	newTVPairsForExistingVariants := TaskVariantPairs{}
	newTVPairsForNewVariants := TaskVariantPairs{}
	for _, execTask := range newTVPairs.ExecTasks {
		if _, ok := buildSet[execTask.Variant]; ok {
			newTVPairsForExistingVariants.ExecTasks = append(newTVPairsForExistingVariants.ExecTasks, execTask)
		} else {
			newTVPairsForNewVariants.ExecTasks = append(newTVPairsForNewVariants.ExecTasks, execTask)
		}
	}
	for _, dispTask := range newTVPairs.DisplayTasks {
		if _, ok := buildSet[dispTask.Variant]; ok {
			newTVPairsForExistingVariants.DisplayTasks = append(newTVPairsForExistingVariants.DisplayTasks, dispTask)
		} else {
			newTVPairsForNewVariants.DisplayTasks = append(newTVPairsForNewVariants.DisplayTasks, dispTask)
		}
	}

	// This will only be populated for patches, not mainline commits.
	projectRef, err := FindMergedProjectRef(ctx, p.Identifier, v.Id, true)
	if err != nil {
		return errors.Wrapf(err, "finding merged project ref '%s' for version '%s'", p.Identifier, v.Id)
	}
	if projectRef == nil {
		return errors.Errorf("project '%s' not found", p.Identifier)
	}

	// Compile a lookup table of task IDs for all tasks to be created, both in
	// existing builds and in newly-generated builds. This lookup table is
	// needed to ensure when a task is created, it can find the IDs of the tasks
	// it will depend on.
	allTasksToBeCreatedIncludingDeps, err := NewTaskIdConfig(p, v, *newTVPairs, projectRef.Identifier)
	if err != nil {
		return errors.Wrap(err, "creating task ID table for new variant-tasks to create")
	}
	tasksToBeGenerated := allTasksToBeCreatedIncludingDeps.Length()
	if err = validateGeneratedProjectMaxTasks(ctx, v, g.Task.Id, tasksToBeGenerated); err != nil {
		return errors.Wrapf(err, "validating the number of tasks to be added by '%s'", g.Task.Id)
	}
	span.SetAttributes(attribute.Int(numGenerateTasksAttribute, tasksToBeGenerated))
	if err = g.Task.SetNumGeneratedTasks(ctx, tasksToBeGenerated); err != nil {
		return errors.Wrapf(err, "setting number of tasks generated by '%s'", g.Task.Id)
	}

	creationInfo := TaskCreationInfo{
		Project:        p,
		ProjectRef:     projectRef,
		Version:        v,
		TaskIDs:        allTasksToBeCreatedIncludingDeps,
		Pairs:          newTVPairsForExistingVariants,
		ActivationInfo: *activationInfo,
		GeneratedBy:    g.Task.Id,
		// If the parent generator is required to finish, then its generated
		// tasks inherit that requirement.
		ActivatedTasksAreEssentialToSucceed: g.Task.IsEssentialToSucceed,
	}
	if evergreen.IsPatchRequester(v.Requester) {
		patchDoc, err := patch.FindOneId(ctx, v.Id)
		if err != nil {
			return errors.Wrapf(err, "finding patch '%s'", v.Id)
		}
		if patchDoc != nil {
			tsParams, err := newTestSelectionParams(patchDoc)
			if err != nil {
				return errors.Wrap(err, "making test selection params for task creation")
			}
			creationInfo.TestSelectionParams = *tsParams
		}
	}

	activatedTasksInExistingBuilds, activatedDependenciesFromTasksInExistingBuilds, err := addNewTasksToExistingBuilds(ctx, creationInfo, existingBuilds, evergreen.GenerateTasksActivator)
	if err != nil {
		return errors.Wrap(err, "adding new tasks")
	}

	creationInfo.Pairs = newTVPairsForNewVariants
	activatedTasksInNewBuilds, activatedDependenciesFromTasksInNewBuilds, err := addNewBuilds(ctx, creationInfo, existingBuilds)
	if err != nil {
		return errors.Wrap(err, "adding new builds")
	}

	if err := updateBuildStatusesForGeneratedTasks(ctx, v.Id, newTVPairsForExistingVariants); err != nil {
		return errors.Wrap(err, "updating statuses for builds that have added generated tasks")
	}

	numActivatedGenerateTasks := len(activatedTasksInExistingBuilds) + len(activatedTasksInNewBuilds) + len(activatedDependenciesFromTasksInExistingBuilds) + len(activatedDependenciesFromTasksInNewBuilds)
	span.SetAttributes(attribute.Int(numActivatedGenerateTasksAttribute, numActivatedGenerateTasks))
	if err = g.Task.SetNumActivatedGeneratedTasks(ctx, numActivatedGenerateTasks); err != nil {
		return errors.Wrapf(err, "setting number of tasks generated and activated by '%s'", g.Task.Id)
	}

	// only want to add dependencies to activated tasks
	if err = g.addDependencies(ctx, append(activatedTasksInExistingBuilds, activatedTasksInNewBuilds...)); err != nil {
		return errors.Wrap(err, "adding dependencies")
	}

	return nil
}

// updateBuildStatusesForGeneratedTasks updates the statuses of existing builds
// that have added generated tasks to run. For example, if a build was already
// finished and just had new tasks generated, the status should be updated to
// running.
func updateBuildStatusesForGeneratedTasks(ctx context.Context, versionID string, newTVPairsForExistingVariants TaskVariantPairs) error {
	bvs := getBuildVariantsFromPairs(newTVPairsForExistingVariants)

	builds, err := build.FindByVersionAndVariants(ctx, versionID, bvs)
	if err != nil {
		return errors.Wrap(err, "finding builds that need their status updated")
	}

	buildIDs := make([]string, 0, len(builds))
	for _, b := range builds {
		buildIDs = append(buildIDs, b.Id)
	}

	return UpdateVersionAndPatchStatusForBuilds(ctx, buildIDs)
}

// getBuildVariantsFromPairs returns a slice of all unique build variant names
// from build variant task pairs.
func getBuildVariantsFromPairs(pairs TaskVariantPairs) []string {
	bvSet := make(map[string]any)
	for _, pair := range pairs.ExecTasks {
		bvSet[pair.Variant] = struct{}{}
	}
	for _, pair := range pairs.DisplayTasks {
		bvSet[pair.Variant] = struct{}{}
	}

	var uniqueBVs []string
	for bv := range bvSet {
		uniqueBVs = append(uniqueBVs, bv)
	}

	return uniqueBVs
}

func validateGeneratedProjectMaxTasks(ctx context.Context, v *Version, taskID string, tasksToBeCreated int) error {
	numExistingTasks, err := task.Count(ctx, db.Query(bson.M{
		task.VersionKey:     v.Id,
		task.GeneratedByKey: bson.M{"$ne": taskID},
	}))
	if err != nil {
		return errors.Wrapf(err, "counting tasks for version '%s'", v.Id)
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	totalTasks := numExistingTasks + tasksToBeCreated
	if settings.TaskLimits.MaxTasksPerVersion > 0 && totalTasks > settings.TaskLimits.MaxTasksPerVersion {
		return errors.Errorf("version's total number of tasks after generation (%d) exceeds maximum limit (%d)", totalTasks, settings.TaskLimits.MaxTasksPerVersion)
	}
	return nil
}

// GetNewTasksAndActivationInfo computes the generate.tasks variant-tasks to be
// created and specific activation information for those tasks.
func (g *GeneratedProject) GetNewTasksAndActivationInfo(ctx context.Context, v *Version, p *Project) (*TaskVariantPairs, *specificActivationInfo) {
	if g.NewTVPairs != nil && g.ActivationInfo != nil {
		return g.NewTVPairs, g.ActivationInfo
	}
	activationInfo := g.findTasksAndVariantsWithSpecificActivations(v.Requester)
	newTasks := g.getNewTasksWithDependencies(ctx, v, p, &activationInfo)
	g.NewTVPairs = &newTasks
	g.ActivationInfo = &activationInfo
	return g.NewTVPairs, g.ActivationInfo
}

// CheckForCycles builds a dependency graph from the existing tasks in the version and simulates
// adding the generated tasks, their dependencies, and dependencies on the generated tasks to the graph.
// Returns a DependencyCycleError error if the resultant graph contains dependency cycles.
func (g *GeneratedProject) CheckForCycles(ctx context.Context, v *Version, p *Project, projectRef *ProjectRef) error {
	ctx, span := tracer.Start(ctx, "check-for-cycles")
	defer span.End()
	existingTasksGraph, err := task.VersionDependencyGraph(ctx, g.Task.Version, false)
	if err != nil {
		return errors.Wrapf(err, "creating dependency graph for version '%s'", g.Task.Version)
	}

	simulatedGraph, err := g.simulateNewTasks(ctx, existingTasksGraph, v, p, projectRef)
	if err != nil {
		return errors.Wrap(err, "simulating new tasks")
	}

	if cycles := simulatedGraph.Cycles(); len(cycles) > 0 {
		return errors.Wrapf(DependencyCycleError, "'%s'", cycles)
	}

	return nil
}

// simulateNewTasks adds the tasks we're planning to add to the version to the graph and
// adds simulated edges from each task that depends on the generator to each of the generated tasks.
func (g *GeneratedProject) simulateNewTasks(ctx context.Context, graph task.DependencyGraph, v *Version, p *Project, projectRef *ProjectRef) (task.DependencyGraph, error) {
	newTVPairs, _ := g.GetNewTasksAndActivationInfo(ctx, v, p)
	creationInfo := TaskCreationInfo{
		Project:    p,
		ProjectRef: projectRef,
		Version:    v,
		Pairs:      *newTVPairs,
	}
	taskIDs, err := getTaskIdConfig(ctx, creationInfo)
	if err != nil {
		return graph, errors.Wrap(err, "getting task ids")
	}

	graph = addTasksToGraph(newTVPairs.ExecTasks, graph, p, taskIDs)
	return g.addDependencyEdgesToGraph(ctx, newTVPairs.ExecTasks, v, p, graph, taskIDs)
}

// getNewTasksWithDependencies returns the generated tasks and their recursive dependencies.
func (g *GeneratedProject) getNewTasksWithDependencies(ctx context.Context, v *Version, p *Project, activationInfo *specificActivationInfo) TaskVariantPairs {
	_, span := tracer.Start(ctx, "get-new-tasks-with-dependencies")
	defer span.End()
	newTVPairs := TaskVariantPairs{}
	for _, bv := range g.BuildVariants {
		newTVPairs = appendTasks(newTVPairs, bv, p)
	}

	var err error
	newTVPairs.ExecTasks, err = IncludeDependenciesWithGenerated(p, newTVPairs.ExecTasks, v.Requester, activationInfo, g.BuildVariants)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "error including dependencies for generator",
		"task":    g.Task.Id,
	}))

	return newTVPairs
}

// addTasksToGraph adds tasks to the graph and adds dependency edges from each task to each of its dependencies.
func addTasksToGraph(tasks TVPairSet, graph task.DependencyGraph, p *Project, taskIDs TaskIdConfig) task.DependencyGraph {
	for _, newTask := range tasks {
		graph.AddTaskNode(task.TaskNode{
			ID:      taskIDs.ExecutionTasks.GetId(newTask.Variant, newTask.TaskName),
			Name:    newTask.TaskName,
			Variant: newTask.Variant,
		})
	}

	nodes := graph.Nodes()
	bvts := make([]BuildVariantTaskUnit, 0, len(nodes))
	for _, node := range nodes {
		bvt := p.FindTaskForVariant(node.Name, node.Variant)
		if bvt != nil {
			bvts = append(bvts, *bvt)
		}
	}

	for _, dep := range dependenciesForTaskUnit(bvts, p) {
		dep.From.ID = taskIDs.ExecutionTasks.GetId(dep.From.Variant, dep.From.Name)
		dep.To.ID = taskIDs.ExecutionTasks.GetId(dep.To.Variant, dep.To.Name)
		graph.AddEdge(dep.From, dep.To, dep.Status)
	}

	return graph
}

// addDependencyEdgesToGraph adds edges from the tasks that depend on the generator to activated generated tasks.
func (g *GeneratedProject) addDependencyEdgesToGraph(ctx context.Context, newTasks TVPairSet, v *Version, p *Project, graph task.DependencyGraph, taskIDs TaskIdConfig) (task.DependencyGraph, error) {
	activatedNewTasks, err := g.filterInactiveTasks(ctx, newTasks, v, p)
	if err != nil {
		return graph, errors.Wrap(err, "filtering inactive tasks")
	}

	for _, newTask := range activatedNewTasks {
		node := task.TaskNode{
			ID:      taskIDs.ExecutionTasks.GetId(newTask.Variant, newTask.TaskName),
			Name:    newTask.TaskName,
			Variant: newTask.Variant,
		}
		for _, edge := range graph.EdgesIntoTask(node) {
			graph.AddEdge(edge.From, node, edge.Status)
		}
	}

	return graph, nil
}

// filterInactiveTasks returns a copy of tasks with the tasks that will not be activated by the generator removed.
func (g *GeneratedProject) filterInactiveTasks(ctx context.Context, tasks TVPairSet, v *Version, p *Project) (TVPairSet, error) {
	existingBuilds, err := build.Find(ctx, build.ByVersion(v.Id))
	if err != nil {
		return nil, errors.Wrap(err, "finding builds for version")
	}
	existingBuildMap := make(map[string]bool)
	for _, b := range existingBuilds {
		existingBuildMap[b.BuildVariant] = true
	}

	buildSet := make(map[string][]string)
	for _, t := range tasks {
		buildSet[t.Variant] = append(buildSet[t.Variant], t.TaskName)
	}

	_, activationInfo := g.GetNewTasksAndActivationInfo(ctx, v, p)
	activatedTasks := make(TVPairSet, 0, len(tasks))
	for bv, tasks := range buildSet {
		if existingBuildMap[bv] {
			// Existing builds are activated when tasks are added as long as the build isn't specifically not activated.
			projectBV := p.FindBuildVariant(bv)
			if projectBV == nil {
				continue
			}
			if !utility.FromBoolTPtr(projectBV.Activate) {
				continue
			}
		} else if activationInfo.variantHasSpecificActivation(bv) {
			// New builds with specific activation are activated later by ActivateElapsedBuildsAndTasks.
			// Skip simulating their dependencies because the builds and their tasks are not activated now so will not be adding dependencies.
			continue
		}

		for _, t := range tasks {
			// If it is a stepback task, it should activate according to the stepback logic.
			if st := activationInfo.getStepbackTask(bv, t); st != nil {
				if st.shouldActivate() {
					activatedTasks = append(activatedTasks, TVPair{Variant: bv, TaskName: t})
				}
				continue
			}
			// Tasks with specific activation are activated later by ActivateElapsedBuildsAndTasks and we do not add dependencies for them.
			if !activationInfo.taskHasSpecificActivation(bv, t) {
				activatedTasks = append(activatedTasks, TVPair{Variant: bv, TaskName: t})
			}
		}
	}

	return activatedTasks, nil
}

type specificActivationInfo struct {
	stepbackTasks      map[string][]specificStepbackInfo // tasks by variant that are being stepped back
	activationTasks    map[string][]string               // tasks by variant that have batchtime or activate specified
	activationVariants []string                          // variants that have batchtime or activate specified
}

type specificStepbackInfo struct {
	task     string
	activate bool
}

func (s *specificStepbackInfo) shouldActivate() bool {
	if s == nil {
		return false
	}
	return s.activate
}

func newSpecificActivationInfo() specificActivationInfo {
	return specificActivationInfo{
		stepbackTasks:      map[string][]specificStepbackInfo{},
		activationTasks:    map[string][]string{},
		activationVariants: []string{},
	}
}

func (b *specificActivationInfo) variantHasSpecificActivation(variant string) bool {
	return utility.StringSliceContains(b.activationVariants, variant)
}

func (b *specificActivationInfo) getActivationTasks(variant string) []string {
	return b.activationTasks[variant]
}

func (b *specificActivationInfo) hasActivationTasks() bool {
	return len(b.activationTasks) > 0
}

func (b *specificActivationInfo) getStepbackTask(variant, task string) *specificStepbackInfo {
	for _, stepbackInfo := range b.stepbackTasks[variant] {
		if stepbackInfo.task == task {
			return &stepbackInfo
		}
	}
	return nil
}

func (b *specificActivationInfo) taskHasSpecificActivation(variant, task string) bool {
	return utility.StringSliceContains(b.activationTasks[variant], task)
}

func (b *specificActivationInfo) taskOrVariantHasSpecificActivation(variant, task string) bool {
	if b == nil {
		return false
	}
	return b.taskHasSpecificActivation(variant, task) || b.variantHasSpecificActivation(variant)
}

func (g *GeneratedProject) findTasksAndVariantsWithSpecificActivations(requester string) specificActivationInfo {
	res := newSpecificActivationInfo()
	for _, bv := range g.BuildVariants {
		// Only consider batchtime for mainline builds. A task/BV will have
		// specific activation if activate is explicitly set to false;
		// otherwise, if it's explicitly set to true, activate it immediately.
		if evergreen.ShouldConsiderBatchtime(requester) && bv.hasSpecificActivation() {
			res.activationVariants = append(res.activationVariants, bv.name())
		} else if !utility.FromBoolTPtr(bv.Activate) {
			res.activationVariants = append(res.activationVariants, bv.name())
		}
		// Regardless of whether the build variant has batchtime, there may be tasks with different batchtime
		batchTimeTasks := []string{}
		for _, bvt := range bv.Tasks {
			// If we are doing stepback, we only want to activate specific tasks.
			if g.Task.ActivatedBy == evergreen.StepbackTaskActivator {
				info := specificStepbackInfo{task: bvt.Name}
				if utility.FromBoolPtr(bvt.Activate) {
					// If the generated task has "activate: true", we should activate it.
					info.activate = true
				} else if isStepbackTask(g.Task, bv.Name, bvt.Name) {
					// If the generated task is one of the ones being stepped back, we should activate it.
					info.activate = true
				}
				res.stepbackTasks[bv.Name] = append(res.stepbackTasks[bv.Name], info)
				continue // Don't consider batchtime/activation if we're stepping generated tasks.
			}
			if evergreen.ShouldConsiderBatchtime(requester) && bvt.hasSpecificActivation() {
				batchTimeTasks = append(batchTimeTasks, bvt.Name)
			} else if !utility.FromBoolTPtr(bvt.Activate) {
				batchTimeTasks = append(batchTimeTasks, bvt.Name)
			}
		}
		if len(batchTimeTasks) > 0 {
			res.activationTasks[bv.name()] = batchTimeTasks
		}
	}
	return res
}

// isStepbackTask returns true if the task unit is supposed to be stepped back for this generator
func isStepbackTask(generatorTask *task.Task, variant, taskName string) bool {
	for bv, tasks := range generatorTask.GeneratedTasksToActivate {
		if bv == variant && utility.StringSliceContains(tasks, taskName) {
			return true
		}
	}
	return false
}

func (g *GeneratedProject) addDependencies(ctx context.Context, newTaskIds []string) error {
	_, span := tracer.Start(ctx, "add-dependencies")
	defer span.End()
	statuses := []string{evergreen.TaskSucceeded, task.AllStatuses}
	for _, status := range statuses {
		if err := g.Task.UpdateDependsOn(ctx, status, newTaskIds); err != nil {
			return errors.Wrapf(err, "updating tasks depending on '%s'", g.Task.Id)
		}
	}

	return nil
}

func appendTasks(pairs TaskVariantPairs, bv parserBV, p *Project) TaskVariantPairs {
	taskGroups := map[string]TaskGroup{}
	for _, tg := range p.TaskGroups {
		taskGroups[tg.Name] = tg
	}
	for _, t := range bv.Tasks {
		if tg, ok := taskGroups[t.Name]; ok {
			for _, taskInGroup := range tg.Tasks {
				pairs.ExecTasks = append(pairs.ExecTasks, TVPair{bv.Name, taskInGroup})
			}
		} else {
			pairs.ExecTasks = append(pairs.ExecTasks, TVPair{bv.Name, t.Name})
		}
	}
	for _, dt := range bv.DisplayTasks {
		pairs.DisplayTasks = append(pairs.DisplayTasks, TVPair{bv.Name, dt.Name})
	}
	return pairs
}

// addGeneratedProjectToConfig takes a ParserProject and a YML config and returns a new one with the GeneratedProject included.
// support for YML config will be degraded.
func (g *GeneratedProject) addGeneratedProjectToConfig(intermediateProject *ParserProject, cachedProject projectMaps) (*ParserProject, error) {
	// Append buildvariants, tasks, and functions to the config.
	intermediateProject.TaskGroups = append(intermediateProject.TaskGroups, g.TaskGroups...)
	intermediateProject.Tasks = append(intermediateProject.Tasks, g.Tasks...)
	for key, val := range g.Functions {
		intermediateProject.Functions[key] = val
	}
	for _, bv := range g.BuildVariants {
		// If the buildvariant already exists, append tasks to it.
		if _, ok := cachedProject.buildVariants[bv.Name]; ok {
			for i, intermediateProjectBV := range intermediateProject.BuildVariants {
				if intermediateProjectBV.Name == bv.Name {
					intermediateProject.BuildVariants[i].Tasks = append(intermediateProject.BuildVariants[i].Tasks, bv.Tasks...)

					for _, dt := range bv.DisplayTasks {
						// check if the display task already exists, and if it does add the exec tasks to the existing display task
						foundExisting := false
						for j, intermediateProjectDT := range intermediateProjectBV.DisplayTasks {
							if intermediateProjectDT.Name == dt.Name {
								foundExisting = true
								execTasksToAdd := []string{}
								for _, generatedExecTask := range dt.ExecutionTasks {
									// Avoid adding duplicate execution tasks to the display task.
									if !slices.Contains(intermediateProjectDT.ExecutionTasks, generatedExecTask) {
										execTasksToAdd = append(execTasksToAdd, generatedExecTask)
									}
								}
								intermediateProject.BuildVariants[i].DisplayTasks[j].ExecutionTasks = append(
									intermediateProject.BuildVariants[i].DisplayTasks[j].ExecutionTasks, execTasksToAdd...)
								break
							}
						}
						if !foundExisting {
							intermediateProject.BuildVariants[i].DisplayTasks = append(intermediateProject.BuildVariants[i].DisplayTasks, dt)
						}
					}

				}
			}
		} else {
			// If the buildvariant does not exist, create it.
			intermediateProject.BuildVariants = append(intermediateProject.BuildVariants, bv)
		}
	}
	return intermediateProject, nil
}

func variantExistsInGeneratedProject(variants []parserBV, variant string) bool {
	for bv := range variants {
		if variants[bv].Name == variant {
			return true
		}
	}
	return false
}

// projectMaps is a struct of maps of project fields, which allows efficient comparisons of generated projects to projects.
type projectMaps struct {
	buildVariants map[string]struct{}
	tasks         map[string]*ProjectTask
	functions     map[string]*YAMLCommandSet
}

// validateMaxTasksAndVariants validates that the GeneratedProject contains fewer than maxGeneratedBuildVariants variants and maxGeneratedTasks tasks.
func (g *GeneratedProject) validateMaxTasksAndVariants() error {
	catcher := grip.NewBasicCatcher()
	if len(g.BuildVariants) > maxGeneratedBuildVariants {
		catcher.Errorf("it is illegal to generate more than %d buildvariants", maxGeneratedBuildVariants)
	}
	if len(g.Tasks) > maxGeneratedTasks {
		catcher.Errorf("it is illegal to generate more than %d tasks", maxGeneratedTasks)
	}
	return catcher.Resolve()
}

// validateNoRedefine validates that buildvariants, tasks, or functions, are not redefined
// except to add a task to a buildvariant.
func (g *GeneratedProject) validateNoRedefine(cachedProject projectMaps) error {
	catcher := grip.NewBasicCatcher()
	for _, bv := range g.BuildVariants {
		if _, ok := cachedProject.buildVariants[bv.Name]; ok {
			{
				if isNonZeroBV(bv) {
					catcher.Errorf("cannot redefine buildvariants in 'generate.tasks' (%s), except to add tasks", bv.Name)
				}
			}
		}
	}
	for _, t := range g.Tasks {
		if _, ok := cachedProject.tasks[t.Name]; ok {
			catcher.Errorf("cannot redefine tasks in 'generate.tasks' (%s)", t.Name)
		}
	}
	for f := range g.Functions {
		if _, ok := cachedProject.functions[f]; ok {
			catcher.Errorf("cannot redefine functions in 'generate.tasks' (%s)", f)
		}
	}
	return catcher.Resolve()
}

func isNonZeroBV(bv parserBV) bool {
	// Note that activate is purposefully not included in this list because it's common for
	// users to specify specifically that activation is false when generating tasks.
	// Even though it only really works if they add it for the individual tasks that they're adding,
	// it's a common enough pattern that we should allow it rather than breaking generators.
	if bv.DisplayName != "" || len(bv.Expansions) > 0 || len(bv.Modules) > 0 ||
		bv.Disable != nil || len(bv.Tags) > 0 ||
		bv.BatchTime != nil || bv.Patchable != nil || bv.PatchOnly != nil ||
		bv.AllowForGitTag != nil || bv.GitTagOnly != nil || len(bv.AllowedRequesters) > 0 ||
		bv.Stepback != nil || bv.DeactivatePrevious != nil || len(bv.RunOn) > 0 {
		return true
	}
	return false
}

// validateNoRecursiveGenerateTasks validates that no 'generate.tasks' calls another 'generate.tasks'.
func (g *GeneratedProject) validateNoRecursiveGenerateTasks(cachedProject projectMaps) error {
	catcher := grip.NewBasicCatcher()
	for _, t := range g.Tasks {
		for _, cmd := range t.Commands {
			if cmd.Command == evergreen.GenerateTasksCommandName {
				catcher.New("cannot define 'generate.tasks' from a 'generate.tasks' block")
			}
		}
	}
	for _, f := range g.Functions {
		for _, cmd := range f.List() {
			if cmd.Command == evergreen.GenerateTasksCommandName {
				catcher.New("cannot define 'generate.tasks' from a 'generate.tasks' block")
			}
		}
	}
	for _, bv := range g.BuildVariants {
		for _, t := range bv.Tasks {
			if projectTask, ok := cachedProject.tasks[t.Name]; ok {
				catcher.Add(validateCommands(projectTask, cachedProject, t))
			}
		}
	}
	return catcher.Resolve()
}

func validateCommands(projectTask *ProjectTask, cachedProject projectMaps, pvt parserBVTaskUnit) error {
	catcher := grip.NewBasicCatcher()
	for _, cmd := range projectTask.Commands {
		if cmd.Command == evergreen.GenerateTasksCommandName {
			catcher.Errorf("cannot assign a task that calls 'generate.tasks' from a 'generate.tasks' block (%s)", pvt.Name)
		}
		if cmd.Function != "" {
			if functionCmds, ok := cachedProject.functions[cmd.Function]; ok {
				for _, functionCmd := range functionCmds.List() {
					if functionCmd.Command == evergreen.GenerateTasksCommandName {
						catcher.Errorf("cannot assign a task that calls 'generate.tasks' from a 'generate.tasks' block (%s)", cmd.Function)
					}
				}
			}
		}
	}
	return catcher.Resolve()
}

func (g *GeneratedProject) validateGeneratedProject(cachedProject projectMaps) error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(g.validateMaxTasksAndVariants())
	catcher.Add(g.validateNoRedefine(cachedProject))
	catcher.Add(g.validateNoRecursiveGenerateTasks(cachedProject))

	return errors.WithStack(catcher.Resolve())
}

package model

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// previousManifestLookback bounds how many earlier versions are checked for a
// manifest, since versions whose config failed to load don't have one.
const previousManifestLookback = 10

// ModuleResolver resolves each module's commit for a single version at most
// once, so a config and manifest built from the same resolver always agree on
// module commits.
type ModuleResolver struct {
	projectRef *ProjectRef
	opts       ModuleResolverOpts

	initMu       sync.Mutex
	initialized  bool
	expansions   *util.Expansions
	baseManifest *manifest.Manifest
	// previousManifest is the manifest module commits are carried forward from.
	previousManifest *manifest.Manifest
	// beforeOrder is the order number previous versions must be below.
	beforeOrder int

	mu       sync.Mutex
	resolved map[string]*manifest.Module
}

// ModuleResolverOpts describes the version whose module commits are being
// resolved.
type ModuleResolverOpts struct {
	// VersionID, if known, is checked for an existing manifest before the base
	// version's manifest. It's only used for patches and periodic builds.
	VersionID string
	// Revision is the project revision, used to find the base manifest for
	// patches and periodic builds.
	Revision            string
	Requester           string
	RevisionOrderNumber int
	IngestTime          time.Time
	// TriggeringModuleCommit pins the module with the same owner, repo and
	// branch to the commit that triggered the version.
	TriggeringModuleCommit *manifest.Module
}

// NewModuleResolver returns a resolver for the module commits of a version.
func NewModuleResolver(projectRef *ProjectRef, opts ModuleResolverOpts) *ModuleResolver {
	return &ModuleResolver{
		projectRef:  projectRef,
		opts:        opts,
		beforeOrder: opts.RevisionOrderNumber,
		resolved:    map[string]*manifest.Module{},
	}
}

// Resolve returns the manifest entry for the module, resolving its commit on
// first use and returning the same entry on later calls. Failures aren't
// cached, so a later call can retry.
func (r *ModuleResolver) Resolve(ctx context.Context, module Module) (*manifest.Module, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mod, ok := r.resolved[module.Name]; ok {
		return mod, nil
	}
	mod, err := r.resolve(ctx, module)
	if err != nil {
		return nil, err
	}
	r.resolved[module.Name] = mod
	return mod, nil
}

// Manifest returns the version's manifest with the resolved module commits.
// It returns nil if there are no modules.
func (r *ModuleResolver) Manifest(ctx context.Context, v *Version, modules ModuleList) (*manifest.Manifest, error) {
	if len(modules) == 0 {
		return nil, nil
	}
	r.initMu.Lock()
	if !r.initialized && r.beforeOrder == 0 {
		r.beforeOrder = v.RevisionOrderNumber
	}
	r.initMu.Unlock()

	resolved := make(map[string]*manifest.Module, len(modules))
	for _, module := range modules {
		mod, err := r.Resolve(ctx, module)
		if err != nil {
			return nil, errors.Wrapf(err, "module '%s'", module.Name)
		}
		resolved[module.Name] = mod
	}
	newManifest := &manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      r.projectRef.Branch,
		IsBase:      v.Requester == evergreen.RepotrackerVersionRequester,
		Modules:     resolved,
	}
	setChangedModules(ctx, v, newManifest, r.baseManifest)
	return newManifest, nil
}

// TriggeringModuleName returns the name of the module pinned by
// TriggeringModuleCommit, or an empty string if no module matches.
func (r *ModuleResolver) TriggeringModuleName(ctx context.Context, modules ModuleList) (string, error) {
	if r.opts.TriggeringModuleCommit == nil {
		return "", nil
	}
	if err := r.init(ctx); err != nil {
		return "", err
	}
	for _, module := range modules {
		expanded, err := r.expand(module)
		if err != nil {
			return "", err
		}
		owner, repo, err := expanded.GetOwnerAndRepo()
		if err != nil {
			return "", errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)
		}
		if r.isTriggeringModule(owner, repo, expanded.Branch) {
			return module.Name, nil
		}
	}
	return "", nil
}

// resolvedModulesKey returns the module commits resolved so far as a stable
// string, so translations of the same revision that read module includes from
// different commits aren't shared.
func (r *ModuleResolver) resolvedModulesKey() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	pairs := make([]string, 0, len(r.resolved))
	for name, mod := range r.resolved {
		pairs = append(pairs, fmt.Sprintf("%s@%s", name, mod.Revision))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, ",")
}

func (r *ModuleResolver) shouldUseBaseRevision() bool {
	return utility.StringSliceContains(evergreen.PatchRequesters, r.opts.Requester) || r.opts.Requester == evergreen.AdHocRequester
}

func (r *ModuleResolver) shouldCarryForward() bool {
	return r.opts.Requester == evergreen.RepotrackerVersionRequester || r.opts.Requester == evergreen.TriggerRequester
}

func (r *ModuleResolver) isTriggeringModule(owner, repo, branch string) bool {
	pin := r.opts.TriggeringModuleCommit
	return pin != nil && owner == pin.Owner && repo == pin.Repo && branch == pin.Branch
}

func (r *ModuleResolver) init(ctx context.Context) error {
	r.initMu.Lock()
	defer r.initMu.Unlock()
	if r.initialized {
		return nil
	}

	projVars, err := FindMergedProjectVars(ctx, r.projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "getting project vars for project '%s'", r.projectRef.Id)
	}
	if projVars != nil {
		r.expansions = util.NewExpansions(projVars.Vars)
	}

	if r.shouldUseBaseRevision() {
		r.baseManifest, err = manifest.FindFromVersion(ctx, r.opts.VersionID, r.projectRef.Id, r.opts.Revision, r.opts.Requester)
		if err != nil {
			return errors.Wrap(err, "getting base manifest")
		}
	}
	if r.shouldCarryForward() {
		r.previousManifest, err = findPreviousWaterfallManifest(ctx, r.projectRef.Id, r.beforeOrder)
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":    "finding previous version's manifest, resolving modules without it",
			"source":     "manifest-creation",
			"version_id": r.opts.VersionID,
			"project_id": r.projectRef.Id,
		}))
	}

	r.initialized = true
	return nil
}

func (r *ModuleResolver) expand(module Module) (Module, error) {
	if r.expansions == nil {
		return module, nil
	}
	if err := util.ExpandValues(&module, r.expansions); err != nil {
		return module, errors.Wrapf(err, "expanding module '%s'", module.Name)
	}
	return module, nil
}

func (r *ModuleResolver) resolve(ctx context.Context, module Module) (*manifest.Module, error) {
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	module, err := r.expand(module)
	if err != nil {
		return nil, err
	}
	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)
	}

	isTriggeringModule := r.isTriggeringModule(owner, repo, module.Branch)
	if isTriggeringModule {
		module.Ref = r.opts.TriggeringModuleCommit.Revision
	}

	// GitHub wikis are always cloned at default-branch HEAD. Do not reuse
	// base manifest pins; every run must resolve via getManifestModule.
	if r.shouldUseBaseRevision() && !module.AutoUpdate && r.baseManifest != nil && !IsWikiRepo(repo) {
		if baseModule, ok := r.baseManifest.Modules[module.Name]; ok {
			// Use base module revision unless the YAML explicitly specifies a different ref.
			if module.Ref == "" || module.Ref == baseModule.Revision {
				inherited := *baseModule
				inherited.SelectionRule = manifest.ModuleSelectionBaseVersion
				return &inherited, nil
			}
		}
	}

	previous := r.previousModule(module.Name, owner, repo, module.Branch)
	if previous != nil && module.Ref == "" && r.isTriggeredByModule() {
		return previous, nil
	}

	mod, err := getManifestModule(ctx, r.projectRef, module, r.opts.Requester, r.opts.IngestTime)
	if err != nil {
		return nil, err
	}
	if isTriggeringModule {
		mod.SelectionRule = manifest.ModuleSelectionTriggerCommit
	}
	if previous != nil && module.Ref == "" {
		mod = r.newerModuleCommit(ctx, module.Name, previous, mod)
	}
	return mod, nil
}

// isTriggeredByModule returns whether the version was triggered by a commit
// to one of the modules in the previous version's manifest.
func (r *ModuleResolver) isTriggeredByModule() bool {
	if r.previousManifest == nil {
		return false
	}
	for _, mod := range r.previousManifest.Modules {
		if mod != nil && r.isTriggeringModule(mod.Owner, mod.Repo, mod.Branch) {
			return true
		}
	}
	return false
}

// previousModule returns the module's entry in the previous manifest if it
// tracked the same repo and branch and wasn't pinned to a ref.
func (r *ModuleResolver) previousModule(name, owner, repo, branch string) *manifest.Module {
	if r.previousManifest == nil {
		return nil
	}
	mod, ok := r.previousManifest.Modules[name]
	if !ok || mod == nil || mod.Revision == "" || mod.SelectionRule == manifest.ModuleSelectionPinnedRef {
		return nil
	}
	if mod.Owner != owner || mod.Repo != repo || mod.Branch != branch {
		return nil
	}
	prev := *mod
	prev.SelectionRule = manifest.ModuleSelectionCarriedForward
	return &prev
}

// newerModuleCommit returns whichever is newer of the previous version's
// commit and candidate, the commit looked up for this version. If the order
// can't be determined, it returns candidate.
func (r *ModuleResolver) newerModuleCommit(ctx context.Context, moduleName string, previous, candidate *manifest.Module) *manifest.Module {
	if previous.Revision == candidate.Revision {
		return candidate
	}
	logFields := func(msg string) message.Fields {
		return message.Fields{
			"message":            msg,
			"source":             "manifest-creation",
			"version_id":         r.opts.VersionID,
			"project_id":         r.projectRef.Id,
			"module":             moduleName,
			"owner":              candidate.Owner,
			"repo":               candidate.Repo,
			"branch":             candidate.Branch,
			"previous_revision":  previous.Revision,
			"candidate_revision": candidate.Revision,
		}
	}

	order, err := getIngestOrder(ctx, previous, candidate)
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, logFields("checking module commit ingest order, using candidate commit")))
		return candidate
	}
	switch order {
	case ingestOrderCandidateNewer, ingestOrderUnknown:
		return candidate
	case ingestOrderPreviousNewer:
		grip.Info(ctx, logFields("keeping previous module commit because candidate commit was ingested before it"))
		return previous
	}

	grip.Info(ctx, logFields("comparing module commits with GitHub"))
	ghCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	status, err := thirdparty.GetCommitComparisonStatus(ghCtx, candidate.Owner, candidate.Repo, previous.Revision, candidate.Revision)
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, logFields("comparing module commits with GitHub, using candidate commit")))
		return candidate
	}
	if status == "behind" {
		grip.Info(ctx, logFields("keeping previous module commit because candidate commit is behind it"))
		return previous
	}
	return candidate
}

type ingestOrder int

const (
	ingestOrderUnknown ingestOrder = iota
	ingestOrderCandidateNewer
	ingestOrderPreviousNewer
	ingestOrderPartial
)

// getIngestOrder returns which of the two module commits Evergreen ingested later.
func getIngestOrder(ctx context.Context, previous, candidate *manifest.Module) (ingestOrder, error) {
	var revisions []RepositoryRevision
	err := db.FindAllQ(ctx, RepositoryRevisionsHistoryCollection, db.Query(bson.M{
		RepositoryRevisionOwnerKey:    candidate.Owner,
		RepositoryRevisionRepoKey:     candidate.Repo,
		RepositoryRevisionBranchKey:   candidate.Branch,
		RepositoryRevisionRevisionKey: bson.M{"$in": []string{previous.Revision, candidate.Revision}},
	}), &revisions)
	if err != nil {
		return ingestOrderUnknown, errors.Wrap(err, "finding module commit ingest times")
	}
	ingestTimes := make(map[string]time.Time, len(revisions))
	for _, rev := range revisions {
		ingestTimes[rev.Revision] = rev.IngestTime
	}
	previousTime, hasPrevious := ingestTimes[previous.Revision]
	candidateTime, hasCandidate := ingestTimes[candidate.Revision]
	switch {
	case hasPrevious && hasCandidate && candidateTime.Before(previousTime):
		return ingestOrderPreviousNewer, nil
	case hasPrevious && hasCandidate:
		return ingestOrderCandidateNewer, nil
	case hasPrevious || hasCandidate:
		return ingestOrderPartial, nil
	default:
		return ingestOrderUnknown, nil
	}
}

// findPreviousWaterfallManifest returns the manifest of the most recent mainline or
// trigger version in the project that has one. If beforeOrder is set, only
// versions before it are considered.
func findPreviousWaterfallManifest(ctx context.Context, projectID string, beforeOrder int) (*manifest.Manifest, error) {
	q := bson.M{
		VersionIdentifierKey: projectID,
		VersionRequesterKey: bson.M{
			"$in": []string{evergreen.RepotrackerVersionRequester, evergreen.TriggerRequester},
		},
	}
	if beforeOrder > 0 {
		q[VersionRevisionOrderNumberKey] = bson.M{"$lt": beforeOrder}
	}
	versions, err := VersionFind(ctx, db.Query(q).
		WithFields(VersionIdKey).
		Sort([]string{"-" + VersionRevisionOrderNumberKey}).
		Limit(previousManifestLookback))
	if err != nil {
		return nil, errors.Wrap(err, "finding previous versions")
	}
	if len(versions) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(versions))
	for _, v := range versions {
		ids = append(ids, v.Id)
	}
	manifests, err := manifest.Find(ctx, manifest.ByIds(ids))
	if err != nil {
		return nil, errors.Wrap(err, "finding previous versions' manifests")
	}
	manifestsByID := make(map[string]manifest.Manifest, len(manifests))
	for _, m := range manifests {
		manifestsByID[m.Id] = m
	}
	for _, v := range versions {
		if m, ok := manifestsByID[v.Id]; ok {
			return &m, nil
		}
	}
	return nil, nil
}

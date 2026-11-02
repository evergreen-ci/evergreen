package model

import (
	"context"
	"fmt"
	"maps"
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

const (
	// previousManifestLookback bounds how many earlier versions are checked for a
	// manifest, since versions whose config failed to load don't have one.
	previousManifestLookback = 10
	// maxWaterfallHeadClaims bounds how many times a version tries to replace
	// the waterfall head when other versions keep replacing it first.
	maxWaterfallHeadClaims = 5
)

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
	// head is the waterfall head a new version carries module commits forward
	// from.
	head *WaterfallHead

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
	Revision  string
	Requester string
	// RevisionOrderNumber is only set when rebuilding the manifest of an
	// existing version, which carries module commits forward from the
	// version before it instead of from the waterfall head.
	RevisionOrderNumber int
	IngestTime          time.Time
	// TriggeringModuleCommit pins the module with the same owner, repo and
	// branch to the commit that triggered the version.
	TriggeringModuleCommit *manifest.Module
	// WaterfallHead, if set, is used instead of looking up the waterfall head.
	WaterfallHead *WaterfallHead
}

// NewModuleResolver returns a resolver for the module commits of a version.
func NewModuleResolver(projectRef *ProjectRef, opts ModuleResolverOpts) *ModuleResolver {
	return &ModuleResolver{
		projectRef: projectRef,
		opts:       opts,
		resolved:   map[string]*manifest.Module{},
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
	resolved, err := r.resolveAll(ctx, modules)
	if err != nil {
		return nil, err
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

// ClaimRevisionOrderNumber returns the revision order number for the new
// version v. A mainline or push-triggered version also becomes the project's
// waterfall head. If another version replaced the head and that changes the
// revision or module commits v's config was read from, it returns
// ErrWaterfallHeadMoved, and v has to be created again with a new resolver.
func (r *ModuleResolver) ClaimRevisionOrderNumber(ctx context.Context, v *Version, modules ModuleList) (int, error) {
	if !r.claimsWaterfallHead() {
		return GetNewRevisionOrderNumber(ctx, r.projectRef.Id)
	}
	if err := r.init(ctx); err != nil {
		return 0, err
	}
	configModules := r.resolvedModuleNames()
	for range maxWaterfallHeadClaims {
		resolved, err := r.resolveAll(ctx, modules)
		if err != nil {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"message":    "resolving module commits, creating version without replacing the waterfall head",
				"source":     "manifest-creation",
				"version_id": v.Id,
				"project_id": r.projectRef.Id,
			}))
			return GetNewRevisionOrderNumber(ctx, r.projectRef.Id)
		}
		order, err := claimWaterfallHead(ctx, r.projectRef.Id, r.head.ID, WaterfallHead{
			ID:        utility.RandomString(),
			VersionID: v.Id,
			Revision:  v.Revision,
			Modules:   resolved,
		})
		if !errors.Is(err, ErrWaterfallHeadMoved) {
			return order, err
		}
		if err = r.rebase(ctx, v, modules, configModules); err != nil {
			return 0, err
		}
	}
	return 0, ErrWaterfallHeadMoved
}

// rebase moves the resolver to the current waterfall head. It returns
// ErrWaterfallHeadMoved if the new head has a different revision or commit for
// a module v's config was read from, or if it makes the triggering commit
// stale.
func (r *ModuleResolver) rebase(ctx context.Context, v *Version, modules ModuleList, configModules []string) error {
	opts := r.opts
	opts.WaterfallHead = nil
	rebased := NewModuleResolver(r.projectRef, opts)
	if err := rebased.init(ctx); err != nil {
		return err
	}
	if r.opts.TriggeringModuleCommit != nil {
		if rebased.head.Revision != "" && rebased.head.Revision != v.Revision {
			return ErrWaterfallHeadMoved
		}
		stale, err := rebased.IsTriggeringCommitStale(ctx)
		if err != nil {
			return err
		}
		if stale {
			return ErrWaterfallHeadMoved
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, module := range modules {
		if !slices.Contains(configModules, module.Name) {
			continue
		}
		mod, err := rebased.Resolve(ctx, module)
		if err != nil {
			return errors.Wrapf(err, "module '%s'", module.Name)
		}
		if mod.Revision != r.resolved[module.Name].Revision {
			return ErrWaterfallHeadMoved
		}
	}
	r.resolved = rebased.resolved
	r.head = rebased.head
	r.previousManifest = rebased.previousManifest
	return nil
}

// IsTriggeringCommitStale returns whether the waterfall head already has the
// triggering module commit or a newer commit of that module, in which case a
// version for it would repeat the head or move the module backwards.
func (r *ModuleResolver) IsTriggeringCommitStale(ctx context.Context) (bool, error) {
	pin := r.opts.TriggeringModuleCommit
	if pin == nil {
		return false, nil
	}
	if err := r.init(ctx); err != nil {
		return false, err
	}
	if r.previousManifest == nil {
		return false, nil
	}
	for name, mod := range r.previousManifest.Modules {
		if mod == nil || mod.Revision == "" || mod.SelectionRule == manifest.ModuleSelectionPinnedRef || !r.isTriggeringModule(mod.Owner, mod.Repo, mod.Branch) {
			continue
		}
		return mod.Revision == pin.Revision || r.isBehind(ctx, name, mod, pin), nil
	}
	return false, nil
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

func (r *ModuleResolver) resolveAll(ctx context.Context, modules ModuleList) (map[string]*manifest.Module, error) {
	resolved := make(map[string]*manifest.Module, len(modules))
	for _, module := range modules {
		mod, err := r.Resolve(ctx, module)
		if err != nil {
			return nil, errors.Wrapf(err, "module '%s'", module.Name)
		}
		resolved[module.Name] = mod
	}
	return resolved, nil
}

func (r *ModuleResolver) resolvedModuleNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Collect(maps.Keys(r.resolved))
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

// claimsWaterfallHead returns whether the resolver is for a new mainline or
// push-triggered version.
func (r *ModuleResolver) claimsWaterfallHead() bool {
	if r.opts.RevisionOrderNumber != 0 {
		return false
	}
	return r.opts.Requester == evergreen.RepotrackerVersionRequester ||
		(r.opts.Requester == evergreen.TriggerRequester && r.opts.TriggeringModuleCommit != nil)
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
	if r.shouldCarryForward() && r.opts.RevisionOrderNumber != 0 {
		r.previousManifest, err = findPreviousWaterfallManifest(ctx, r.projectRef.Id, r.opts.RevisionOrderNumber)
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":    "finding previous version's manifest, resolving modules without it",
			"source":     "manifest-creation",
			"version_id": r.opts.VersionID,
			"project_id": r.projectRef.Id,
		}))
	} else if r.shouldCarryForward() {
		r.head = r.opts.WaterfallHead
		if r.head == nil {
			r.head, err = FindWaterfallHead(ctx, r.projectRef.Id)
			if err != nil {
				return errors.Wrap(err, "finding waterfall head")
			}
		}
		r.previousManifest = &manifest.Manifest{Id: r.head.VersionID, Modules: r.head.Modules}
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
	if previous.Revision != candidate.Revision && r.isBehind(ctx, moduleName, previous, candidate) {
		grip.Info(ctx, message.Fields{
			"message":            "keeping previous module commit because candidate commit is older",
			"source":             "manifest-creation",
			"version_id":         r.opts.VersionID,
			"project_id":         r.projectRef.Id,
			"module":             moduleName,
			"previous_revision":  previous.Revision,
			"candidate_revision": candidate.Revision,
		})
		return previous
	}
	return candidate
}

// isBehind returns whether candidate is an older commit than previous. If the
// order can't be determined, it returns false.
func (r *ModuleResolver) isBehind(ctx context.Context, moduleName string, previous, candidate *manifest.Module) bool {
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
		grip.Warning(ctx, message.WrapError(err, logFields("checking module commit ingest order, assuming candidate commit is newer")))
		return false
	}
	switch order {
	case ingestOrderCandidateNewer:
		return false
	case ingestOrderPreviousNewer:
		return true
	}

	grip.Info(ctx, logFields("comparing module commits with GitHub"))
	ghCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	status, err := thirdparty.GetCommitComparisonStatus(ghCtx, candidate.Owner, candidate.Repo, previous.Revision, candidate.Revision)
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, logFields("comparing module commits with GitHub, assuming candidate commit is newer")))
		return false
	}
	return status == "behind"
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

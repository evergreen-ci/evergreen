package model

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
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
	Revision   string
	Requester  string
	IngestTime time.Time
	// TriggeringModuleCommit pins the module with the same owner, repo and
	// branch to the commit that triggered the version.
	TriggeringModuleCommit *manifest.Module
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

	mod, err := getManifestModule(ctx, r.projectRef, module, r.opts.Requester, r.opts.IngestTime)
	if err != nil {
		return nil, err
	}
	if isTriggeringModule {
		mod.SelectionRule = manifest.ModuleSelectionTriggerCommit
	}
	return mod, nil
}

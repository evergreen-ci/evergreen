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

	mu           sync.Mutex
	initialized  bool
	expansions   *util.Expansions
	baseManifest *manifest.Manifest
	resolved     map[string]*manifest.Module
}

// ModuleResolverOpts describes the version whose module commits are being
// resolved.
type ModuleResolverOpts struct {
	VersionID              string
	Revision               string
	Requester              string
	IngestTime             time.Time
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
// first use and returning its cached value on later calls.
func (r *ModuleResolver) Resolve(ctx context.Context, module Module) (*manifest.Module, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mod, ok := r.resolved[module.Name]; ok {
		return mod, nil
	}
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	mod, err := r.resolve(ctx, module)
	if err != nil {
		return nil, err
	}
	r.resolved[module.Name] = mod
	return mod, nil
}

// Manifest builds the version's manifest from the resolved module commits.
func (r *ModuleResolver) Manifest(ctx context.Context, v *Version, modules ModuleList) (*manifest.Manifest, error) {
	if len(modules) == 0 {
		return nil, nil
	}
	newManifest := &manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      r.projectRef.Branch,
		IsBase:      v.Requester == evergreen.RepotrackerVersionRequester,
		Modules:     map[string]*manifest.Module{},
	}
	for _, module := range modules {
		mod, err := r.Resolve(ctx, module)
		if err != nil {
			return nil, errors.Wrapf(err, "module '%s'", module.Name)
		}
		newManifest.Modules[module.Name] = mod
	}
	return newManifest, nil
}

// resolvedModulesKey returns the module commits resolved so far as a stable
// string. Concurrent loads of the same revision share a translated
// project, so this keeps loads that read module includes from different commits separate.
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

func (r *ModuleResolver) init(ctx context.Context) error {
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

func (r *ModuleResolver) resolve(ctx context.Context, module Module) (*manifest.Module, error) {
	if r.expansions != nil {
		if err := util.ExpandValues(&module, r.expansions); err != nil {
			return nil, errors.Wrapf(err, "expanding module '%s'", module.Name)
		}
	}
	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)
	}

	if pin := r.opts.TriggeringModuleCommit; pin != nil && owner == pin.Owner && repo == pin.Repo && module.Branch == pin.Branch {
		module.Ref = pin.Revision
	}

	// GitHub wikis are always cloned at default-branch HEAD. Do not reuse
	// base manifest pins; every run must resolve via getManifestModule.
	if r.shouldUseBaseRevision() && !module.AutoUpdate && r.baseManifest != nil && !IsWikiRepo(repo) {
		if baseModule, ok := r.baseManifest.Modules[module.Name]; ok {
			// Use base module revision unless the YAML explicitly specifies a different ref.
			if module.Ref == "" || module.Ref == baseModule.Revision {
				return baseModule, nil
			}
		}
	}

	return getManifestModule(ctx, r.projectRef, module, r.opts.Requester, r.opts.IngestTime)
}

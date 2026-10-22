package model

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

const maxConcurrentModuleResolutions = 10

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

	mu sync.Mutex
	// moduleLocks serializes resolving each module without blocking the
	// others, since resolving can make slow GitHub requests.
	moduleLocks map[string]*sync.Mutex
	resolved    map[string]*manifest.Module
}

// errNoModuleCommits means a module's branch had no commits as of the version's
// ingest time.
var errNoModuleCommits = errors.New("no commits found")

// moduleLookupError is a failure looking up a module's commit that may succeed
// on retry. Any other resolution error is a problem with how the module is
// defined, such as a nonexistent branch or ref.
type moduleLookupError struct{ error }

func (e moduleLookupError) Unwrap() error { return e.error }

// IsModuleLookupError returns whether resolving a module failed in a way that
// may succeed on retry.
func IsModuleLookupError(err error) bool {
	return errors.As(err, &moduleLookupError{})
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
		projectRef:  projectRef,
		opts:        opts,
		moduleLocks: map[string]*sync.Mutex{},
		resolved:    map[string]*manifest.Module{},
	}
}

// Resolve returns the manifest entry for the module, resolving its commit on
// first use and returning the same entry on later calls. Failures aren't
// cached, so a later call can retry.
func (r *ModuleResolver) Resolve(ctx context.Context, module Module) (*manifest.Module, error) {
	r.mu.Lock()
	moduleLock, ok := r.moduleLocks[module.Name]
	if !ok {
		moduleLock = &sync.Mutex{}
		r.moduleLocks[module.Name] = moduleLock
	}
	r.mu.Unlock()

	moduleLock.Lock()
	defer moduleLock.Unlock()

	r.mu.Lock()
	mod, ok := r.resolved[module.Name]
	r.mu.Unlock()
	if ok {
		return mod, nil
	}

	mod, err := r.resolve(ctx, module)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.resolved[module.Name] = mod
	r.mu.Unlock()
	return mod, nil
}

// Manifest returns the version's manifest with the resolved module commits.
// It returns nil if there are no modules.
func (r *ModuleResolver) Manifest(ctx context.Context, v *Version, modules ModuleList) (*manifest.Manifest, error) {
	resolved, err := r.ResolveModules(ctx, modules)
	if err != nil {
		return nil, err
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	return &manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      r.projectRef.Branch,
		IsBase:      v.Requester == evergreen.RepotrackerVersionRequester,
		Modules:     resolved,
	}, nil
}

// ResolveModules resolves the commit of every module in parallel, keyed by
// module name. If any module fails with a lookup error, it returns that error
// so that callers know to retry.
func (r *ModuleResolver) ResolveModules(ctx context.Context, modules ModuleList) (map[string]*manifest.Module, error) {
	if len(modules) == 0 {
		return nil, nil
	}
	resolved := make([]*manifest.Module, len(modules))
	errs := make([]error, len(modules))
	var g errgroup.Group
	g.SetLimit(maxConcurrentModuleResolutions)
	for i, module := range modules {
		g.Go(func() error {
			resolved[i], errs[i] = r.Resolve(ctx, module)
			return nil
		})
	}
	_ = g.Wait()

	var firstErr error
	byName := make(map[string]*manifest.Module, len(modules))
	for i, module := range modules {
		if errs[i] == nil {
			byName[module.Name] = resolved[i]
			continue
		}
		err := errors.Wrapf(errs[i], "module '%s'", module.Name)
		if IsModuleLookupError(err) {
			return nil, err
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return byName, nil
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

// resolveIncludedModules resolves the modules that includes are read from up
// front and in parallel. Module definition problems are left for the includes
// to report as config errors.
func (r *ModuleResolver) resolveIncludedModules(ctx context.Context, modules ModuleList, includes []parserInclude) error {
	if r == nil {
		return nil
	}
	var included ModuleList
	for _, module := range modules {
		if slices.ContainsFunc(includes, func(include parserInclude) bool { return include.Module == module.Name }) {
			included = append(included, module)
		}
	}
	if _, err := r.ResolveModules(ctx, included); IsModuleLookupError(err) {
		return err
	}
	return nil
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
		return moduleLookupError{errors.Wrapf(err, "getting project vars for project '%s'", r.projectRef.Id)}
	}
	if projVars != nil {
		r.expansions = util.NewExpansions(projVars.Vars)
	}

	if r.shouldUseBaseRevision() {
		r.baseManifest, err = manifest.FindFromVersion(ctx, r.opts.VersionID, r.projectRef.Id, r.opts.Revision, r.opts.Requester)
		if err != nil {
			return moduleLookupError{errors.Wrap(err, "getting base manifest")}
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

	if r.isTriggeringModule(owner, repo, module.Branch) {
		module.Ref = r.opts.TriggeringModuleCommit.Revision
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

	mod, err := getManifestModule(ctx, r.projectRef, module, r.opts.Requester, r.opts.IngestTime)
	if err != nil && !isPermanentModuleLookupFailure(err) {
		return nil, moduleLookupError{err}
	}
	return mod, err
}

// isPermanentModuleLookupFailure returns whether GitHub rejected a module
// lookup in a way that retrying won't fix, such as a nonexistent repo, branch
// or ref, or a repo the GitHub app isn't installed on.
func isPermanentModuleLookupFailure(err error) bool {
	if errors.Is(err, githubapp.ErrGitHubAppNotInstalled) || errors.Is(err, errNoModuleCommits) {
		return true
	}
	var apiErr thirdparty.APIRequestError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusUnprocessableEntity)
}

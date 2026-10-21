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
	moduleLocks  map[string]*sync.Mutex
	resolved     map[string]*manifest.Module
	configErrors map[string]error
}

// errNoModuleCommits means a module's branch had no commits as of the version's
// ingest time.
var errNoModuleCommits = errors.New("no commits found")

// moduleConfigError is a problem with how a module is defined, such as a
// nonexistent branch or ref, as opposed to a failure looking up its commit.
type moduleConfigError struct{ error }

// Cause lets errors.Cause reach the underlying GitHub error, which the agent's
// manifest route checks for.
func (e moduleConfigError) Cause() error  { return e.error }
func (e moduleConfigError) Unwrap() error { return e.error }

// moduleLookupError is a failure looking up a module's commit that may succeed
// on retry.
type moduleLookupError struct{ error }

func (e moduleLookupError) Unwrap() error { return e.error }

// IsModuleConfigError returns whether resolving a module failed because of
// how the module is defined.
func IsModuleConfigError(err error) bool {
	return errors.As(err, &moduleConfigError{})
}

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
		projectRef:   projectRef,
		opts:         opts,
		moduleLocks:  map[string]*sync.Mutex{},
		resolved:     map[string]*manifest.Module{},
		configErrors: map[string]error{},
	}
}

// Resolve returns the manifest entry for the module, resolving its commit on
// first use and returning the same result on later calls. Lookup failures
// aren't cached, so a later call can retry them.
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
	mod, resolved := r.resolved[module.Name]
	configErr := r.configErrors[module.Name]
	r.mu.Unlock()
	if resolved {
		return mod, nil
	}
	if configErr != nil {
		return nil, configErr
	}

	mod, err := r.resolve(ctx, module)
	r.mu.Lock()
	defer r.mu.Unlock()
	if IsModuleConfigError(err) {
		r.configErrors[module.Name] = err
	}
	if err != nil {
		return nil, err
	}
	r.resolved[module.Name] = mod
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
// module name. If modules fail for different reasons, it prefers returning a
// lookup failure over a config error so that callers can retry.
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

	var configErr error
	byName := make(map[string]*manifest.Module, len(modules))
	for i, module := range modules {
		if errs[i] == nil {
			byName[module.Name] = resolved[i]
			continue
		}
		err := errors.Wrapf(errs[i], "module '%s'", module.Name)
		if !IsModuleConfigError(err) {
			return nil, moduleLookupError{err}
		}
		if configErr == nil {
			configErr = err
		}
	}
	if configErr != nil {
		return nil, configErr
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
	var g errgroup.Group
	g.SetLimit(maxConcurrentModuleResolutions)
	seen := map[string]bool{}
	for _, include := range includes {
		if include.Module == "" || seen[include.Module] {
			continue
		}
		seen[include.Module] = true
		module, err := GetModuleByName(modules, include.Module)
		if err != nil {
			continue
		}
		g.Go(func() error {
			_, err := r.Resolve(ctx, *module)
			if err != nil && !IsModuleConfigError(err) {
				return moduleLookupError{errors.Wrapf(err, "module '%s'", module.Name)}
			}
			return nil
		})
	}
	return g.Wait()
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
		return module, moduleConfigError{errors.Wrapf(err, "expanding module '%s'", module.Name)}
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
		return nil, moduleConfigError{errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)}
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
	if isPermanentModuleLookupFailure(err) {
		return nil, moduleConfigError{err}
	}
	return mod, err
}

// isPermanentModuleLookupFailure returns whether GitHub rejected a module
// lookup in a way that retrying won't fix, such as a nonexistent repo, branch
// or ref, or a repo the GitHub app isn't installed on.
func isPermanentModuleLookupFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, githubapp.ErrGitHubAppNotInstalled) || errors.Is(err, errNoModuleCommits) {
		return true
	}
	var apiErr thirdparty.APIRequestError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusUnprocessableEntity)
}

package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleResolver(t *testing.T) {
	ingestTime := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	const (
		olderModuleRevision = "olderModuleRevision"
		newerModuleRevision = "newerModuleRevision"
	)
	projectRef := &ProjectRef{
		Id:     "resolver-project",
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "main",
	}
	module := Module{
		Name:   "sample",
		Owner:  "evergreen-ci",
		Repo:   "sample",
		Branch: "main",
	}

	for tName, tCase := range map[string]func(t *testing.T){
		"ResolveShouldKeepFirstCommitAfterNewerCommitIsIngested": func(t *testing.T) {
			require.NoError(t, UpsertRepositoryRevision(t.Context(), module.Owner, module.Repo, module.Branch, olderModuleRevision, ingestTime.Add(-time.Minute)))
			resolver := NewModuleResolver(projectRef, ModuleResolverOpts{
				Requester:  evergreen.RepotrackerVersionRequester,
				IngestTime: ingestTime,
			})

			mod, err := resolver.Resolve(t.Context(), module)
			require.NoError(t, err)
			require.NotNil(t, mod)
			assert.Equal(t, olderModuleRevision, mod.Revision)

			require.NoError(t, UpsertRepositoryRevision(t.Context(), module.Owner, module.Repo, module.Branch, newerModuleRevision, ingestTime))

			revision, err := getRevisionForRemoteModule(t.Context(), module, module.Name, GetProjectOpts{ModuleResolver: resolver})
			require.NoError(t, err)
			assert.Equal(t, olderModuleRevision, revision, "includes should be read from the already resolved commit")

			v := &Version{Id: "v", Identifier: projectRef.Id, Requester: evergreen.RepotrackerVersionRequester}
			mfst, err := resolver.Manifest(t.Context(), v, ModuleList{module})
			require.NoError(t, err)
			require.NotNil(t, mfst)
			require.Contains(t, mfst.Modules, module.Name)
			assert.Equal(t, olderModuleRevision, mfst.Modules[module.Name].Revision, "manifest should use the already resolved commit")
		},
		"ResolveShouldExpandModuleFieldsWithProjectVars": func(t *testing.T) {
			require.NoError(t, (&ProjectVars{Id: projectRef.Id, Vars: map[string]string{"module_branch": "main"}}).Insert(t.Context()))
			require.NoError(t, UpsertRepositoryRevision(t.Context(), module.Owner, module.Repo, module.Branch, olderModuleRevision, ingestTime))
			resolver := NewModuleResolver(projectRef, ModuleResolverOpts{
				Requester:  evergreen.RepotrackerVersionRequester,
				IngestTime: ingestTime,
			})

			unexpanded := module
			unexpanded.Branch = "${module_branch}"
			mod, err := resolver.Resolve(t.Context(), unexpanded)
			require.NoError(t, err)
			require.NotNil(t, mod)
			assert.Equal(t, "main", mod.Branch)
			assert.Equal(t, olderModuleRevision, mod.Revision)
		},
		"ResolveForPeriodicBuildShouldUseBaseManifestCommit": func(t *testing.T) {
			require.NoError(t, UpsertRepositoryRevision(t.Context(), module.Owner, module.Repo, module.Branch, newerModuleRevision, ingestTime))
			baseManifest := manifest.Manifest{
				Id:          "base-version",
				Revision:    "base-revision",
				ProjectName: projectRef.Id,
				IsBase:      true,
				Modules: map[string]*manifest.Module{
					module.Name: {Owner: module.Owner, Repo: module.Repo, Branch: module.Branch, Revision: olderModuleRevision},
				},
			}
			require.NoError(t, baseManifest.Insert(t.Context()))
			resolver := NewModuleResolver(projectRef, ModuleResolverOpts{
				Revision:  "base-revision",
				Requester: evergreen.AdHocRequester,
			})

			mod, err := resolver.Resolve(t.Context(), module)
			require.NoError(t, err)
			require.NotNil(t, mod)
			assert.Equal(t, olderModuleRevision, mod.Revision)
		},
		"GetRevisionForRemoteModuleShouldFallBackToBranchForWiki": func(t *testing.T) {
			resolver := NewModuleResolver(projectRef, ModuleResolverOpts{Requester: evergreen.RepotrackerVersionRequester})
			wiki := Module{Name: "wiki", Owner: "evergreen-ci", Repo: "sample.wiki", Branch: "master"}

			revision, err := getRevisionForRemoteModule(t.Context(), wiki, wiki.Name, GetProjectOpts{ModuleResolver: resolver})
			require.NoError(t, err)
			assert.Equal(t, wiki.Branch, revision)
		},
		"ResolvedModulesKeyShouldBeSortedByModuleName": func(t *testing.T) {
			resolver := NewModuleResolver(projectRef, ModuleResolverOpts{})
			resolver.resolved["b"] = &manifest.Module{Revision: "rev-b"}
			resolver.resolved["a"] = &manifest.Module{Revision: "rev-a"}
			assert.Equal(t, "a@rev-a,b@rev-b", resolver.resolvedModulesKey())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, RepositoryRevisionsHistoryCollection, ProjectVarsCollection, manifest.Collection))
			t.Cleanup(func() {
				assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepositoryRevisionsHistoryCollection, ProjectVarsCollection, manifest.Collection))
			})
			require.NoError(t, projectRef.Insert(t.Context()))
			tCase(t)
		})
	}
}

func TestModuleIncludeAndManifestUseSameModuleCommit(t *testing.T) {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings)
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepositoryRevisionsHistoryCollection, ProjectVarsCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepositoryRevisionsHistoryCollection, ProjectVarsCollection))
	})

	const (
		commitWithoutModules = "b27779f856b211ffaf97cbc124b7082a20ea8bc0"
		commitWithModules    = "cf46076567e4949f9fc68e0634139d4ac495c89b"
	)
	projectRef := &ProjectRef{
		Id:         "module-include-project",
		Identifier: "module-include-project",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "main",
	}
	require.NoError(t, projectRef.Insert(t.Context()))
	mainConfig := []byte(`
modules:
- name: sample
  owner: evergreen-ci
  repo: sample
  branch: main
include:
- filename: evergreen.yml
  module: sample
`)

	for _, tc := range []struct {
		name               string
		triggeringRevision string
		moduleRevision     string
		expectRenderModule bool
	}{
		{name: "IncludeShouldBeReadFromIngestedCommit", moduleRevision: commitWithoutModules},
		{name: "IncludeShouldBeReadFromTriggeringCommit", triggeringRevision: commitWithModules, moduleRevision: commitWithModules, expectRenderModule: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(RepositoryRevisionsHistoryCollection))
			ingestTime := time.Now()
			require.NoError(t, UpsertRepositoryRevision(t.Context(), "evergreen-ci", "sample", "main", commitWithoutModules, ingestTime.Add(-time.Minute)))
			opts := ModuleResolverOpts{
				Requester:  evergreen.RepotrackerVersionRequester,
				IngestTime: ingestTime,
			}
			if tc.triggeringRevision != "" {
				opts.Requester = evergreen.TriggerRequester
				opts.TriggeringModuleCommit = &manifest.Module{Owner: "evergreen-ci", Repo: "sample", Branch: "main", Revision: tc.triggeringRevision}
			}
			resolver := NewModuleResolver(projectRef, opts)

			p := &Project{}
			_, err := LoadProjectInto(t.Context(), mainConfig, &GetProjectOpts{
				Ref:            projectRef,
				Revision:       "main",
				ReadFileFrom:   ReadFromGithub,
				ModuleResolver: resolver,
			}, projectRef.Id, p)
			require.NoError(t, err)
			_, err = GetModuleByName(p.Modules, "render-module")
			assert.Equal(t, tc.expectRenderModule, err == nil, "include should be read from the resolved module commit")
			require.NoError(t, UpsertRepositoryRevision(t.Context(), "evergreen-ci", "sample", "main", "88dcc12106a40cb4917f552deab7574ececd9a3e", ingestTime))
			require.NotEmpty(t, p.Modules)
			mfst, err := resolver.Manifest(t.Context(), &Version{Id: tc.name, Identifier: projectRef.Id}, ModuleList{p.Modules[0]})
			require.NoError(t, err)
			require.NotNil(t, mfst)
			require.Contains(t, mfst.Modules, "sample")
			assert.Equal(t, tc.moduleRevision, mfst.Modules["sample"].Revision)
		})
	}
}

package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type ProjectAliasSuite struct {
	suite.Suite
	aliases []ProjectAlias
}

func TestProjectAliasSuite(t *testing.T) {
	s := &ProjectAliasSuite{}
	suite.Run(t, s)
}

func (s *ProjectAliasSuite) SetupTest() {
	s.Require().NoError(db.Clear(ProjectAliasCollection))
	s.aliases = []ProjectAlias{}
	for i := 0; i < 10; i++ {
		s.aliases = append(s.aliases, ProjectAlias{
			ProjectID:   fmt.Sprintf("project-%d", i),
			Alias:       fmt.Sprintf("alias-%d", i),
			Variant:     fmt.Sprintf("variant-%d", i),
			Task:        fmt.Sprintf("task-%d", i),
			Description: fmt.Sprintf("description-%d", i),
		})
	}
}

func (s *ProjectAliasSuite) TestInsertTaskAndVariantWithNoTags() {
	for _, a := range s.aliases {
		s.NoError(a.Upsert(s.T().Context()))
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{projectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQContext(s.T().Context(), ProjectAliasCollection, q, &out))
		s.Equal(a.ProjectID, out.ProjectID)
		s.Equal(a.Alias, out.Alias)
		s.Equal(a.Variant, out.Variant)
		s.Equal(a.Task, out.Task)
		s.Equal(a.Description, out.Description)
	}
}

func (s *ProjectAliasSuite) TestInsertTagsAndNoTask() {
	tags := []string{"tag1", "tag2"}
	for _, alias := range s.aliases {
		aliasCopy := alias
		aliasCopy.Task = ""
		aliasCopy.TaskTags = tags
		s.NoError(aliasCopy.Upsert(s.T().Context()))
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{projectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQContext(s.T().Context(), ProjectAliasCollection, q, &out))
		s.Equal(a.ProjectID, out.ProjectID)
		s.Equal(a.Alias, out.Alias)
		s.Equal(a.Variant, out.Variant)
		s.Empty(out.VariantTags)
		s.Equal("", out.Task)
		s.Equal(tags, out.TaskTags)
		s.Equal(a.Description, out.Description)
	}
}

func (s *ProjectAliasSuite) TestHasMatchingGitTagAliasAndRemotePath() {
	newAlias := ProjectAlias{
		ProjectID: "project_id",
		Alias:     evergreen.GitTagAlias,
		GitTag:    "release",
		Variant:   "variant",
		Task:      "task",
	}
	s.NoError(newAlias.Upsert(s.T().Context()))
	newAlias2 := ProjectAlias{
		ProjectID:  "project_id",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "file.yml",
	}
	s.NoError(newAlias2.Upsert(s.T().Context()))
	hasAliases, path, err := HasMatchingGitTagAliasAndRemotePath(s.T().Context(), "project_id", "release")
	s.Error(err)
	s.False(hasAliases)
	s.Empty(path)

	newAlias2.RemotePath = ""
	s.NoError(newAlias2.Upsert(s.T().Context()))
	hasAliases, path, err = HasMatchingGitTagAliasAndRemotePath(s.T().Context(), "project_id", "release")
	s.NoError(err)
	s.True(hasAliases)
	s.Empty(path)

	hasAliases, path, err = HasMatchingGitTagAliasAndRemotePath(s.T().Context(), "project_id2", "release")
	s.Error(err)
	s.False(hasAliases)
	s.Empty(path)

	newAlias3 := ProjectAlias{
		ProjectID:  "project_id2",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "file.yml",
	}
	s.NoError(newAlias3.Upsert(s.T().Context()))
	hasAliases, path, err = HasMatchingGitTagAliasAndRemotePath(s.T().Context(), "project_id2", "release")
	s.NoError(err)
	s.True(hasAliases)
	s.Equal("file.yml", path)
}

func (s *ProjectAliasSuite) TestInsertTagsAndNoVariant() {
	tags := []string{"tag1", "tag2"}
	for _, alias := range s.aliases {
		aliasCopy := alias
		aliasCopy.Variant = ""
		aliasCopy.VariantTags = tags
		s.NoError(aliasCopy.Upsert(s.T().Context()))
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{projectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQContext(s.T().Context(), ProjectAliasCollection, q, &out))
		s.Equal(a.ProjectID, out.ProjectID)
		s.Equal(a.Alias, out.Alias)
		s.Equal(a.Task, out.Task)
		s.Empty(out.TaskTags)
		s.Equal("", out.Variant)
		s.Equal(tags, out.VariantTags)
		s.Equal(a.Description, out.Description)
	}
}

func (s *ProjectAliasSuite) TestRemove() {
	for i, a := range s.aliases {
		s.NoError(a.Upsert(s.T().Context()))
		s.aliases[i] = a
	}
	var out []ProjectAlias
	q := db.Query(bson.M{})
	s.NoError(db.FindAllQ(s.T().Context(), ProjectAliasCollection, q, &out))
	s.Len(out, 10)

	for i, a := range s.aliases {
		s.NoError(RemoveProjectAlias(s.T().Context(), a.ID.Hex()))
		s.NoError(db.FindAllQ(s.T().Context(), ProjectAliasCollection, q, &out))
		s.Len(out, 10-i-1)
	}
}

func (s *ProjectAliasSuite) TestFindAliasesForProject() {
	for _, a := range s.aliases {
		s.NoError(a.Upsert(s.T().Context()))
	}
	a1 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-1",
		Variant:   "variants-111",
		Task:      "variants-11",
	}
	s.NoError(a1.Upsert(s.T().Context()))

	out, err := FindAliasesForProjectFromDb(s.T().Context(), "project-1")
	s.NoError(err)
	s.Len(out, 2)
}

func (s *ProjectAliasSuite) TestFindAliasInProject() {
	for _, a := range s.aliases {
		s.NoError(a.Upsert(s.T().Context()))
	}
	a1 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-1",
		Variant:   "variants-11",
		Task:      "variants-11",
	}
	a2 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-2",
		Variant:   "variants-11",
		Task:      "variants-11",
	}
	a3 := ProjectAlias{
		ProjectID: "project-2",
		Alias:     "alias-1",
		Variant:   "variants-11",
		Task:      "variants-11",
	}
	s.NoError(a1.Upsert(s.T().Context()))
	s.NoError(a2.Upsert(s.T().Context()))
	s.NoError(a3.Upsert(s.T().Context()))

	found, err := findMatchingAliasForProjectRef(s.T().Context(), "project-1", "alias-1")
	s.NoError(err)
	s.Len(found, 2)
}

func (s *ProjectAliasSuite) TestFindAliasInProjectOrConfig() {
	s.Require().NoError(db.ClearCollections(ProjectAliasCollection, ProjectConfigCollection, ProjectRefCollection))
	pRef := ProjectRef{
		Id:                    "project-1",
		RepoRefId:             "r1",
		VersionControlEnabled: utility.TruePtr(),
	}
	s.NoError(pRef.Replace(s.T().Context()))
	a1 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     evergreen.CommitQueueAlias,
	}
	a2 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     evergreen.CommitQueueAlias,
	}
	a3 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     evergreen.GithubPRAlias,
	}
	patchAlias := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-0",
	}
	duplicateAlias := ProjectAlias{
		ProjectID:   "project-1",
		Alias:       "duplicate",
		Description: "from UI",
	}
	s.NoError(a1.Upsert(s.T().Context()))
	s.NoError(a2.Upsert(s.T().Context()))
	s.NoError(a3.Upsert(s.T().Context()))
	s.NoError(patchAlias.Upsert(s.T().Context()))
	s.NoError(duplicateAlias.Upsert(s.T().Context()))

	projectConfig := &ProjectConfig{
		Id:      "project-1",
		Project: "project-1",
		ProjectConfigFields: ProjectConfigFields{
			PatchAliases: []ProjectAlias{
				{
					ID:    mgobson.NewObjectId(),
					Alias: "alias-2",
				},
				{
					ID:    mgobson.NewObjectId(),
					Alias: "alias-1",
				},
				{
					ID:          mgobson.NewObjectId(),
					Alias:       "duplicate",
					Description: "from project config",
				},
			},
			CommitQueueAliases: []ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
				},
			},
			GitHubChecksAliases: []ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
				},
			},
		}}
	s.NoError(projectConfig.Insert(s.T().Context()))

	projectAliases, err := FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", evergreen.CommitQueueAlias)
	s.NoError(err)
	s.Len(projectAliases, 2)

	projectAliases, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", evergreen.GithubPRAlias)
	s.NoError(err)
	s.Len(projectAliases, 1)

	projectAliases, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", evergreen.GithubChecksAlias)
	s.NoError(err)
	s.Len(projectAliases, 1)

	projectAliases, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", "alias-0")
	s.NoError(err)
	s.Len(projectAliases, 1)
	projectAliases, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", "alias-1")
	s.NoError(err)
	s.Len(projectAliases, 1)
	projectAliases, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", "alias-2")
	s.NoError(err)
	s.Len(projectAliases, 1)

	// If the same alias is defined in both UI and project config,
	// UI takes precedence.
	projectAliases, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "project-1", "duplicate")
	s.NoError(err)
	s.Len(projectAliases, 1)
	s.Equal("from UI", projectAliases[0].Description)

}

func TestFindMergedAliasesFromProjectRepoOrProjectConfig(t *testing.T) {
	pRef := ProjectRef{
		Id:                    "p1",
		RepoRefId:             "r1",
		VersionControlEnabled: utility.TruePtr(),
		GithubChecksEnabled:   utility.TruePtr(),
	}
	cqAliases := []ProjectAlias{
		{
			Alias:       evergreen.CommitQueueAlias,
			Description: "first",
		},
		{
			Alias:       evergreen.CommitQueueAlias,
			Description: "second",
		},
	}
	gitTagAliases := []ProjectAlias{
		{
			Alias:       evergreen.GitTagAlias,
			Description: "first",
		},
		{
			Alias:       evergreen.GitTagAlias,
			Description: "second",
		},
	}
	patchAliases := []ProjectAlias{
		{
			Alias: "something rad",
		},
		{
			Alias: "something dastardly",
		},
	}
	githubChecksAlias := []ProjectAlias{
		{
			Alias: evergreen.GithubChecksAlias,
		},
	}
	projectConfig := ProjectConfig{ProjectConfigFields: ProjectConfigFields{
		PatchAliases: []ProjectAlias{
			{
				Alias: "something cool",
			},
		},
		CommitQueueAliases: []ProjectAlias{
			{
				Alias: "something useless",
			},
		},
	}}

	for testName, testCase := range map[string]func(t *testing.T){
		"nothing enabled": func(t *testing.T) {
			assert.NoError(t, UpsertAliasesForProject(t.Context(), cqAliases, pRef.Id))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), githubChecksAlias, pRef.Id))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), gitTagAliases, pRef.RepoRefId))
			tempRef := ProjectRef{ // This ref has nothing else enabled so merging should only return project aliases
				Id: pRef.Id,
			}
			res, err := ConstructMergedAliasesByPrecedence(t.Context(), &tempRef, &projectConfig, "")
			assert.NoError(t, err)
			require.Len(t, res, 2)
			assert.Equal(t, res[0].ProjectID, pRef.Id)
			assert.Equal(t, res[1].ProjectID, pRef.Id)
		},
		"all enabled": func(t *testing.T) {
			assert.NoError(t, UpsertAliasesForProject(t.Context(), cqAliases, pRef.Id))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), cqAliases, pRef.RepoRefId))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), gitTagAliases, pRef.RepoRefId))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), githubChecksAlias, pRef.RepoRefId))
			res, err := ConstructMergedAliasesByPrecedence(t.Context(), &pRef, &projectConfig, pRef.RepoRefId)
			assert.NoError(t, err)
			// Uses aliases from project, repo, and config
			require.Len(t, res, 6)
			cqCount := 0
			// There should only be two commit queue aliases, and they should all be from the project
			for _, a := range res {
				if a.Alias == evergreen.CommitQueueAlias {
					cqCount++
					assert.Equal(t, a.ProjectID, pRef.Id)
					assert.Equal(t, AliasSourceProject, a.Source)
				} else if a.Alias == evergreen.GitTagAlias {
					assert.Equal(t, a.ProjectID, pRef.RepoRefId)
					assert.Equal(t, AliasSourceRepo, a.Source)
				} else if a.Alias == evergreen.GithubChecksAlias {
					assert.Equal(t, AliasSourceRepo, a.Source)
				} else {
					assert.Equal(t, AliasSourceConfig, a.Source)
				}
			}
			assert.Equal(t, 2, cqCount)
		},
		"project and repo only used": func(t *testing.T) {
			assert.NoError(t, UpsertAliasesForProject(t.Context(), cqAliases, pRef.Id))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), cqAliases, pRef.RepoRefId))
			assert.NoError(t, UpsertAliasesForProject(t.Context(), patchAliases, pRef.RepoRefId))
			res, err := ConstructMergedAliasesByPrecedence(t.Context(), &pRef, &projectConfig, pRef.RepoRefId)
			assert.NoError(t, err)
			// Ignores config aliases because they're already used
			require.Len(t, res, 4)
			cqCount := 0
			patchCount := 0
			for _, a := range res {
				if a.Alias == evergreen.CommitQueueAlias {
					cqCount++
					assert.Equal(t, a.ProjectID, pRef.Id)
					assert.Equal(t, AliasSourceProject, a.Source)
				} else {
					patchCount++
					assert.Equal(t, a.ProjectID, pRef.RepoRefId)
					assert.Equal(t, AliasSourceRepo, a.Source)
				}
			}
			assert.Equal(t, 2, cqCount)
			assert.Equal(t, 2, patchCount)
		},
	} {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection,
			ProjectConfigCollection, ProjectAliasCollection))

		t.Run(testName, testCase)
	}
}

func (s *ProjectAliasSuite) TestFindAliasInProjectRepoOrConfig() {
	s.Require().NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectConfigCollection))

	repoRef := RepoRef{ProjectRef{
		Id:    "repo_ref",
		Owner: "mongodb",
		Repo:  "test_repo",
	}}
	pRef1 := ProjectRef{
		Id:        "p1",
		RepoRefId: repoRef.Id,
	}
	pRef2 := ProjectRef{
		Id:        "p2",
		RepoRefId: repoRef.Id,
	}
	projectConfig := ProjectConfig{
		Project: pRef1.Id,
		ProjectConfigFields: ProjectConfigFields{
			PatchAliases: []ProjectAlias{
				{
					// This alias should be ignored because it's already defined at the higher project level
					Alias:   "alias-3",
					Variant: "*",
					Task:    "*",
				},
				{
					// This alias should not be ignored because it's not defined at any higher level
					Alias:   "alias-6",
					Variant: "*",
					Task:    "*",
				},
			},
			CommitQueueAliases: []ProjectAlias{
				{
					Variant: "cq-.*",
					Task:    "cq-.*",
				},
			},
		}}
	s.NoError(repoRef.Replace(s.T().Context()))
	s.NoError(pRef1.Replace(s.T().Context()))
	s.NoError(pRef2.Replace(s.T().Context()))
	s.NoError(projectConfig.Insert(s.T().Context()))

	for i := 0; i < 3; i++ {
		alias := ProjectAlias{
			ProjectID: repoRef.Id,
			Alias:     "alias-1",
			Variant:   "variants-11",
			Task:      "variants-11",
		}
		if i%2 != 0 {
			alias.Alias = "alias-2"
		}
		s.NoError(alias.Upsert(s.T().Context()))
	}

	for i := 0; i < 6; i++ {
		alias := ProjectAlias{
			ProjectID: pRef1.Id,
			Alias:     "alias-3",
			Variant:   "variants-11",
			Task:      "variants-11",
		}
		if i%2 == 0 {
			alias.Alias = "alias-4"
		}
		s.NoError(alias.Upsert(s.T().Context()))
	}

	// Test project with aliases
	found, err := FindAliasInProjectRepoOrConfig(s.T().Context(), pRef1.Id, "alias-3")
	s.NoError(err)
	s.Len(found, 3)

	// Test project without aliases; parent repo has aliases
	found, err = FindAliasInProjectRepoOrConfig(s.T().Context(), pRef2.Id, "alias-1")
	s.NoError(err)
	s.Len(found, 2)

	// Test non-existent project
	found, err = FindAliasInProjectRepoOrConfig(s.T().Context(), "bad-project", "alias-1")
	s.Error(err)
	s.Empty(found)

	// Test no aliases found
	found, err = FindAliasInProjectRepoOrConfig(s.T().Context(), pRef1.Id, "alias-5")
	s.NoError(err)
	s.Empty(found)

	// Test project config
	found, err = FindAliasInProjectRepoOrConfig(s.T().Context(), pRef1.Id, "alias-6")
	s.NoError(err)
	s.Require().Len(found, 1)
	s.Equal("alias-6", found[0].Alias)
	s.Equal("*", found[0].Task)
	s.Equal("*", found[0].Variant)

	// Test non-patch aliases defined in config
	found, err = FindAliasInProjectRepoOrConfig(s.T().Context(), pRef1.Id, evergreen.CommitQueueAlias)
	s.NoError(err)
	s.Require().Len(found, 1)
	s.Equal(evergreen.CommitQueueAlias, found[0].Alias)
	s.Equal("cq-.*", found[0].Task)
	s.Equal("cq-.*", found[0].Variant)
}

func (s *ProjectAliasSuite) TestUpsertAliasesForProject() {
	for _, a := range s.aliases {
		a.ProjectID = "old-project"

		s.NoError(a.Upsert(s.T().Context()))
	}
	s.NoError(UpsertAliasesForProject(s.T().Context(), s.aliases, "new-project"))

	found, err := FindAliasesForProjectFromDb(s.T().Context(), "new-project")
	s.NoError(err)
	s.Len(found, 10)

	// verify old aliases not overwritten
	found, err = FindAliasesForProjectFromDb(s.T().Context(), "old-project")
	s.NoError(err)
	s.Len(found, 10)
}

func TestProjectAliasGitTagMatching(t *testing.T) {
	aliases := ProjectAliases{
		{Alias: "one", Variant: "bv1", Task: "t1", GitTag: "tag-."},
		{Alias: "two", Variant: "bv2", Task: "t2"},
		{Alias: "three", Variant: "bv3", TaskTags: []string{"tag3"}},
		{Alias: "four", VariantTags: []string{"variantTag"}, TaskTags: []string{"tag4"}},
		{Alias: "five", Variant: "bv4", TaskTags: []string{"!tag3", "tag5"}},
		{Alias: "six", Variant: "bv4", TaskTags: []string{"!tag3 tag4"}},
	}

	t.Run("GitTagNameMatchesGitTagRegexp", func(t *testing.T) {
		match, err := aliases.HasMatchingGitTag("tag-1")
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("GitTagNameDoesNotMatchGitTagRegexp", func(t *testing.T) {
		match, err := aliases.HasMatchingGitTag("tag1")
		assert.NoError(t, err)
		assert.False(t, match)
	})
}

func TestProjectAliasVariantMatching(t *testing.T) {
	t.Run("MatchesVariantRegexp", func(t *testing.T) {
		a := ProjectAlias{Alias: "one", Variant: "bv1"}
		match, err := a.HasMatchingVariant("bv12345", nil)
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("DoesNotMatchVariantRegexp", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", Variant: "bv1"}
		match, err := a.HasMatchingVariant("nonexistent", nil)
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("DoesNotMatchVariantTags", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", VariantTags: []string{"tag3"}}
		match, err := a.HasMatchingVariant("", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("DoesNotMatchVariantRegexpOrTag", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", Variant: "v1"}
		match, err := a.HasMatchingVariant("nonexistent", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("MatchesVariantTagButNotRegexp", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", VariantTags: []string{"tag1"}}
		match, err := a.HasMatchingVariant("nonexistent", []string{"tag1"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("MatchesAtLeastOneVariantTag", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", VariantTags: []string{"tag1"}}
		match, err := a.HasMatchingVariant("nonexistent", []string{"nonexistent", "tag1"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("MatchesVariantTagWithMultipleCriteria", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", VariantTags: []string{"!tag1 tag2 tag3"}}
		match, err := a.HasMatchingVariant("nonexistent", []string{"tag2", "tag3", "tag4"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("DoesNotMatchVariantTagWithMultipleCriteria", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", VariantTags: []string{"!tag1 tag2 tag3"}}
		match, err := a.HasMatchingVariant("nonexistent", []string{"tag2"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("DoesNotMatchVariantTagWithMultipleCriteriaAndNegation", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", VariantTags: []string{"!tag1 tag2 tag3"}}
		match, err := a.HasMatchingVariant("nonexistent", []string{"tag1", "tag2", "tag3"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("MatchesVariantRegexpButNotTag", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", Variant: "v2"}
		match, err := a.HasMatchingVariant("v2", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("DoesNotMatchEmpty", func(t *testing.T) {
		regexpAlias := ProjectAlias{Alias: "alias", Variant: "v1"}
		match, err := regexpAlias.HasMatchingVariant("", nil)
		assert.NoError(t, err)
		assert.False(t, match)

		tagAlias := ProjectAlias{Alias: "alias", VariantTags: []string{"tag"}}
		match, err = tagAlias.HasMatchingVariant("", nil)
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("MatchesAtLeastOneAlias", func(t *testing.T) {
		aliases := ProjectAliases{
			{Alias: "one", Variant: "v1", VariantTags: []string{"tag1"}},
			{Alias: "two", Variant: "v2"},
		}
		matches, err := aliases.AliasesMatchingVariant("v1", []string{"nonexistent"})
		assert.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, aliases[0], matches[0])
	})
}

func TestProjectAliasTaskMatching(t *testing.T) {
	t.Run("MatchesTaskRegexp", func(t *testing.T) {
		a := ProjectAlias{Alias: "one", Task: "t1"}
		match, err := a.HasMatchingTask("t12345", nil)
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("DoesNotMatchTaskRegexp", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", Task: "t1"}
		match, err := a.HasMatchingTask("nonexistent", nil)
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("DoesNotMatchTaskTags", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", TaskTags: []string{"tag3"}}
		match, err := a.HasMatchingTask("", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("DoesNotMatchTaskRegexpOrTag", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", Task: "t1"}
		match, err := a.HasMatchingTask("nonexistent", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("MatchesTaskTagButNotRegexp", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", TaskTags: []string{"tag3"}}
		match, err := a.HasMatchingTask("nonexistent", []string{"tag3"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("MatchesAtLeastOneTaskTag", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", TaskTags: []string{"tag3"}}
		match, err := a.HasMatchingTask("nonexistent", []string{"nonexistent", "tag3"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("MatchesTaskTagWithMultipleCriteria", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", TaskTags: []string{"!tag1 tag2 tag3"}}
		match, err := a.HasMatchingTask("nonexistent", []string{"tag2", "tag3", "tag4"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("DoesNotMatchTaskTagWithMultipleCriteria", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", TaskTags: []string{"!tag1 tag2 tag3"}}
		match, err := a.HasMatchingTask("nonexistent", []string{"tag2"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("DoesNotMatchTaskTagWithMultipleCriteriaAndNegation", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", TaskTags: []string{"!tag1 tag2 tag3"}}
		match, err := a.HasMatchingTask("nonexistent", []string{"tag1", "tag2", "tag3"})
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("MatchesTaskRegexpButNotTag", func(t *testing.T) {
		a := ProjectAlias{Alias: "alias", Task: "t2"}
		match, err := a.HasMatchingTask("t2", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("DoesNotMatchEmpty", func(t *testing.T) {
		regexpAlias := ProjectAlias{Alias: "alias", Task: "t1"}
		match, err := regexpAlias.HasMatchingTask("", nil)
		assert.NoError(t, err)
		assert.False(t, match)

		tagAlias := ProjectAlias{Alias: "alias", TaskTags: []string{"tag"}}
		match, err = tagAlias.HasMatchingTask("", nil)
		assert.NoError(t, err)
		assert.False(t, match)
	})
	t.Run("MatchesAtLeastOneAlias", func(t *testing.T) {
		aliases := ProjectAliases{
			{Alias: "one", TaskTags: []string{"tag1"}},
			{Alias: "two", Task: "t1"},
		}
		match, err := aliases.HasMatchingTask("t1", []string{"nonexistent"})
		assert.NoError(t, err)
		assert.True(t, match)
	})
}

func TestValidateGitTagAlias(t *testing.T) {
	a := ProjectAlias{Alias: "one"}
	errs := validateGitTagAlias(a, "gitTag", 1)
	assert.NotEmpty(t, errs)

	a.GitTag = "#$!)"
	errs = validateGitTagAlias(a, "gitTag", 1)
	assert.NotEmpty(t, errs)

	a.GitTag = "tag-1"
	errs = validateGitTagAlias(a, "gitTag", 1)
	assert.NotEmpty(t, errs)

	a.RemotePath = "hello.yml"
	a.Variant = "variant-also-defined"
	errs = validateGitTagAlias(a, "gitTag", 1)
	assert.NotEmpty(t, errs)

	a.RemotePath = ""
	a.Variant = ""
	errs = validateGitTagAlias(a, "gitTag", 1)
	assert.NotEmpty(t, errs)

	a.RemotePath = "hello.yml"
	errs = validateGitTagAlias(a, "gitTag", 1)
	assert.Empty(t, errs)
}

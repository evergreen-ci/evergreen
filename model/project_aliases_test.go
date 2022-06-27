package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/stretchr/testify/assert"
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
		s.NoError(a.Upsert())
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{projectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQ(ProjectAliasCollection, q, &out))
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
		s.NoError(aliasCopy.Upsert())
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{projectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQ(ProjectAliasCollection, q, &out))
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
	s.NoError(newAlias.Upsert())
	newAlias2 := ProjectAlias{
		ProjectID:  "project_id",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "file.yml",
	}
	s.NoError(newAlias2.Upsert())
	hasAliases, path, err := HasMatchingGitTagAliasAndRemotePath("project_id", "release")
	s.Error(err)
	s.False(hasAliases)
	s.Empty(path)

	newAlias2.RemotePath = ""
	s.NoError(newAlias2.Upsert())
	hasAliases, path, err = HasMatchingGitTagAliasAndRemotePath("project_id", "release")
	s.NoError(err)
	s.True(hasAliases)
	s.Empty(path)

	hasAliases, path, err = HasMatchingGitTagAliasAndRemotePath("project_id2", "release")
	s.Error(err)
	s.False(hasAliases)
	s.Empty(path)

	newAlias3 := ProjectAlias{
		ProjectID:  "project_id2",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "file.yml",
	}
	s.NoError(newAlias3.Upsert())
	hasAliases, path, err = HasMatchingGitTagAliasAndRemotePath("project_id2", "release")
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
		s.NoError(aliasCopy.Upsert())
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{projectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQ(ProjectAliasCollection, q, &out))
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
		s.NoError(a.Upsert())
		s.aliases[i] = a
	}
	var out []ProjectAlias
	q := db.Query(bson.M{})
	s.NoError(db.FindAllQ(ProjectAliasCollection, q, &out))
	s.Len(out, 10)

	for i, a := range s.aliases {
		s.NoError(RemoveProjectAlias(a.ID.Hex()))
		s.NoError(db.FindAllQ(ProjectAliasCollection, q, &out))
		s.Len(out, 10-i-1)
	}
}

func (s *ProjectAliasSuite) TestFindAliasesForProject() {
	for _, a := range s.aliases {
		s.NoError(a.Upsert())
	}
	a1 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-1",
		Variant:   "variants-111",
		Task:      "variants-11",
	}
	s.NoError(a1.Upsert())

	out, err := FindAliasesForProjectFromDb("project-1")
	s.NoError(err)
	s.Len(out, 2)
}

func (s *ProjectAliasSuite) TestFindAliasInProject() {
	for _, a := range s.aliases {
		s.NoError(a.Upsert())
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
	s.NoError(a1.Upsert())
	s.NoError(a2.Upsert())
	s.NoError(a3.Upsert())

	found, shouldExit, err := findMatchingAliasForProjectRef("project-1", "alias-1")
	s.NoError(err)
	s.True(shouldExit)
	s.Len(found, 2)
}

func (s *ProjectAliasSuite) TestMergeAliasesWithProjectConfig() {
	s.Require().NoError(db.ClearCollections(ProjectAliasCollection, ProjectConfigCollection, ProjectRefCollection))
	pRef := ProjectRef{
		Id:        "project-1",
		RepoRefId: "r1",
	}
	s.NoError(pRef.Upsert())
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
	s.NoError(a1.Upsert())
	s.NoError(a2.Upsert())
	s.NoError(a3.Upsert())
	s.NoError(patchAlias.Upsert())

	projectConfig := &ProjectConfig{
		Id:      "project-1",
		Project: "project-1",
		ProjectConfigFields: ProjectConfigFields{
			PatchAliases: []ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     "alias-2",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     "alias-1",
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
	s.NoError(projectConfig.Insert())

	projectAliases, err := FindAliasInProjectRepoOrConfig("project-1", evergreen.CommitQueueAlias)
	s.NoError(err)
	s.Len(projectAliases, 2)

	projectAliases, err = FindAliasInProjectRepoOrConfig("project-1", evergreen.GithubPRAlias)
	s.NoError(err)
	s.Len(projectAliases, 1)

	projectAliases, err = FindAliasInProjectRepoOrConfig("project-1", evergreen.GithubChecksAlias)
	s.NoError(err)
	s.Len(projectAliases, 1)

	projectAliases, err = FindAliasInProjectRepoOrConfig("project-1", "alias-0")
	s.NoError(err)
	s.Len(projectAliases, 1)

	projectAliases, err = FindAliasInProjectRepoOrConfig("project-1", "alias-2")
	s.NoError(err)
	s.Len(projectAliases, 0)

	projectAliases, err = FindAliasInProjectRepoOrConfig("project-1", "alias-2")
	s.NoError(err)
	s.Len(projectAliases, 0)

	projectAliases, err = FindAliasInProjectRepoOrConfig("project-1", "nonexistent")
	s.NoError(err)
	s.Len(projectAliases, 0)
}

func (s *ProjectAliasSuite) TestFindAliasInProjectOrPatchedConfig() {
	projYml := `
patch_aliases:
  - alias: "test"
    variant: "^ubuntu1604$"
    task: "^test.*$"
    remote_path: ""
	description: "Test Description"
`
	projectAliases, err := FindAliasInProjectRepoOrPatchedConfig("", "test", projYml)
	s.NoError(err)
	s.Len(projectAliases, 1)
	s.Equal("^ubuntu1604$", projectAliases[0].Variant)
	s.Equal("Test Description", projectAliases[0].Description)
}

func (s *ProjectAliasSuite) TestFindAliasInProjectOrRepo() {
	s.Require().NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

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
	s.NoError(repoRef.Upsert())
	s.NoError(pRef1.Upsert())
	s.NoError(pRef2.Upsert())

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
		s.NoError(alias.Upsert())
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
		s.NoError(alias.Upsert())
	}

	// Test project with aliases
	found, err := FindAliasInProjectRepoOrConfig(pRef1.Id, "alias-3")
	s.NoError(err)
	s.Len(found, 3)

	// Test project without aliases; parent repo has aliases
	found, err = FindAliasInProjectRepoOrConfig(pRef2.Id, "alias-1")
	s.NoError(err)
	s.Len(found, 2)

	// Test project doesn't match alias but other patch aliases are defined so we don't continue to repo
	found, err = FindAliasInProjectRepoOrConfig(pRef1.Id, "alias-1")
	s.NoError(err)
	s.Len(found, 0)

	// Test non-existent project
	found, err = FindAliasInProjectRepoOrConfig("bad-project", "alias-1")
	s.Error(err)
	s.Len(found, 0)

	// Test no aliases found
	found, err = FindAliasInProjectRepoOrConfig(pRef1.Id, "alias-5")
	s.NoError(err)
	s.Len(found, 0)
}

func (s *ProjectAliasSuite) TestUpsertAliasesForProject() {
	for _, a := range s.aliases {
		a.ProjectID = "old-project"
		s.NoError(a.Upsert())
	}
	s.NoError(UpsertAliasesForProject(s.aliases, "new-project"))

	found, err := FindAliasesForProjectFromDb("new-project")
	s.NoError(err)
	s.Len(found, 10)

	// verify old aliases not overwritten
	found, err = FindAliasesForProjectFromDb("old-project")
	s.NoError(err)
	s.Len(found, 10)
}

func TestMatching(t *testing.T) {
	assert := assert.New(t)
	aliases := ProjectAliases{
		{Alias: "one", Variant: "bv1", Task: "t1", GitTag: "tag-."},
		{Alias: "two", Variant: "bv2", Task: "t2"},
		{Alias: "three", Variant: "bv3", TaskTags: []string{"tag3"}},
		{Alias: "four", VariantTags: []string{"variantTag"}, TaskTags: []string{"tag4"}},
		{Alias: "five", Variant: "bv4", TaskTags: []string{"!tag3", "tag5"}},
	}
	bv1Matches, err := aliases.AliasesMatchingVariant("bv1", nil)
	assert.NoError(err)
	assert.NotEmpty(bv1Matches)
	bv2Matches, err := aliases.AliasesMatchingVariant("bv2", nil)
	assert.NoError(err)
	assert.NotEmpty(bv2Matches)
	bv3Matches, err := aliases.AliasesMatchingVariant("bv3", nil)
	assert.NoError(err)
	assert.NotEmpty(bv3Matches)
	bv4Matches, err := aliases.AliasesMatchingVariant("bv4", nil)
	assert.NoError(err)
	assert.NotEmpty(bv4Matches)
	bv5Matches, err := aliases.AliasesMatchingVariant("bv5", nil)
	assert.NoError(err)
	assert.Empty(bv5Matches)
	tagsMatches, err := aliases.AliasesMatchingVariant("", []string{"variantTag", "notATag"})
	assert.NoError(err)
	assert.NotEmpty(tagsMatches)
	matches, err := aliases.AliasesMatchingVariant("", []string{"notATag"})
	assert.NoError(err)
	assert.Empty(matches)

	matches, err = aliases.AliasesMatchingVariant("variantTag", nil)
	assert.NoError(err)
	assert.Empty(matches)

	task := &ProjectTask{
		Name: "t1",
	}
	match, err := bv1Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.True(match)
	task = &ProjectTask{
		Name: "t2",
	}
	match, err = bv1Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.False(match)
	task = &ProjectTask{
		Name: "t2",
	}
	match, err = bv2Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.True(match)
	task = &ProjectTask{
		Tags: []string{"tag3"},
		Name: "t3",
	}
	match, err = bv3Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.True(match)
	task = &ProjectTask{}
	match, err = bv3Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.False(match)

	task = &ProjectTask{
		Tags: []string{"tag4"},
	}
	match, err = bv5Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.False(match)

	match, err = tagsMatches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.True(match)

	match, err = bv4Matches.HasMatchingTask(task.Name, task.Tags)
	assert.NoError(err)
	assert.True(match)

	match, err = aliases.HasMatchingGitTag("tag-1")
	assert.NoError(err)
	assert.True(match)

	match, err = aliases.HasMatchingGitTag("tag1")
	assert.NoError(err)
	assert.False(match)
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

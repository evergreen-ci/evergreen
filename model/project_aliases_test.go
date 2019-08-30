package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
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
			ProjectID: fmt.Sprintf("project-%d", i),
			Alias:     fmt.Sprintf("alias-%d", i),
			Variant:   fmt.Sprintf("variant-%d", i),
			Task:      fmt.Sprintf("task-%d", i),
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
	}
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

	out, err := FindAliasesForProject("project-1")
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

	found, err := FindAliasInProject("project-1", "alias-1")
	s.NoError(err)
	s.Len(found, 2)
}

func (s *ProjectAliasSuite) TestUpsertAliasesForProject() {
	for _, a := range s.aliases {
		a.ProjectID = "old-project"
		s.NoError(a.Upsert())
	}
	s.NoError(UpsertAliasesForProject(s.aliases, "new-project"))

	found, err := FindAliasesForProject("new-project")
	s.NoError(err)
	s.Len(found, 10)

	// verify old aliases not overwritten
	found, err = FindAliasesForProject("old-project")
	s.NoError(err)
	s.Len(found, 10)
}

func TestMatching(t *testing.T) {
	assert := assert.New(t)
	aliases := ProjectAliases{
		{Alias: "one", Variant: "bv1", Task: "t1"},
		{Alias: "two", Variant: "bv2", Task: "t2"},
		{Alias: "three", Variant: "bv3", TaskTags: []string{"tag3"}},
		{Alias: "four", VariantTags: []string{"variantTag"}, TaskTags: []string{"tag4"}},
	}
	match, err := aliases.HasMatchingVariant("bv1", nil)
	assert.NoError(err)
	assert.True(match)
	match, err = aliases.HasMatchingVariant("bv5", nil)
	assert.NoError(err)
	assert.False(match)
	match, err = aliases.HasMatchingVariant("", []string{"variantTag"})
	assert.NoError(err)
	assert.True(match)

	match, err = aliases.HasMatchingVariant("variantTag", nil)
	assert.NoError(err)
	assert.False(match)
	match, err = aliases.HasMatchingVariant("", []string{"notATag"})
	assert.NoError(err)
	assert.False(match)

	task := &ProjectTask{
		Name: "t1",
	}
	match, err = aliases.HasMatchingTask("bv1", nil, task)
	assert.NoError(err)
	assert.True(match)
	task = &ProjectTask{
		Name: "t2",
	}
	match, err = aliases.HasMatchingTask("bv1", nil, task)
	assert.NoError(err)
	assert.False(match)
	task = &ProjectTask{
		Name: "t2",
	}
	match, err = aliases.HasMatchingTask("bv2", nil, task)
	assert.NoError(err)
	assert.True(match)
	task = &ProjectTask{
		Tags: []string{"tag3"},
		Name: "t3",
	}
	match, err = aliases.HasMatchingTask("bv3", nil, task)
	assert.NoError(err)
	assert.True(match)
	task = &ProjectTask{}
	match, err = aliases.HasMatchingTask("bv3", nil, task)
	assert.NoError(err)
	assert.False(match)

	task = &ProjectTask{
		Tags: []string{"tag4"},
	}
	match, err = aliases.HasMatchingTask("bv4", nil, task)
	assert.NoError(err)
	assert.False(match)

	match, err = aliases.HasMatchingTask("", []string{"variantTag", "notATag"}, task)
	assert.NoError(err)
	assert.True(match)
}

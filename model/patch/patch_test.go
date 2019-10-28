package patch

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestConfigChanged(t *testing.T) {
	assert := assert.New(t)
	remoteConfigPath := "config/evergreen.yml"
	p := &Patch{
		Patches: []ModulePatch{{
			PatchSet: PatchSet{
				Summary: []Summary{{
					Name:      remoteConfigPath,
					Additions: 3,
					Deletions: 3,
				}},
			},
		}},
	}

	assert.True(p.ConfigChanged(remoteConfigPath))

	p.Patches[0].PatchSet.Summary[0].Name = "dakar"
	assert.False(p.ConfigChanged(remoteConfigPath))
}

type patchSuite struct {
	suite.Suite
	testConfig *evergreen.Settings

	patches []*Patch
	time    time.Time
}

func TestPatchSuite(t *testing.T) {
	suite.Run(t, new(patchSuite))
}

func (s *patchSuite) SetupTest() {
	s.testConfig = testutil.TestConfig()

	s.NoError(db.ClearCollections(Collection))
	s.time = time.Now().Add(-12 * time.Hour)
	s.patches = []*Patch{
		{
			Author:     "octocat",
			CreateTime: s.time,
			GithubPatchData: GithubPatch{
				PRNumber:  9001,
				Author:    "octocat",
				BaseOwner: "evergreen-ci",
				BaseRepo:  "evergreen",
				HeadOwner: "octocat",
				HeadRepo:  "evergreen",
			},
		},
		{
			CreateTime: s.time.Add(-time.Hour),
			GithubPatchData: GithubPatch{
				PRNumber:  9001,
				Author:    "octocat",
				BaseOwner: "evergreen-ci",
				BaseRepo:  "evergreen",
				HeadOwner: "octocat",
				HeadRepo:  "evergreen",
			},
		},
		{
			CreateTime: s.time.Add(time.Hour),
			GithubPatchData: GithubPatch{
				PRNumber:  9001,
				Author:    "octocat",
				BaseOwner: "evergreen-ci",
				BaseRepo:  "evergreen",
				HeadOwner: "octocat",
				HeadRepo:  "evergreen",
			},
		},
		{
			CreateTime: s.time.Add(time.Hour),
			GithubPatchData: GithubPatch{
				PRNumber:  9002,
				Author:    "octodog",
				BaseOwner: "evergreen-ci",
				BaseRepo:  "evergreen",
				HeadOwner: "octocat",
				HeadRepo:  "evergreen",
			},
		},
		{
			CreateTime: s.time,
			GithubPatchData: GithubPatch{
				PRNumber:       9002,
				Author:         "octodog",
				MergeCommitSHA: "abcdef",
			},
		},
	}

	for _, patch := range s.patches {
		s.NoError(patch.Insert())
	}

	s.True(s.patches[0].IsGithubPRPatch())
	s.False(s.patches[0].IsPRMergePatch())
	s.True(s.patches[1].IsGithubPRPatch())
	s.False(s.patches[1].IsPRMergePatch())
	s.True(s.patches[2].IsGithubPRPatch())
	s.False(s.patches[2].IsPRMergePatch())
	s.True(s.patches[3].IsGithubPRPatch())
	s.False(s.patches[3].IsPRMergePatch())
	s.True(s.patches[4].IsPRMergePatch())
	s.False(s.patches[4].IsGithubPRPatch())
}

func (s *patchSuite) TestByGithubPRAndCreatedBefore() {
	patches, err := Find(ByGithubPRAndCreatedBefore(time.Now(), "evergreen-ci", "evergreen", 1))
	s.NoError(err)
	s.Empty(patches)

	patches, err = Find(ByGithubPRAndCreatedBefore(time.Now(), "octodog", "evergreen", 9002))
	s.NoError(err)
	s.Empty(patches)

	patches, err = Find(ByGithubPRAndCreatedBefore(time.Now(), "", "", 0))
	s.NoError(err)
	s.Empty(patches)

	patches, err = Find(ByGithubPRAndCreatedBefore(s.patches[2].CreateTime, "evergreen-ci", "evergreen", 9001))
	s.NoError(err)
	s.Len(patches, 2)

	patches, err = Find(ByGithubPRAndCreatedBefore(s.time, "evergreen-ci", "evergreen", 9001))
	s.NoError(err)
	s.Len(patches, 1)
}

func (s *patchSuite) TestMakeMergePatch() {
	pr := &github.PullRequest{
		Base: &github.PullRequestBranch{
			SHA: github.String("abcdef"),
		},
		User: &github.User{
			ID: github.Int64(1),
		},
		Number:         github.Int(1),
		MergeCommitSHA: github.String("abcdef"),
	}

	p, err := MakeMergePatch(pr, "mci", evergreen.CommitQueueAlias)
	s.NoError(err)
	s.Equal("mci", p.Project)
	s.Equal(evergreen.PatchCreated, p.Status)
	s.Equal(*pr.MergeCommitSHA, p.GithubPatchData.MergeCommitSHA)
}

func (s *patchSuite) TestUpdateGithashProjectAndTasks() {
	patch, err := FindOne(ByUser("octocat"))
	s.NoError(err)
	s.Empty(patch.Githash)
	s.Empty(patch.VariantsTasks)
	s.Empty(patch.Tasks)
	s.Empty(patch.BuildVariants)
	s.Empty(patch.PatchedConfig)

	patch.Githash = "abcdef"
	patch.Patches = []ModulePatch{{Githash: "abcdef"}}
	patch.Tasks = append(patch.Tasks, "task1")
	patch.BuildVariants = append(patch.BuildVariants, "bv1")
	patch.PatchedConfig = "config"
	patch.VariantsTasks = []VariantTasks{
		{
			Variant: "variant1",
		},
	}

	s.NoError(patch.UpdateGithashProjectAndTasks())

	dbPatch, err := FindOne(ById(patch.Id))
	s.NoError(err)

	s.Equal("abcdef", dbPatch.Githash)
	s.Equal("config", dbPatch.PatchedConfig)

	s.Require().Len(dbPatch.Patches, 1)
	s.Equal("abcdef", dbPatch.Patches[0].Githash)

	s.Require().NotEmpty(patch.Tasks)
	s.Equal("task1", dbPatch.Tasks[0])

	s.Require().NotEmpty(patch.BuildVariants)
	s.Equal("bv1", dbPatch.BuildVariants[0])

	s.Require().NotEmpty(patch.VariantsTasks)
	s.Equal("variant1", dbPatch.VariantsTasks[0].Variant)
}

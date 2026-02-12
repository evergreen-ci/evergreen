package patch

import (
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestConfigChanged(t *testing.T) {
	assert := assert.New(t)
	remoteConfigPath := "config/evergreen.yml"
	p := &Patch{
		Patches: []ModulePatch{{
			PatchSet: PatchSet{
				Summary: []thirdparty.Summary{{
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

func TestShouldPatchFileWithDiff(t *testing.T) {
	remoteConfigPath := "config/evergreen.yml"

	for name, tc := range map[string]struct {
		patch                   *Patch
		shouldPatchFileWithDiff bool
	}{
		"RegularPatchWithConfigChanged": {
			patch: &Patch{
				Patches: []ModulePatch{{
					PatchSet: PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      remoteConfigPath,
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			},
			shouldPatchFileWithDiff: true,
		},
		"RegularPatchWithConfigNotChanged": {
			patch: &Patch{
				Patches: []ModulePatch{{
					PatchSet: PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      "other/file.yml",
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			},
			shouldPatchFileWithDiff: false,
		},
		"GitHubPRPatchWithConfigChanged": {
			patch: &Patch{
				GithubPatchData: thirdparty.GithubPatch{
					HeadOwner: "owner",
				},
				Patches: []ModulePatch{{
					PatchSet: PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      remoteConfigPath,
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			},
			shouldPatchFileWithDiff: false,
		},
		"MergeQueuePatchWithConfigChangedViaAlias": {
			patch: &Patch{
				Alias: evergreen.CommitQueueAlias,
				Patches: []ModulePatch{{
					PatchSet: PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      remoteConfigPath,
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			},
			shouldPatchFileWithDiff: false,
		},
		"MergeQueuePatchWithConfigChangedViaHeadSHA": {
			patch: &Patch{
				GithubMergeData: thirdparty.GithubMergeGroup{
					HeadSHA: "abc123",
				},
				Patches: []ModulePatch{{
					PatchSet: PatchSet{
						Summary: []thirdparty.Summary{{
							Name:      remoteConfigPath,
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			},
			shouldPatchFileWithDiff: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.shouldPatchFileWithDiff, tc.patch.ShouldPatchFileWithDiff(remoteConfigPath))
		})
	}
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
			GithubPatchData: thirdparty.GithubPatch{
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
			GithubPatchData: thirdparty.GithubPatch{
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
			GithubPatchData: thirdparty.GithubPatch{
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
			GithubPatchData: thirdparty.GithubPatch{
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
			GithubPatchData: thirdparty.GithubPatch{
				PRNumber: 9002,
				Author:   "octodog",
			},
		},
	}

	for _, patch := range s.patches {
		s.NoError(patch.Insert(s.T().Context()))
	}

	s.True(s.patches[0].IsGithubPRPatch())
	s.True(s.patches[1].IsGithubPRPatch())
	s.True(s.patches[2].IsGithubPRPatch())
	s.True(s.patches[3].IsGithubPRPatch())
	s.False(s.patches[4].IsGithubPRPatch())
}

func (s *patchSuite) TestByGithubPRAndCreatedBefore() {
	patches, err := Find(s.T().Context(), ByGithubPRAndCreatedBefore(time.Now(), "evergreen-ci", "evergreen", 1))
	s.NoError(err)
	s.Empty(patches)

	patches, err = Find(s.T().Context(), ByGithubPRAndCreatedBefore(time.Now(), "octodog", "evergreen", 9002))
	s.NoError(err)
	s.Empty(patches)

	patches, err = Find(s.T().Context(), ByGithubPRAndCreatedBefore(time.Now(), "", "", 0))
	s.NoError(err)
	s.Empty(patches)

	patches, err = Find(s.T().Context(), ByGithubPRAndCreatedBefore(s.patches[2].CreateTime, "evergreen-ci", "evergreen", 9001))
	s.NoError(err)
	s.Len(patches, 2)

	patches, err = Find(s.T().Context(), ByGithubPRAndCreatedBefore(s.time, "evergreen-ci", "evergreen", 9001))
	s.NoError(err)
	s.Len(patches, 1)
}

func (s *patchSuite) TestUpdateGithashProjectAndTasks() {
	patch, err := FindOne(s.T().Context(), ByUserAndCommitQueue("octocat", false))
	s.NoError(err)
	s.Empty(patch.Githash)
	s.Empty(patch.VariantsTasks)
	s.Empty(patch.Tasks)
	s.Empty(patch.BuildVariants)

	patch.Githash = "abcdef"
	patch.Patches = []ModulePatch{{Githash: "abcdef"}}
	patch.Tasks = append(patch.Tasks, "task1")
	patch.BuildVariants = append(patch.BuildVariants, "bv1")
	patch.VariantsTasks = []VariantTasks{
		{
			Variant: "variant1",
		},
	}

	s.NoError(patch.UpdateGithashProjectAndTasks(s.T().Context()))

	dbPatch, err := FindOne(s.T().Context(), ById(patch.Id))
	s.NoError(err)

	s.Equal("abcdef", dbPatch.Githash)

	s.Require().Len(dbPatch.Patches, 1)
	s.Equal("abcdef", dbPatch.Patches[0].Githash)

	s.Require().NotEmpty(patch.Tasks)
	s.Equal("task1", dbPatch.Tasks[0])

	s.Require().NotEmpty(patch.BuildVariants)
	s.Equal("bv1", dbPatch.BuildVariants[0])

	s.Require().NotEmpty(patch.VariantsTasks)
	s.Equal("variant1", dbPatch.VariantsTasks[0].Variant)
}

func TestPatchSortByCreateTime(t *testing.T) {
	assert := assert.New(t)
	patches := PatchesByCreateTime{
		{PatchNumber: 5, CreateTime: time.Now().Add(time.Hour * 3)},
		{PatchNumber: 3, CreateTime: time.Now().Add(time.Hour)},
		{PatchNumber: 1, CreateTime: time.Now()},
		{PatchNumber: 4, CreateTime: time.Now().Add(time.Hour * 2)},
		{PatchNumber: 100, CreateTime: time.Now().Add(time.Hour * 4)},
	}
	sort.Sort(patches)
	assert.Equal(1, patches[0].PatchNumber)
	assert.Equal(3, patches[1].PatchNumber)
	assert.Equal(4, patches[2].PatchNumber)
	assert.Equal(5, patches[3].PatchNumber)
	assert.Equal(100, patches[4].PatchNumber)
}

func TestMergeVariantsTasks(t *testing.T) {
	for testName, testCase := range map[string]struct {
		vt1      []VariantTasks
		vt2      []VariantTasks
		expected []VariantTasks
	}{
		"AddsAllVariantTasks": {
			vt1: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			vt2: []VariantTasks{
				{
					Variant: "bv2",
					Tasks:   []string{"t2"},
				},
			},
			expected: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2"},
				},
			},
		},
		"MergesDuplicateVariantTasks": {
			vt1: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			vt2: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t2"},
				},
			},
			expected: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2"},
				},
			},
		},
		"MergesDuplicateVariantDisplayTasks": {
			vt1: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t1"},
						},
					},
				},
			},
			vt2: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t2"},
						},
					},
				},
			},
			expected: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t1", "t2"},
						},
					},
				},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			actual := MergeVariantsTasks(testCase.vt1, testCase.vt2)
			checkEqualVTs(t, testCase.expected, actual)
		})
	}
}

// checkEqualVT checks that the two VariantTasks are identical.
func checkEqualVT(t *testing.T, expected VariantTasks, actual VariantTasks) {
	missingExpected, missingActual := utility.StringSliceSymmetricDifference(expected.Tasks, actual.Tasks)
	assert.Empty(t, missingExpected, "unexpected tasks '%s' for build variant'%s'", missingExpected, expected.Variant)
	assert.Empty(t, missingActual, "missing expected tasks '%s' for build variant '%s'", missingActual, actual.Variant)

	expectedDTs := map[string]DisplayTask{}
	for _, dt := range expected.DisplayTasks {
		expectedDTs[dt.Name] = dt
	}
	actualDTs := map[string]DisplayTask{}
	for _, dt := range actual.DisplayTasks {
		actualDTs[dt.Name] = dt
	}
	assert.Len(t, actualDTs, len(expectedDTs))
	for _, expectedDT := range expectedDTs {
		actualDT, ok := actualDTs[expectedDT.Name]
		if !assert.True(t, ok, "display task '%s'") {
			continue
		}
		missingExpected, missingActual = utility.StringSliceSymmetricDifference(expectedDT.ExecTasks, actualDT.ExecTasks)
		assert.Empty(t, missingExpected, "unexpected exec tasks '%s' for display task '%s' in build variant '%s'", missingExpected, expectedDT.Name, expected.Variant)
		assert.Empty(t, missingActual, "missing exec tasks '%s' for display task '%s' in build variant '%s'", missingActual, actualDT.Name, actual.Variant)
	}
}

// checkEqualVTs checks that the two slices of VariantTasks are identical sets.
func checkEqualVTs(t *testing.T, expected []VariantTasks, actual []VariantTasks) {
	assert.Len(t, actual, len(expected))
	for _, expectedVT := range expected {
		var found bool
		for _, actualVT := range actual {
			if actualVT.Variant != expectedVT.Variant {
				continue
			}
			found = true
			checkEqualVT(t, expectedVT, actualVT)
			break
		}
		assert.True(t, found, "build variant '%s' not found", expectedVT.Variant)
	}
}

func TestSetParametersFromParent(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	parentPatchID := bson.NewObjectId()
	parentPatch := Patch{
		Id: parentPatchID,
		Triggers: TriggerInfo{
			DownstreamParameters: []Parameter{
				{
					Key:   "hello",
					Value: "notHello",
				},
			},
		},
	}
	assert.NoError(parentPatch.Insert(t.Context()))
	p := Patch{
		Id: bson.NewObjectId(),
		Triggers: TriggerInfo{
			ParentPatch: parentPatchID.Hex(),
		},
	}
	assert.NoError(p.Insert(t.Context()))
	_, err := p.SetParametersFromParent(t.Context())
	assert.NoError(err)
	assert.Equal(parentPatch.Triggers.DownstreamParameters[0].Key, p.Parameters[0].Key)
}

func TestSetDownstreamParameters(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	p := Patch{
		Id: bson.NewObjectId(),
		Triggers: TriggerInfo{
			DownstreamParameters: []Parameter{
				{
					Key:   "key_0",
					Value: "value_0",
				},
			},
		},
	}
	assert.NoError(p.Insert(t.Context()))

	paramsToAdd := []Parameter{
		{
			Key:   "key_1",
			Value: "value_1",
		},
		{
			Key:   "key_2",
			Value: "value_2",
		},
	}

	assert.NoError(p.SetDownstreamParameters(t.Context(), paramsToAdd))
	assert.Equal("key_0", p.Triggers.DownstreamParameters[0].Key)
	assert.Equal("key_1", p.Triggers.DownstreamParameters[1].Key)
	assert.Equal("key_2", p.Triggers.DownstreamParameters[2].Key)
}

func TestSetTriggerAliases(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	p := Patch{
		Id: bson.NewObjectId(),
		Triggers: TriggerInfo{
			Aliases: []string{"alias_0"},
		},
	}
	assert.NoError(p.Insert(t.Context()))

	p.Triggers.Aliases = []string{
		"alias_1",
		"alias_2",
	}

	assert.NoError(p.SetTriggerAliases(t.Context()))

	dbPatch, err := FindOne(t.Context(), ById(p.Id))
	assert.NoError(err)
	assert.Equal("alias_0", dbPatch.Triggers.Aliases[0])
	assert.Equal("alias_1", dbPatch.Triggers.Aliases[1])
	assert.Equal("alias_2", dbPatch.Triggers.Aliases[2])
}

func TestSetChildPatches(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	p := Patch{
		Id: bson.NewObjectId(),
		Triggers: TriggerInfo{
			ChildPatches: []string{"id_0"},
		},
	}
	assert.NoError(p.Insert(t.Context()))

	p.Triggers.ChildPatches = []string{
		"id_1",
		"id_2",
	}

	assert.NoError(p.SetChildPatches(t.Context()))

	dbPatch, err := FindOne(t.Context(), ById(p.Id))
	assert.NoError(err)
	assert.Equal("id_0", dbPatch.Triggers.ChildPatches[0])
	assert.Equal("id_1", dbPatch.Triggers.ChildPatches[1])
	assert.Equal("id_2", dbPatch.Triggers.ChildPatches[2])
}

func TestGetCollectiveStatusFromPatchStatuses(t *testing.T) {
	successful := []string{evergreen.VersionSucceeded}
	assert.Equal(t, evergreen.VersionSucceeded, GetCollectiveStatusFromPatchStatuses(successful))

	assert.Equal(t, evergreen.VersionSucceeded, GetCollectiveStatusFromPatchStatuses(successful))
	failed := []string{evergreen.VersionSucceeded, evergreen.VersionFailed}
	assert.Equal(t, evergreen.VersionFailed, GetCollectiveStatusFromPatchStatuses(failed))

	started := []string{evergreen.VersionStarted, evergreen.VersionSucceeded, evergreen.VersionFailed}
	assert.Equal(t, evergreen.VersionStarted, GetCollectiveStatusFromPatchStatuses(started))

	started = []string{evergreen.VersionCreated, evergreen.VersionSucceeded, evergreen.VersionFailed}
	assert.Equal(t, evergreen.VersionStarted, GetCollectiveStatusFromPatchStatuses(started))

	aborted := []string{evergreen.VersionSucceeded, evergreen.VersionAborted}
	assert.Equal(t, evergreen.VersionAborted, GetCollectiveStatusFromPatchStatuses(aborted))

	created := []string{evergreen.VersionCreated}
	assert.Equal(t, evergreen.VersionCreated, GetCollectiveStatusFromPatchStatuses(created))
}

func TestGetRequester(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	p1 := Patch{
		Id:    bson.NewObjectId(),
		Alias: "",
		GithubPatchData: thirdparty.GithubPatch{
			HeadOwner: "me",
		},
	}
	require.NoError(t, p1.Insert(t.Context()))

	p2 := Patch{
		Id:    bson.NewObjectId(),
		Alias: evergreen.CommitQueueAlias,
		GithubMergeData: thirdparty.GithubMergeGroup{
			HeadSHA: "1234567",
		},
	}
	require.NoError(t, p2.Insert(t.Context()))

	p3 := Patch{
		Id:    bson.NewObjectId(),
		Alias: "",
	}
	require.NoError(t, p3.Insert(t.Context()))

	require.Equal(t, evergreen.GithubPRRequester, p1.GetRequester())
	require.Equal(t, evergreen.GithubMergeRequester, p2.GetRequester())
	require.Equal(t, evergreen.PatchVersionRequester, p3.GetRequester())
}

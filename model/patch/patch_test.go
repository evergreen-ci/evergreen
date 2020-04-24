package patch

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
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
	patch, err := FindOne(ByUserAndCommitQueue("octocat", false))
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

func TestIsMailbox(t *testing.T) {
	isMBP, err := IsMailbox(filepath.Join(testutil.GetDirectoryOfFile(), "..", "testdata", "filethatdoesntexist.txt"))
	assert.Error(t, err)
	assert.False(t, isMBP)

	isMBP, err = IsMailbox(filepath.Join(testutil.GetDirectoryOfFile(), "..", "testdata", "test.patch"))
	assert.NoError(t, err)
	assert.True(t, isMBP)

	isMBP, err = IsMailbox(filepath.Join(testutil.GetDirectoryOfFile(), "..", "testdata", "patch.diff"))
	assert.NoError(t, err)
	assert.False(t, isMBP)

	isMBP, err = IsMailbox(filepath.Join(testutil.GetDirectoryOfFile(), "..", "testdata", "emptyfile.txt"))
	assert.NoError(t, err)
	assert.False(t, isMBP)
}

func TestUpdateModulePatch(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	p := &Patch{Description: "original message"}
	require.NoError(t, p.Insert())
	p, err := FindOne(db.Q{})
	require.NoError(t, err)

	// with a new module patch
	assert.NoError(t, p.UpdateModulePatch(ModulePatch{
		ModuleName: "mod1",
		Message:    "new message",
	}))

	assert.Len(t, p.Patches, 1)
	assert.Equal(t, "new message", p.Description)

	pDb, err := FindOne(db.Q{})
	assert.NoError(t, err)
	assert.Equal(t, "new message", pDb.Description)
	assert.Len(t, pDb.Patches, 1)

	// with an existing module patch
	assert.NoError(t, p.UpdateModulePatch(ModulePatch{
		ModuleName: "mod1",
		Message:    "newer message",
	}))

	assert.Len(t, p.Patches, 1)
	assert.Equal(t, "newer message", p.Description)

	pDb, err = FindOne(db.Q{})
	assert.NoError(t, err)
	assert.Equal(t, "newer message", pDb.Description)
	assert.Len(t, pDb.Patches, 1)
}

func TestResolveSyncVariantsTasks(t *testing.T) {
	for testName, testCase := range map[string]struct {
		bvs      []string
		tasks    []string
		vts      []VariantTasks
		expected []VariantTasks
	}{
		"EmptyForEmptyInputs": {},
		"ReturnsEmptyForNoVTs": {
			bvs:   []string{"bv1", "tvB"},
			tasks: []string{"t1", "t2"},
		},
		"ReturnsMatchingBVsAndTasks": {
			bvs:   []string{"bv1", "bv2", "bv3"},
			tasks: []string{"t1", "t2"},
			vts: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1", "t2", "t3"},
				}, {
					Variant: "bv4",
					Tasks:   []string{"t1"},
				},
			},
			expected: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1", "t2"},
				},
			},
		},
		"ReturnsMatchingDisplayTasks": {
			bvs:   []string{"bv1", "bv2"},
			tasks: []string{"dt1", "dt2", "t1"},
			vts: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t1"},
						},
						{Name: "dt3"},
					},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt2",
							ExecTasks: []string{"t2", "t3"},
						},
					},
				},
			},
			expected: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t1"},
						},
					},
				}, {
					Variant: "bv2",
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt2",
							ExecTasks: []string{"t2", "t3"},
						},
					},
				},
			},
		},
		"AllBVsMatched": {
			bvs:   []string{"all"},
			tasks: []string{"t1", "t2", "dt1"},
			vts: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t3"},
						},
					},
				}, {
					Variant: "bv3",
					Tasks:   []string{"t3", "t4"},
				},
			},
			expected: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t3"},
						},
					},
				},
			},
		},
		"AllTasksMatch": {
			bvs:   []string{"bv1"},
			tasks: []string{"all"},
			vts: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t2", "t3"},
						},
					},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1", "t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t2", "t3"},
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
							ExecTasks: []string{"t2", "t3"},
						},
					},
				},
			},
		},
		"AllBVsAndTasksMatch": {
			bvs:   []string{"all"},
			tasks: []string{"all"},
			vts: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2", "t3"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"t1", "t2"},
						},
					},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1", "t3", "t4"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt2",
							ExecTasks: []string{"t1", "t3"},
						},
					},
				}, {
					Variant: "bv3",
					Tasks:   []string{"t1", "t2"},
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
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1", "t3", "t4"},
					DisplayTasks: []DisplayTask{
						{
							Name:      "dt2",
							ExecTasks: []string{"t1", "t3"},
						},
					},
				}, {
					Variant: "bv3",
					Tasks:   []string{"t1", "t2"},
				},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			p := &Patch{
				SyncBuildVariants: testCase.bvs,
				SyncTasks:         testCase.tasks,
			}
			actual := p.ResolveSyncVariantTasks(testCase.vts)
			assert.Len(t, actual, len(testCase.expected))
			checkEqualVTs(t, testCase.expected, actual)
		})
	}
}

func TestAddSyncVariantsTasks(t *testing.T) {
	for testName, testCase := range map[string]struct {
		syncBVs         []string
		syncTasks       []string
		existingSyncVTs []VariantTasks
		newVTs          []VariantTasks
		expectedSyncVTs []VariantTasks
	}{
		"AddsNewBVsWithNewTasks": {
			syncBVs:   []string{"bv1", "bv2"},
			syncTasks: []string{"t1"},
			existingSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			newVTs: []VariantTasks{
				{
					Variant: "bv2",
					Tasks:   []string{"t1"},
				},
			},
			expectedSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1"},
				},
			},
		},
		"AddsNewTasksToExistingBVs": {
			syncBVs:   []string{"bv1", "bv2"},
			syncTasks: []string{"t1", "t2"},
			existingSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			newVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t2", "t3"},
				},
			},
			expectedSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2"},
				},
			},
		},
		"AddsNewBVsWithNewTasksAndAddsNewTasksToExistingBVs": {
			syncBVs:   []string{"bv1", "bv2"},
			syncTasks: []string{"t1", "t2"},
			existingSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			newVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t2"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2", "t3"},
				},
			},
			expectedSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2"},
				},
			},
		},
		"IgnoresDuplicates": {
			syncBVs:   []string{"bv1", "bv2"},
			syncTasks: []string{"t1", "t2"},
			existingSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			newVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2", "t3"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1", "t3"},
				},
			},
			expectedSyncVTs: []VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t1"},
				},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			p := Patch{
				Id:                mgobson.NewObjectId(),
				SyncBuildVariants: testCase.syncBVs,
				SyncTasks:         testCase.syncTasks,
				SyncVariantsTasks: testCase.existingSyncVTs,
			}
			require.NoError(t, p.Insert())

			require.NoError(t, p.AddSyncVariantsTasks(testCase.newVTs))
			dbPatch, err := FindOne(ById(p.Id))
			require.NoError(t, err)
			checkEqualVTs(t, testCase.expectedSyncVTs, dbPatch.SyncVariantsTasks)
			checkEqualVTs(t, testCase.expectedSyncVTs, p.SyncVariantsTasks)
		})
	}
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

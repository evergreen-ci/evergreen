package patch

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
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

	p, err := MakeNewMergePatch(pr, "mci", evergreen.CommitQueueAlias, "title", "message")
	s.NoError(err)
	s.Equal("mci", p.Project)
	s.Equal(evergreen.VersionCreated, p.Status)
	s.Equal(*pr.MergeCommitSHA, p.GithubPatchData.MergeCommitSHA)
	s.Equal("title", p.GithubPatchData.CommitTitle)
	s.Equal("message", p.GithubPatchData.CommitMessage)
}

func (s *patchSuite) TestUpdateGithashProjectAndTasks() {
	patch, err := FindOne(ByUserAndCommitQueue("octocat", false))
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

	s.NoError(patch.UpdateGithashProjectAndTasks())

	dbPatch, err := FindOne(ById(patch.Id))
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

func TestIsBackport(t *testing.T) {
	p := Patch{}
	assert.False(t, p.IsBackport())

	p.BackportOf.PatchID = "abc"
	assert.True(t, p.IsBackport())

	p = Patch{}
	p.BackportOf.SHA = "abc"
	assert.True(t, p.IsBackport())
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
				SyncAtEndOpts: SyncAtEndOptions{
					BuildVariants: testCase.bvs,
					Tasks:         testCase.tasks,
				},
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
				Id: bson.NewObjectId(),
				SyncAtEndOpts: SyncAtEndOptions{
					BuildVariants: testCase.syncBVs,
					Tasks:         testCase.syncTasks,
					VariantsTasks: testCase.existingSyncVTs,
				},
			}
			require.NoError(t, p.Insert())

			require.NoError(t, p.AddSyncVariantsTasks(testCase.newVTs))
			dbPatch, err := FindOne(ById(p.Id))
			require.NoError(t, err)
			checkEqualVTs(t, testCase.expectedSyncVTs, dbPatch.SyncAtEndOpts.VariantsTasks)
			checkEqualVTs(t, testCase.expectedSyncVTs, p.SyncAtEndOpts.VariantsTasks)
		})
	}
}

func TestMakeMergePatchPatches(t *testing.T) {
	require.NoError(t, db.ClearGridCollections(GridFSPrefix))
	patchDiff := "Lorem Ipsum"
	patchFileID := bson.NewObjectId()
	require.NoError(t, db.WriteGridFile(GridFSPrefix, patchFileID.Hex(), strings.NewReader(patchDiff)))

	existingPatch := &Patch{
		Patches: []ModulePatch{
			{
				ModuleName: "0",
				PatchSet: PatchSet{
					PatchFileId: patchFileID.Hex(),
				},
			},
		},
		GitInfo: &GitMetadata{
			Email:    "octocat@github.com",
			Username: "octocat",
		},
	}
	newPatches, err := MakeMergePatchPatches(existingPatch, "new message")
	assert.NoError(t, err)
	assert.Len(t, newPatches, 1)
	assert.NotEqual(t, patchFileID.Hex(), newPatches[0].PatchSet.PatchFileId)

	patchContents, err := FetchPatchContents(newPatches[0].PatchSet.PatchFileId)
	require.NoError(t, err)
	assert.Contains(t, patchContents, "From: octocat <octocat@github.com>")
	assert.Contains(t, patchContents, patchDiff)
}

func TestAddMetadataToDiff(t *testing.T) {
	diff := "+ func diffToMbox(diffData *localDiff, subject string) (string, error) {"
	commitTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := map[string]func(*testing.T){
		"without git version": func(t *testing.T) {
			metadata := GitMetadata{
				Username: "octocat",
				Email:    "octocat@github.com",
			}
			mboxDiff, err := addMetadataToDiff(diff, "EVG-12345 diff to mbox", commitTime, metadata)
			assert.NoError(t, err)
			assert.Equal(t, `From 72899681697bc4c45b1dae2c97c62e2e7e5d597b Mon Sep 17 00:00:00 2001
From: octocat <octocat@github.com>
Date: Tue, 10 Nov 2009 23:00:00 +0000
Subject: EVG-12345 diff to mbox

---
+ func diffToMbox(diffData *localDiff, subject string) (string, error) {
`, mboxDiff)
		},
		"with git version": func(t *testing.T) {
			metadata := GitMetadata{
				Username:   "octocat",
				Email:      "octocat@github.com",
				GitVersion: "2.19.1",
			}
			mboxDiff, err := addMetadataToDiff(diff, "EVG-12345 diff to mbox", commitTime, metadata)
			assert.NoError(t, err)
			assert.Equal(t, `From 72899681697bc4c45b1dae2c97c62e2e7e5d597b Mon Sep 17 00:00:00 2001
From: octocat <octocat@github.com>
Date: Tue, 10 Nov 2009 23:00:00 +0000
Subject: EVG-12345 diff to mbox

---
+ func diffToMbox(diffData *localDiff, subject string) (string, error) {
--
2.19.1

`, mboxDiff)
		},
	}

	for name, test := range tests {
		t.Run(name, test)
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
	assert.NoError(parentPatch.Insert())
	p := Patch{
		Id: bson.NewObjectId(),
		Triggers: TriggerInfo{
			ParentPatch: parentPatchID.Hex(),
		},
	}
	assert.NoError(p.Insert())
	_, err := p.SetParametersFromParent()
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
	assert.NoError(p.Insert())

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

	assert.NoError(p.SetDownstreamParameters(paramsToAdd))
	assert.Equal(p.Triggers.DownstreamParameters[0].Key, "key_0")
	assert.Equal(p.Triggers.DownstreamParameters[1].Key, "key_1")
	assert.Equal(p.Triggers.DownstreamParameters[2].Key, "key_2")
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
	assert.NoError(p.Insert())

	p.Triggers.Aliases = []string{
		"alias_1",
		"alias_2",
	}

	assert.NoError(p.SetTriggerAliases())

	dbPatch, err := FindOne(ById(p.Id))
	assert.NoError(err)
	assert.Equal(dbPatch.Triggers.Aliases[0], "alias_0")
	assert.Equal(dbPatch.Triggers.Aliases[1], "alias_1")
	assert.Equal(dbPatch.Triggers.Aliases[2], "alias_2")
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
	assert.NoError(p.Insert())

	p.Triggers.ChildPatches = []string{
		"id_1",
		"id_2",
	}

	assert.NoError(p.SetChildPatches())

	dbPatch, err := FindOne(ById(p.Id))
	assert.NoError(err)
	assert.Equal(dbPatch.Triggers.ChildPatches[0], "id_0")
	assert.Equal(dbPatch.Triggers.ChildPatches[1], "id_1")
	assert.Equal(dbPatch.Triggers.ChildPatches[2], "id_2")
}

func TestGetCollectiveStatusFromPatchStatuses(t *testing.T) {
	successful := []string{evergreen.VersionSucceeded}
	assert.Equal(t, evergreen.VersionSucceeded, GetCollectiveStatusFromPatchStatuses(successful))

	successfulLegacy := []string{evergreen.LegacyPatchSucceeded}
	assert.Equal(t, evergreen.VersionSucceeded, GetCollectiveStatusFromPatchStatuses(successfulLegacy))

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
		Alias: evergreen.CommitQueueAlias,
	}
	require.NoError(t, p1.Insert())

	p2 := Patch{
		Id:    bson.NewObjectId(),
		Alias: "",
		GithubPatchData: thirdparty.GithubPatch{
			HeadOwner: "me",
		},
	}
	require.NoError(t, p2.Insert())

	p3 := Patch{
		Id:    bson.NewObjectId(),
		Alias: evergreen.CommitQueueAlias,
		GithubMergeData: thirdparty.GithubMergeGroup{
			HeadSHA: "1234567",
		},
	}
	require.NoError(t, p3.Insert())

	p4 := Patch{
		Id:    bson.NewObjectId(),
		Alias: "",
	}
	require.NoError(t, p4.Insert())

	require.Equal(t, p1.GetRequester(), evergreen.MergeTestRequester)
	require.Equal(t, p2.GetRequester(), evergreen.GithubPRRequester)
	require.Equal(t, p3.GetRequester(), evergreen.GithubMergeRequester)
	require.Equal(t, p4.GetRequester(), evergreen.PatchVersionRequester)
}

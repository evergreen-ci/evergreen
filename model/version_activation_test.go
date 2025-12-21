package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type VersionActivationSuite struct {
	suite.Suite
	ctx context.Context
}

func TestVersionActivationSuite(t *testing.T) {
	suite.Run(t, new(VersionActivationSuite))
}

func (s *VersionActivationSuite) SetupTest() {
	s.ctx = context.Background()
	require.NoError(s.T(), db.ClearCollections(VersionCollection, ProjectRefCollection))
}

func (s *VersionActivationSuite) TestDoProjectActivationWithBuffer() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project"
	now := time.Now()

	projectRef := &ProjectRef{
		Id: projectID,
	}
	require.NoError(projectRef.Insert(s.ctx))

	// Create versions at different times
	versions := []Version{
		{
			Id:                  "version-1",
			Requester:           evergreen.RepotrackerVersionRequester,
			Identifier:          projectID,
			CreateTime:          now.Add(-15 * time.Minute),
			Ignored:             false,
			RevisionOrderNumber: 1,
		},
		{
			Id:                  "version-2",
			Requester:           evergreen.RepotrackerVersionRequester,
			Identifier:          projectID,
			CreateTime:          now.Add(-6 * time.Minute),
			Ignored:             false,
			RevisionOrderNumber: 2,
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "bv",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-7 * time.Minute),
					},
				},
			},
		},
		{
			Id:                  "version-3",
			Requester:           evergreen.RepotrackerVersionRequester,
			Identifier:          projectID,
			CreateTime:          now.Add(-4 * time.Minute), // within buffer
			Ignored:             false,
			RevisionOrderNumber: 3,
		},
		{
			Id:                  "version-4",
			Requester:           evergreen.RepotrackerVersionRequester,
			Identifier:          projectID,
			CreateTime:          now.Add(-1 * time.Minute), // within buffer
			Ignored:             false,
			RevisionOrderNumber: 4,
		},
	}

	// Insert versions
	for _, v := range versions {
		require.NoError(v.Insert(s.ctx))
	}

	// Test activation
	activated, err := DoProjectActivation(s.ctx, projectRef, now.Add(-CronActiveRange))
	require.NoError(err)
	require.True(activated)

	// Verify that we got the correct version (version-2)
	// This version should be selected because it's the most recent one outside the 5-minute buffer
	activatedVersion, err := VersionFindOne(s.ctx, VersionByMostRecentNonIgnored(projectID, now.Add(-CronActiveRange)))
	require.NoError(err)
	require.NotNil(activatedVersion)
	require.Equal("version-2", activatedVersion.Id)
}

func (s *VersionActivationSuite) TestDoProjectActivationSkipsIgnoredBuildVariants() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project"
	now := time.Now()

	projectRef := &ProjectRef{
		Id: projectID,
	}
	require.NoError(projectRef.Insert(s.ctx))

	// Create a version with both ignored and non-ignored build variants
	// This simulates build variants that were ignored due to path filtering
	version := Version{
		Id:                  "version-with-ignored-variants",
		Requester:           evergreen.RepotrackerVersionRequester,
		Identifier:          projectID,
		CreateTime:          now.Add(-10 * time.Minute),
		Ignored:             false,
		RevisionOrderNumber: 1,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "normal-variant",
				BuildId:      "build-1",
				Ignored:      false, // This variant matched the changed files
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: now.Add(-5 * time.Minute), // Elapsed
				},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName: "normal-task",
						TaskId:   "task-1",
						ActivationStatus: ActivationStatus{
							Activated:  false,
							ActivateAt: now.Add(-5 * time.Minute), // Elapsed
						},
					},
				},
			},
			{
				BuildVariant: "path-filtered-variant",
				BuildId:      "build-2",
				Ignored:      true, // This variant was ignored due to path filtering (changed files didn't match)
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: now.Add(-5 * time.Minute), // Elapsed but should be ignored
				},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName: "path-filtered-task",
						TaskId:   "task-2",
						ActivationStatus: ActivationStatus{
							Activated:  false,
							ActivateAt: now.Add(-5 * time.Minute), // Elapsed but should be ignored
						},
					},
				},
			},
			{
				BuildVariant: "another-normal-variant",
				BuildId:      "build-3",
				Ignored:      false, // This variant also matched the changed files
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: now.Add(-3 * time.Minute), // Elapsed
				},
			},
		},
	}

	// Insert the version
	require.NoError(version.Insert(s.ctx))

	// Test activation
	activated, err := DoProjectActivation(s.ctx, projectRef, now.Add(-CronActiveRange))
	require.NoError(err)
	require.True(activated)

	// Verify the version was processed
	updatedVersion, err := VersionFindOneId(s.ctx, version.Id)
	require.NoError(err)
	require.NotNil(updatedVersion)

	// Get the build variants to check their activation status
	_, err = updatedVersion.GetBuildVariants(s.ctx)
	require.NoError(err)

	// Verify that only non-ignored build variants were activated
	for _, bv := range updatedVersion.BuildVariants {
		if bv.Ignored {
			// Build variants ignored due to path filtering should remain unactivated
			require.False(bv.Activated, "Path-filtered build variant %s should not be activated", bv.BuildVariant)
			// Tasks in path-filtered build variants should also remain unactivated
			for _, task := range bv.BatchTimeTasks {
				require.False(task.Activated, "Task %s in path-filtered build variant %s should not be activated", task.TaskName, bv.BuildVariant)
			}
		} else {
			// Build variants that matched the changed files should be activated if their time has elapsed
			if bv.ShouldActivate(now) {
				require.True(bv.Activated, "Non-path-filtered build variant %s with elapsed time should be activated", bv.BuildVariant)
			}
			// Tasks in non-path-filtered build variants should be activated if their time has elapsed
			for _, task := range bv.BatchTimeTasks {
				if task.ShouldActivate(now) {
					require.True(task.Activated, "Task %s in non-path-filtered build variant %s with elapsed time should be activated", task.TaskName, bv.BuildVariant)
				}
			}
		}
	}
}

func (s *VersionActivationSuite) TestDoProjectActivationMultipleUnactivatedCommits() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project-multi"
	now := time.Now()

	projectRef := &ProjectRef{
		Id:                     projectID,
		RunEveryMainlineCommit: true,
	}
	require.NoError(projectRef.Insert(s.ctx))

	// Create an older activated version (last activated reference point)
	activatedVersion := &Version{
		Id:                  "version-activated",
		Identifier:          projectID,
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          now.Add(-10 * time.Minute),
		Revision:            "activated123",
		RevisionOrderNumber: 1,
		Activated:           utility.ToBoolPtr(true), // This version is already activated
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "test-variant-activated",
				BuildId:      "build-activated",
				ActivationStatus: ActivationStatus{
					Activated:  true, // Already activated
					ActivateAt: now.Add(-10 * time.Minute),
				},
			},
		},
	}

	// Create multiple unactivated versions after the activated one
	unactivatedVersions := []*Version{
		{
			Id:                  "version-1",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-5 * time.Minute),
			Revision:            "abc123",
			RevisionOrderNumber: 2,                        // Higher order number (newer)
			Activated:           utility.ToBoolPtr(false), // Not activated
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "test-variant-1",
					BuildId:      "build-1",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-6 * time.Minute), // Elapsed
					},
				},
			},
		},
		{
			Id:                  "version-2",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-3 * time.Minute),
			Revision:            "def456",
			RevisionOrderNumber: 3,                        // Higher order number (newer)
			Activated:           utility.ToBoolPtr(false), // Not activated
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "test-variant-2",
					BuildId:      "build-2",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-4 * time.Minute), // Elapsed
					},
				},
			},
		},
		{
			Id:                  "version-3",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-1 * time.Minute),
			Revision:            "ghi789",
			RevisionOrderNumber: 4,                        // Highest order number (newest)
			Activated:           utility.ToBoolPtr(false), // Not activated
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "test-variant-3",
					BuildId:      "build-3",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-2 * time.Minute), // Elapsed
					},
				},
			},
		},
	}

	// Insert the activated version first
	require.NoError(activatedVersion.Insert(s.ctx))

	// Insert all unactivated versions
	for _, version := range unactivatedVersions {
		require.NoError(version.Insert(s.ctx))
	}

	// Test activation - should activate all unactivated commits since the last activated one
	activated, err := DoProjectActivation(s.ctx, projectRef, now)
	require.NoError(err)
	require.True(activated)

	// Verify all unactivated versions were activated
	for _, originalVersion := range unactivatedVersions {
		updatedVersion, err := VersionFindOneId(s.ctx, originalVersion.Id)
		require.NoError(err)
		require.NotNil(updatedVersion)

		_, err = updatedVersion.GetBuildVariants(s.ctx)
		require.NoError(err)

		// Check that the build variant was activated
		require.Len(updatedVersion.BuildVariants, 1)
		require.True(updatedVersion.BuildVariants[0].Activated,
			"Version %s should have been activated", originalVersion.Id)
	}

	// Verify the already activated version remains activated
	updatedActivatedVersion, err := VersionFindOneId(s.ctx, activatedVersion.Id)
	require.NoError(err)
	require.NotNil(updatedActivatedVersion)
	require.True(utility.FromBoolPtr(updatedActivatedVersion.Activated))
}

func (s *VersionActivationSuite) TestDoProjectActivationNewProject() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project-new"
	now := time.Now()

	projectRef := &ProjectRef{
		Id:                     projectID,
		RunEveryMainlineCommit: true,
	}
	require.NoError(projectRef.Insert(s.ctx))

	// Create multiple versions with no previously activated versions (simulating a new project)
	versions := []*Version{
		{
			Id:                  "version-1",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-10 * time.Minute),
			Revision:            "commit1",
			RevisionOrderNumber: 1,
			Activated:           utility.ToBoolPtr(false), // Not activated
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "test-variant",
					BuildId:      "build-1",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-11 * time.Minute), // Elapsed
					},
				},
			},
		},
		{
			Id:                  "version-2",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-5 * time.Minute),
			Revision:            "commit2",
			RevisionOrderNumber: 2,
			Activated:           utility.ToBoolPtr(false), // Not activated
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "test-variant",
					BuildId:      "build-2",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-6 * time.Minute), // Elapsed
					},
				},
			},
		},
		{
			Id:                  "version-3",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-2 * time.Minute),
			Revision:            "commit3",
			RevisionOrderNumber: 3,                        // Most recent
			Activated:           utility.ToBoolPtr(false), // Not activated
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant: "test-variant",
					BuildId:      "build-3",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: now.Add(-3 * time.Minute), // Elapsed
					},
				},
			},
		},
	}

	// Insert all versions
	for _, version := range versions {
		require.NoError(version.Insert(s.ctx))
	}

	// Test activation - should activate ALL versions when no previously activated versions exist
	// This ensures new projects don't miss any commits
	activated, err := DoProjectActivation(s.ctx, projectRef, now)
	require.NoError(err)
	require.True(activated)

	// Verify ALL versions were activated (critical for new projects)
	for _, originalVersion := range versions {
		updatedVersion, err := VersionFindOneId(s.ctx, originalVersion.Id)
		require.NoError(err)
		require.NotNil(updatedVersion)

		_, err = updatedVersion.GetBuildVariants(s.ctx)
		require.NoError(err)

		require.True(updatedVersion.BuildVariants[0].Activated,
			"Version %s should be activated in new project", originalVersion.Id)
	}
}

func (s *VersionActivationSuite) TestDoProjectActivationSingleCommitBehaviorPreserved() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project-single"
	now := time.Now()

	projectRef := &ProjectRef{
		Id:                     projectID,
		RunEveryMainlineCommit: true,
	}
	require.NoError(projectRef.Insert(s.ctx))

	// Create a single version
	version := &Version{
		Id:                  "single-version",
		Identifier:          projectID,
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          now.Add(-2 * time.Minute),
		Revision:            "single123",
		RevisionOrderNumber: 1,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "test-variant",
				BuildId:      "build-single",
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: now.Add(-3 * time.Minute), // Elapsed
				},
			},
		},
	}

	require.NoError(version.Insert(s.ctx))

	// Test activation - should work exactly as before
	activated, err := DoProjectActivation(s.ctx, projectRef, now)
	require.NoError(err)
	require.True(activated)

	// Verify version was activated
	updatedVersion, err := VersionFindOneId(s.ctx, "single-version")
	require.NoError(err)
	require.NotNil(updatedVersion)
	_, err = updatedVersion.GetBuildVariants(s.ctx)
	require.NoError(err)
	require.True(updatedVersion.BuildVariants[0].Activated, "Single version should be activated")
}

func (s *VersionActivationSuite) TestVersionsUnactivatedSinceLastActivated() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project-query"
	now := time.Now()

	// Create versions with different activation states
	versions := []*Version{
		{
			Id:                  "activated-version",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-10 * time.Minute),
			Revision:            "activated123",
			RevisionOrderNumber: 1,
			Activated:           utility.ToBoolPtr(true), // Activated
		},
		{
			Id:                  "unactivated-1",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-5 * time.Minute),
			Revision:            "unactivated123",
			RevisionOrderNumber: 2,                        // After activated version
			Activated:           utility.ToBoolPtr(false), // Not activated
		},
		{
			Id:                  "unactivated-2",
			Identifier:          projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
			CreateTime:          now.Add(-2 * time.Minute),
			Revision:            "unactivated456",
			RevisionOrderNumber: 3,                        // After activated version
			Activated:           utility.ToBoolPtr(false), // Not activated
		},
	}

	// Insert all versions
	for _, version := range versions {
		require.NoError(version.Insert(s.ctx))
	}

	// Test the query to find unactivated versions since last activated
	unactivatedVersions, err := VersionFind(s.ctx, VersionsUnactivatedSinceLastActivated(projectID, now, 1, 1000))
	require.NoError(err)
	require.Len(unactivatedVersions, 2, "Should find 2 unactivated versions after the activated one")

	// Verify the correct versions were returned
	foundIds := make(map[string]bool)
	for _, v := range unactivatedVersions {
		foundIds[v.Id] = true
	}
	require.True(foundIds["unactivated-1"], "Should include unactivated-1")
	require.True(foundIds["unactivated-2"], "Should include unactivated-2")
	require.False(foundIds["activated-version"], "Should not include activated version")
}

func (s *VersionActivationSuite) TestDoProjectActivationNoVersionsToActivate() {
	t := s.T()
	require := require.New(t)

	projectID := "test-project-empty"
	now := time.Now()

	projectRef := &ProjectRef{
		Id:                     projectID,
		RunEveryMainlineCommit: true,
	}
	require.NoError(projectRef.Insert(s.ctx))

	// Test activation with no versions
	activated, err := DoProjectActivation(s.ctx, projectRef, now)
	require.NoError(err)
	require.False(activated)
}

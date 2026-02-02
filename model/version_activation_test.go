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
	activated, err := DoProjectActivation(s.ctx, projectID, now.Add(-CronActiveRange))
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

	// Create a version with both ignored and non-ignored build variants.
	// This simulates build variants that were ignored due to path filtering, i.e. activate time is zero.
	version := Version{
		Id:                  "version-with-ignored-variants",
		Requester:           evergreen.RepotrackerVersionRequester,
		Identifier:          projectID,
		CreateTime:          now.Add(-10 * time.Minute),
		Ignored:             false,
		RevisionOrderNumber: 1,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "normal-variant", // i.e. this variant doesn't have path filtering behavior.
				BuildId:      "build-1",
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
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: utility.ZeroTime, // This should be respected and prevent general activation
				},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName: "cron-task",
						TaskId:   "task-2",
						ActivationStatus: ActivationStatus{
							Activated:  false,
							ActivateAt: now.Add(-5 * time.Minute), // This should still be respected
						},
					},
				},
			},
		},
	}

	// Insert the version
	require.NoError(version.Insert(s.ctx))

	// Test activation
	activated, err := DoProjectActivation(s.ctx, projectID, now.Add(-CronActiveRange))
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
		if bv.DisplayName == "path-filtered-variant" {
			// Build variants ignored due to path filtering should remain unactivated
			require.False(bv.Activated, "Path-filtered build variant %s should not be activated", bv.BuildVariant)
			// Tasks in path-filtered build variants should be activated, i.e. respect cron/batchtime behavior
			for _, task := range bv.BatchTimeTasks {
				require.False(task.Activated, "Task %s in path-filtered build variant %s should be activated", task.TaskName, bv.BuildVariant)
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

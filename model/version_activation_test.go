package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
	require.NoError(s.T(), db.ClearCollections(VersionCollection))
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

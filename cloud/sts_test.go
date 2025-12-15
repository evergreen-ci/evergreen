package cloud

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssumeRole(t *testing.T) {
	taskID := "task_id"
	projectID := "project_id"
	repoRefID := "repo_ref_id"
	requester := "requester"

	roleARN := "role_arn"
	policy := "policy"
	externalID := fmt.Sprintf("%s-%s", projectID, requester)
	externalIDUntracked := fmt.Sprintf("untracked-%s-%s", repoRefID, requester)

	testCases := map[string]func(t *testing.T, manager STSManager, awsClientMock *awsClientMock){
		"InvalidTask": func(t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			_, err := manager.AssumeRole(t.Context(), taskID, AssumeRoleOptions{
				RoleARN: roleARN,
				Policy:  &policy,
			})
			require.ErrorContains(t, err, fmt.Sprintf("task '%s' not found", taskID))
		},
		"Success": func(t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			task := task.Task{Id: taskID, Project: projectID, Requester: requester}
			require.NoError(t, task.Insert(t.Context()))
			project := model.ProjectRef{Id: projectID, RepoRefId: repoRefID}
			require.NoError(t, project.Insert(t.Context()))
			// Only being attached to a repo shouldn't affect the outcome, since
			// the project ref is a tracked branch.
			repoRef := model.RepoRef{ProjectRef: model.ProjectRef{Id: repoRefID}}
			require.NoError(t, repoRef.Replace(t.Context()))

			creds, err := manager.AssumeRole(t.Context(), taskID, AssumeRoleOptions{
				RoleARN:         roleARN,
				Policy:          &policy,
				DurationSeconds: aws.Int32(int32(time.Hour.Seconds())),
			})
			require.NoError(t, err)
			// Return credentials
			assert.Equal(t, "access_key", creds.AccessKeyID)
			assert.Equal(t, "secret_key", creds.SecretAccessKey)
			assert.Equal(t, "session_token", creds.SessionToken)
			assert.WithinDuration(t, time.Now().Add(time.Hour), creds.Expiration, time.Second/4)

			// Mock implementation received the correct input from the manager.
			assert.Equal(t, roleARN, utility.FromStringPtr(awsClientMock.AssumeRoleInput.RoleArn))
			assert.Equal(t, policy, utility.FromStringPtr(awsClientMock.AssumeRoleInput.Policy))
			assert.Equal(t, externalID, utility.FromStringPtr(awsClientMock.AssumeRoleInput.ExternalId))
		},
		"Success/UntrackedBranch": func(t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			task := task.Task{Id: taskID, Project: projectID, Requester: requester}
			require.NoError(t, task.Insert(t.Context()))
			project := model.ProjectRef{
				Id:        projectID,
				RepoRefId: repoRefID,
				Enabled:   false,
				Hidden:    utility.TruePtr(),
			}
			require.True(t, project.IsUntracked())
			require.NoError(t, project.Insert(t.Context()))
			// Only being attached to a repo shouldn't affect the outcome, since
			// the project ref is a tracked branch.
			repoRef := model.RepoRef{ProjectRef: model.ProjectRef{Id: repoRefID}}
			require.NoError(t, repoRef.Replace(t.Context()))

			creds, err := manager.AssumeRole(t.Context(), taskID, AssumeRoleOptions{
				RoleARN:         roleARN,
				Policy:          &policy,
				DurationSeconds: aws.Int32(int32(time.Hour.Seconds())),
			})
			require.NoError(t, err)
			// Return credentials
			assert.Equal(t, "access_key", creds.AccessKeyID)
			assert.Equal(t, "secret_key", creds.SecretAccessKey)
			assert.Equal(t, "session_token", creds.SessionToken)
			assert.WithinDuration(t, time.Now().Add(time.Hour), creds.Expiration, time.Second/4)

			// Mock implementation received the correct input from the manager.
			assert.Equal(t, roleARN, utility.FromStringPtr(awsClientMock.AssumeRoleInput.RoleArn))
			assert.Equal(t, policy, utility.FromStringPtr(awsClientMock.AssumeRoleInput.Policy))
			assert.Equal(t, externalIDUntracked, utility.FromStringPtr(awsClientMock.AssumeRoleInput.ExternalId))
		},
	}
	for tName, tCase := range testCases {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, model.RepoRefCollection))

			manager := GetSTSManager(true)
			stsManagerImpl, ok := manager.(*stsManagerImpl)
			require.True(t, ok)
			awsClientMock, ok := stsManagerImpl.client.(*awsClientMock)
			require.True(t, ok)

			tCase(t, manager, awsClientMock)
		})
	}
}

func TestGetCallerIdentity(t *testing.T) {
	roleARN := "role_arn"

	testCases := map[string]func(t *testing.T, manager STSManager, awsClientMock *awsClientMock){
		"InvalidReturnedARN": func(t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			awsClientMock.GetCallerIdentityOutput = &sts.GetCallerIdentityOutput{
				Arn: nil,
			}
			_, err := manager.GetCallerIdentityARN(t.Context())
			require.ErrorContains(t, err, "caller identity ARN is nil")
		},
		"Success": func(t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			awsClientMock.GetCallerIdentityOutput = &sts.GetCallerIdentityOutput{
				Arn: &roleARN,
			}

			arn, err := manager.GetCallerIdentityARN(t.Context())
			require.NoError(t, err)
			assert.Equal(t, roleARN, arn)
		},
	}
	for tName, tCase := range testCases {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection))

			manager := GetSTSManager(true)
			stsManagerImpl, ok := manager.(*stsManagerImpl)
			require.True(t, ok)
			awsClientMock, ok := stsManagerImpl.client.(*awsClientMock)
			require.True(t, ok)

			tCase(t, manager, awsClientMock)
		})
	}
}

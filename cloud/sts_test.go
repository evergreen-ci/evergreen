package cloud

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssumeRole(t *testing.T) {
	taskID := "task_id"
	projectID := "project_id"
	requester := "requester"

	roleARN := "role_arn"
	policy := "policy"
	externalID := fmt.Sprintf("%s-%s", projectID, requester)

	testCases := map[string]func(ctx context.Context, t *testing.T, manager STSManager, awsClientMock *awsClientMock){
		"InvalidTask": func(ctx context.Context, t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			_, err := manager.AssumeRole(ctx, taskID, AssumeRoleOptions{
				RoleARN: roleARN,
				Policy:  &policy,
			})
			require.ErrorContains(t, err, "task not found")
		},
		"Success": func(ctx context.Context, t *testing.T, manager STSManager, awsClientMock *awsClientMock) {
			task := task.Task{Id: taskID, Project: projectID, Requester: requester}
			require.NoError(t, task.Insert())

			creds, err := manager.AssumeRole(ctx, taskID, AssumeRoleOptions{
				RoleARN: roleARN,
				Policy:  &policy,
			})
			require.NoError(t, err)
			// Return credentials
			assert.Equal(t, "access_key", creds.AccessKeyID)
			assert.Equal(t, "secret_key", creds.SecretAccessKey)
			assert.Equal(t, "session_token", creds.SessionToken)
			assert.WithinDuration(t, time.Now().Add(time.Hour), creds.Expiration, time.Second)

			// Mock implementation received the correct input from the manager.
			assert.Equal(t, roleARN, utility.FromStringPtr(awsClientMock.AssumeRoleInput.RoleArn))
			assert.Equal(t, policy, utility.FromStringPtr(awsClientMock.AssumeRoleInput.Policy))
			assert.Equal(t, externalID, utility.FromStringPtr(awsClientMock.AssumeRoleInput.ExternalId))
		},
	}
	for tName, tCase := range testCases {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			manager := GetSTSManager(true)
			stsManagerImpl, ok := manager.(*stsManagerImpl)
			require.True(t, ok)
			awsClientMock, ok := stsManagerImpl.client.(*awsClientMock)
			require.True(t, ok)

			tCase(ctx, t, manager, awsClientMock)
		})
	}

}

package command

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvergreenCredentials(t *testing.T) {
	comm := client.NewMock("localhost")
	taskData := client.TaskData{ID: "task_id", Secret: "task_secret"}

	t.Run("ImplmenetsCredentialsProvider", func(t *testing.T) {
		provider := createEvergreenCredentials(comm, taskData, "")
		assert.Implements(t, (*aws.CredentialsProvider)(nil), provider)
	})

	t.Run("RoleARN", func(t *testing.T) {
		expires := time.Now().Add(time.Hour).Format(time.RFC3339)
		comm.AssumeRoleResponse = &apimodels.AWSCredentials{
			AccessKeyID:     "assume_access_key",
			SecretAccessKey: "secret_access_key",
			SessionToken:    "session_token",
			Expiration:      expires,
		}

		provider := createEvergreenCredentials(comm, taskData, "role_arn")

		creds, err := provider.Retrieve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "assume_access_key", creds.AccessKeyID)
		assert.Equal(t, "secret_access_key", creds.SecretAccessKey)
		assert.Equal(t, "session_token", creds.SessionToken)
		assert.Equal(t, expires, creds.Expires.Format(time.RFC3339))
	})
}

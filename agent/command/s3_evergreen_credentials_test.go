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
		provider := createEvergreenCredentials(comm, taskData, nil, "", nil)
		assert.Implements(t, (*aws.CredentialsProvider)(nil), provider)
	})

	t.Run("RoleARN", func(t *testing.T) {
		t.Run("TimeFormat", func(t *testing.T) {
			t.Run("Valid", func(t *testing.T) {
				expires := time.Now().Add(time.Hour).Format(time.RFC3339)
				comm.AssumeRoleResponse = &apimodels.AWSCredentials{
					AccessKeyID:     "assume_access_key",
					SecretAccessKey: "secret_access_key",
					SessionToken:    "session_token",
					Expiration:      expires,
					ExternalID:      "external_id",
				}

				var externalID *string
				provider := createEvergreenCredentials(comm, taskData, nil, "role_arn", func(s string) {
					externalID = &s
				})

				creds, err := provider.Retrieve(t.Context())
				require.NoError(t, err)
				assert.Equal(t, "assume_access_key", creds.AccessKeyID)
				assert.Equal(t, "secret_access_key", creds.SecretAccessKey)
				assert.Equal(t, "session_token", creds.SessionToken)
				assert.Equal(t, expires, creds.Expires.Format(time.RFC3339))
				require.NotNil(t, externalID)
				assert.Equal(t, "external_id", *externalID)
			})

			t.Run("Invalid", func(t *testing.T) {
				expires := time.Now().Add(time.Hour).String()
				comm.AssumeRoleResponse = &apimodels.AWSCredentials{
					AccessKeyID:     "assume_access_key",
					SecretAccessKey: "secret_access_key",
					SessionToken:    "session_token",
					Expiration:      expires,
				}

				provider := createEvergreenCredentials(comm, taskData, nil, "role_arn", nil)

				creds, err := provider.Retrieve(t.Context())
				require.Error(t, err)
				assert.Empty(t, creds)
			})
		})

		t.Run("PassesWithNilExternalID", func(t *testing.T) {
			expires := time.Now().Add(time.Hour).Format(time.RFC3339)
			comm.AssumeRoleResponse = &apimodels.AWSCredentials{
				AccessKeyID:     "assume_access_key",
				SecretAccessKey: "secret_access_key",
				SessionToken:    "session_token",
				Expiration:      expires,
				ExternalID:      "external_id",
			}

			provider := createEvergreenCredentials(comm, taskData, nil, "role_arn", nil)

			creds, err := provider.Retrieve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, "assume_access_key", creds.AccessKeyID)
			assert.Equal(t, "secret_access_key", creds.SecretAccessKey)
			assert.Equal(t, "session_token", creds.SessionToken)
			assert.Equal(t, expires, creds.Expires.Format(time.RFC3339))
			assert.Nil(t, provider.updateExternalID)
		})

		t.Run("ExistingCredentials", func(t *testing.T) {
			t.Run("NotExpired", func(t *testing.T) {
				assumeRoleExpires := time.Now().Add(time.Hour).Format(time.RFC3339)
				comm.AssumeRoleResponse = &apimodels.AWSCredentials{
					AccessKeyID:     "assume_access_key",
					SecretAccessKey: "secret_access_key",
					SessionToken:    "session_token",
					Expiration:      assumeRoleExpires,
					ExternalID:      "external_id",
				}
				existingExpires := time.Now().Add(expiryWindow + time.Second)
				existingCredentials := &aws.Credentials{
					AccessKeyID:     "existing_access_key",
					SecretAccessKey: "existing_secret_access_key",
					SessionToken:    "existing_session_token",
					Expires:         existingExpires,
					CanExpire:       true,
				}

				provider := createEvergreenCredentials(comm, taskData, existingCredentials, "role_arn", nil)

				creds, err := provider.Retrieve(t.Context())
				require.NoError(t, err)
				assert.Equal(t, "existing_access_key", creds.AccessKeyID)
				assert.Equal(t, "existing_secret_access_key", creds.SecretAccessKey)
				assert.Equal(t, "existing_session_token", creds.SessionToken)
				// The expiry window is subtracted from the existing credentials to
				// as that's what the provider does to ensure valid credentials.
				assert.Equal(t, existingExpires.Add(-expiryWindow).Format(time.RFC3339), creds.Expires.Format(time.RFC3339))
				assert.Nil(t, provider.updateExternalID)

				t.Run("AssumeRoleAfterExistingExpires", func(t *testing.T) {
					existingCredentials.Expires = time.Now().Add(-time.Hour)

					creds, err = provider.Retrieve(t.Context())
					require.NoError(t, err)
					assert.Equal(t, "assume_access_key", creds.AccessKeyID)
					assert.Equal(t, "secret_access_key", creds.SecretAccessKey)
					assert.Equal(t, "session_token", creds.SessionToken)
					assert.Equal(t, assumeRoleExpires, creds.Expires.Format(time.RFC3339))
					assert.Nil(t, provider.updateExternalID)
				})
			})

			t.Run("Expired", func(t *testing.T) {
				expires := time.Now().Add(time.Hour).Format(time.RFC3339)
				comm.AssumeRoleResponse = &apimodels.AWSCredentials{
					AccessKeyID:     "assume_access_key",
					SecretAccessKey: "secret_access_key",
					SessionToken:    "session_token",
					Expiration:      expires,
					ExternalID:      "external_id",
				}
				existingCredentials := &aws.Credentials{
					AccessKeyID:     "existing_access_key",
					SecretAccessKey: "existing_secret_access_key",
					SessionToken:    "existing_session_token",
					Expires:         time.Now().Add(-time.Hour),
					CanExpire:       true,
				}

				provider := createEvergreenCredentials(comm, taskData, existingCredentials, "role_arn", nil)

				creds, err := provider.Retrieve(t.Context())
				require.NoError(t, err)
				assert.Equal(t, "assume_access_key", creds.AccessKeyID)
				assert.Equal(t, "secret_access_key", creds.SecretAccessKey)
				assert.Equal(t, "session_token", creds.SessionToken)
				assert.Equal(t, expires, creds.Expires.Format(time.RFC3339))
				assert.Nil(t, provider.updateExternalID)
			})
		})
	})
}

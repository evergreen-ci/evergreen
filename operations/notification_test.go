package operations

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestNotificationSlackCommand(t *testing.T) {
	for testName, testCase := range map[string]struct {
		args      []string
		expectErr bool
		expected  *restmodel.APISlack
	}{
		"FailsWithMissingTarget": {
			args:      []string{"notify", "slack", "--msg", "Test message"},
			expectErr: true,
		},
		"FailsWithMissingMessage": {
			args:      []string{"notify", "slack", "--target", "channel"},
			expectErr: true,
		},
		"SucceedsWithValidInput": {
			args:      []string{"notify", "slack", "--target", "channel", "--msg", "Test message"},
			expectErr: false,
			expected: &restmodel.APISlack{
				Target: utility.ToStringPtr("channel"),
				Msg:    utility.ToStringPtr("Test message"),
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			mockClient = &client.Mock{}

			app := cli.NewApp()
			app.Commands = []cli.Command{Notification()}

			set := flag.NewFlagSet("test", 0)
			require.NoError(t, set.Parse(testCase.args))

			ctx := cli.NewContext(app, set, nil)
			err := Notification().Run(ctx)

			if testCase.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.expected, mockClient.SendSlackNotificationData)
		})
	}
}

func TestNotificationEmailCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "email_body.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("Test Email Body"), 0644))

	for testName, testCase := range map[string]struct {
		args      []string
		bodyFile  string
		body      string
		expectErr bool
		expected  *restmodel.APIEmail
	}{
		"FailsWithMissingBodyFile": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--bodyFile", "nonexistent.txt"},
			bodyFile:  "nonexistent.txt",
			expectErr: true,
		},
		"FailsWithMissingRecipients": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--body", "Test body"},
			expectErr: true,
		},
		"SucceedsWithValidBodyFile": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--bodyFile", testFile},
			bodyFile:  testFile,
			expectErr: false,
			expected: &restmodel.APIEmail{
				Subject:    utility.ToStringPtr("Test Subject"),
				Recipients: []string{"test@example.com"},
				Body:       utility.ToStringPtr("Test Email Body"),
			},
		},
		"SucceedsWithInlineBody": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--body", "Inline Email Body"},
			body:      "Inline Email Body",
			expectErr: false,
			expected: &restmodel.APIEmail{
				Subject:    utility.ToStringPtr("Test Subject"),
				Recipients: []string{"test@example.com"},
				Body:       utility.ToStringPtr("Inline Email Body"),
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			mockClient = &client.Mock{}

			app := cli.NewApp()
			app.Commands = []cli.Command{Notification()}

			set := flag.NewFlagSet("test", 0)
			require.NoError(t, set.Parse(testCase.args))

			ctx := cli.NewContext(app, set, nil)
			err := Notification().Run(ctx)
			if testCase.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.expected, mockClient.SendEmailNotificationData)
		})
	}
}

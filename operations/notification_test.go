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
		expectErr string
		expected  *restmodel.APISlack
	}{
		"FailsWithMissingTarget": {
			args:      []string{"notify", "slack", "--msg", "Test message"},
			expectErr: "--target",
		},
		"FailsWithMissingMessage": {
			args:      []string{"notify", "slack", "--target", "channel"},
			expectErr: "--msg",
		},
		"SucceedsWithValidInput": {
			args: []string{"notify", "slack", "--target", "channel", "--msg", "Test message"},
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

			if testCase.expectErr != "" {
				assert.ErrorContains(t, err, testCase.expectErr)
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
	emptyTestFile := filepath.Join(tmpDir, "empty_email_body.txt")
	require.NoError(t, os.WriteFile(emptyTestFile, []byte{}, 0644))

	for testName, testCase := range map[string]struct {
		args      []string
		body      string
		expectErr string
		expected  *restmodel.APIEmail
	}{
		"FailsWithMissingBodyFile": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--bodyFile", "nonexistent.txt"},
			expectErr: "reading email body from file",
		},
		"FailsWithMissingRecipients": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--body", "Test body"},
			expectErr: "--recipients",
		},
		"FailsWithMissingSubject": {
			args:      []string{"notify", "email", "--recipients", "test@example.com", "--body", "Test body"},
			expectErr: "--subject",
		},
		"FailsWithMissingBody": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com"},
			expectErr: "body | bodyFile",
		},
		"FailsWithEmptyFileBody": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--bodyFile", emptyTestFile},
			expectErr: "the given body file has no content",
		},
		"FailsWithInlineBodyAndBodyFile": {
			args:      []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--body", "Inline Email Body", "--bodyFile", emptyTestFile},
			expectErr: "only one of (body | bodyFile) can be set",
		},
		"SucceedsWithValidBodyFile": {
			args: []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--bodyFile", testFile},
			expected: &restmodel.APIEmail{
				Subject:    utility.ToStringPtr("Test Subject"),
				Recipients: []string{"test@example.com"},
				Body:       utility.ToStringPtr("Test Email Body"),
			},
		},
		"SucceedsWithInlineBody": {
			args: []string{"notify", "email", "--subject", "Test Subject", "--recipients", "test@example.com", "--body", "Inline Email Body"},
			body: "Inline Email Body",
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
			if testCase.expectErr != "" {
				assert.ErrorContains(t, err, testCase.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.expected, mockClient.SendEmailNotificationData)
		})
	}
}

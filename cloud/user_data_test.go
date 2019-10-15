package cloud

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteUserDataHeaders(t *testing.T) {
	buf := &bytes.Buffer{}
	boundary := "some_boundary"
	require.NoError(t, writeUserDataHeaders(buf, boundary))
	res := strings.ToLower(buf.String())
	assert.Contains(t, res, "mime-version: 1.0")
	assert.Contains(t, res, "content-type: multipart/mixed")
	assert.Contains(t, res, fmt.Sprintf("boundary=\"%s\"", boundary))
	assert.Equal(t, 1, strings.Count(res, boundary))
}

func TestParseUserDataContentType(t *testing.T) {
	for _, userData := range []string{
		"#!/bin/bash\necho 'foobar'",
		"#include\nhttps://example.com/foobar.txt",
		"#cloud-config\nruncmd:\n  - echo 'foobar'",
		"#upstart-job\ndescription: \"foobar\"",
		"#cloud-boothook\necho 'foobar'",
		"#part-handler\ndef list_types():\nreturn(['foobar'])\ndef handle_part(data,ctype,filename,payload):\nprint 'foobar'\nreturn",
	} {
		contentType, err := parseUserDataContentType(userData)
		require.NoError(t, err)
		assert.NotEmpty(t, contentType)
	}
	_, err := parseUserDataContentType("foo\nbar")
	assert.Error(t, err)
}

func TestWriteUserDataPart(t *testing.T) {
	buf := &bytes.Buffer{}
	mimeWriter := multipart.NewWriter(buf)
	boundary := "some_boundary"
	require.NoError(t, mimeWriter.SetBoundary(boundary))

	userData := "#!/bin/bash\necho 'foobar'"
	require.NoError(t, writeUserDataPart(mimeWriter, userData, "foobar.txt"))

	res := strings.ToLower(buf.String())
	assert.Contains(t, res, "mime-version: 1.0")
	assert.Contains(t, res, "content-type: text/x-shellscript")
	assert.Contains(t, res, "content-disposition: attachment; filename=\"foobar.txt\"")
	assert.Contains(t, res, userData)
	assert.Equal(t, 1, strings.Count(res, boundary))
}

func TestWriteUserDataPartDefaultForUnrecognizedFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	mimeWriter := multipart.NewWriter(buf)
	userData := "this user data has no cloud-init directive"
	require.NoError(t, writeUserDataPart(mimeWriter, userData, "foo.txt"))
	assert.Contains(t, buf.String(), "Content-Type: text/x-shellscript")
}

func TestWriteUserDataPartEmptyFileName(t *testing.T) {
	buf := &bytes.Buffer{}
	mimeWriter := multipart.NewWriter(buf)
	userData := "#!/bin/bash\necho 'foobar'"
	assert.Error(t, writeUserDataPart(mimeWriter, userData, ""))
}

func TestMakeMultipartUserData(t *testing.T) {
	userData := "#!/bin/bash\necho 'foobar'"
	noUserData := ""
	fileOne := "1.txt"
	fileTwo := "2.txt"

	res, err := makeMultipartUserData(map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, res)

	res, err = makeMultipartUserData(map[string]string{
		fileOne: noUserData,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res)
	assert.False(t, strings.Contains(res, fileOne))

	res, err = makeMultipartUserData(map[string]string{
		fileOne: userData,
		fileTwo: userData,
	})
	require.NoError(t, err)
	assert.Contains(t, res, fileOne)
	assert.Contains(t, res, fileTwo)
	assert.Equal(t, 2, strings.Count(res, userData))

	res, err = makeMultipartUserData(map[string]string{
		fileOne: noUserData,
		fileTwo: userData,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res)
	assert.False(t, strings.Contains(res, fileOne))
	assert.Contains(t, res, fileTwo)
	assert.Contains(t, res, userData)
}

func withCredentialsBootstrap(t *testing.T, fn func(settings *evergreen.Settings)) {
	env := &mock.Environment{}
	var cancel context.CancelFunc
	env.EnvContext, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, env.Configure(env.EnvContext, "", nil))
	env.Settings().DomainName = "test"

	require.NoError(t, db.ClearCollections(evergreen.CredentialsCollection, host.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(evergreen.CredentialsCollection, host.Collection))
	}()

	fn(env.Settings())
}

func TestBootstrapUserData(t *testing.T) {
	tctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tenv := testutil.NewEnvironment(tctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string){
		"ContainsCommandsToSetupHostForRunningTasks": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			userData, err := bootstrapUserData(ctx, env, h, "")
			require.NoError(t, err)

			cmd, err := h.SetupScriptCommands(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd = h.CurlCommandWithRetry(env.Settings(), host.CurlDefaultNumRetries, host.CurlDefaultMaxSecs)
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.StartAgentMonitorRequest(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.SetupSpawnHostCommand(env.Settings())
			require.NoError(t, err)
			assert.NotContains(t, userData, cmd)

			cmd, err = h.MarkUserDataDoneCommands()
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)
		},
		"ContainsCommandsToSetupSpawnHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			h.StartedBy = ""
			userData, err := bootstrapUserData(ctx, env, h, "")
			require.NoError(t, err)

			cmd, err := h.SetupScriptCommands(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd = h.CurlCommandWithRetry(env.Settings(), host.CurlDefaultNumRetries, host.CurlDefaultMaxSecs)
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.StartAgentMonitorRequest(env.Settings())
			require.NoError(t, err)
			assert.NotContains(t, userData, cmd)

			cmd, err = h.SetupSpawnHostCommand(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.MarkUserDataDoneCommands()
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)
		},
		"ChecksProvisionOptionsForSpawnHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			h.StartedBy = ""
			h.ProvisionOptions = &host.ProvisionOptions{LoadCLI: true}

			_, err := bootstrapUserData(ctx, env, h, "")
			assert.Error(t, err)
		},
		"PassesWithoutCustomUserData": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			userData, err := bootstrapUserData(ctx, env, h, "")
			require.NoError(t, err)
			assert.NotEmpty(t, userData)
		},
		"CreatesHostJasperCredentials": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			_, err := bootstrapUserData(ctx, env, h, "")
			require.NoError(t, err)
			assert.Equal(t, h.JasperCredentialsID, h.Id)

			assert.Equal(t, h.JasperCredentialsID, h.Id)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, h.Id, dbHost.JasperCredentialsID)

			creds, err := h.JasperCredentials(ctx, env)
			require.NoError(t, err)
			assert.NotNil(t, creds)
		},
		"PassesWithCustomUserData": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			customUserData := "#!/bin/bash\necho 'foobar'"
			userData, err := bootstrapUserData(ctx, env, h, customUserData)
			require.NoError(t, err)
			assert.NotEmpty(t, userData)
			assert.True(t, len(userData) > len(customUserData))

			assert.Equal(t, h.JasperCredentialsID, h.Id)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, h.Id, dbHost.JasperCredentialsID)

			creds, err := h.JasperCredentials(ctx, env)
			require.NoError(t, err)
			assert.NotNil(t, creds)
		},
		"ReturnsUserDataUnmodifiedIfNotBootstrapping": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host, userID string) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			customUserData := "foo bar"
			userData, err := bootstrapUserData(ctx, env, h, customUserData)
			require.NoError(t, err)
			assert.Equal(t, customUserData, userData)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			withCredentialsBootstrap(t, func(settings *evergreen.Settings) {
				require.NoError(t, db.ClearCollections(user.Collection))
				defer func() {
					assert.NoError(t, db.ClearCollections(user.Collection))
				}()

				userID := "user"
				user := &user.DBUser{Id: userID}
				require.NoError(t, user.Insert())

				h := &host.Host{Id: "host", Distro: distro.Distro{
					Arch: distro.ArchLinuxAmd64,
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						JasperCredentialsPath: "/bar",
						ClientDir:             "/client_dir",
					},
				},
					StartedBy:        evergreen.User,
					ProvisionOptions: &host.ProvisionOptions{LoadCLI: true, OwnerId: userID},
				}
				require.NoError(t, h.Insert())
				ctx, ccancel := context.WithTimeout(tctx, 5*time.Second)
				defer ccancel()

				testCase(ctx, t, tenv, h, userID)
			})
		})
	}
}

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

func TestBootstrapUserData(t *testing.T) {
	tctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host){
		"ContainsCommandsToSetupHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			userData, err := bootstrapUserData(ctx, env, h, "", false)
			require.NoError(t, err)

			cmd, err := h.CheckUserDataStartedCommand()
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.StartAgentMonitorRequest(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.MarkUserDataDoneCommands()
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)
		},
		"PassesWithoutCustomUserData": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			userData, err := bootstrapUserData(ctx, env, h, "", false)
			require.NoError(t, err)
			assert.NotEmpty(t, userData)
		},
		"PassesWithoutCustomUserDataWithPersistOnWindows": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			h.Distro.Arch = distro.ArchWindowsAmd64
			h.Distro.BootstrapSettings.ServiceUser = "user"
			userData, err := bootstrapUserData(ctx, env, h, "", false)
			require.NoError(t, err)
			assert.NotEmpty(t, userData)
			assert.Contains(t, userData, persistTag)
		},
		"CreatesHostJasperCredentials": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			_, err := bootstrapUserData(ctx, env, h, "", false)
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
		"PassesWithCustomUserData": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			customUserData := "#!/bin/bash\necho 'foobar'"
			userData, err := bootstrapUserData(ctx, env, h, customUserData, false)
			require.NoError(t, err)

			cmd, err := h.StartAgentMonitorRequest(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.MarkUserDataDoneCommands()
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			assert.Equal(t, h.JasperCredentialsID, h.Id)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, h.Id, dbHost.JasperCredentialsID)

			creds, err := h.JasperCredentials(ctx, env)
			require.NoError(t, err)
			assert.NotNil(t, creds)
		},
		"ReturnsUserDataUnmodifiedIfNotBootstrapping": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			customUserData := "foo bar"
			userData, err := bootstrapUserData(ctx, env, h, customUserData, false)
			require.NoError(t, err)
			assert.Equal(t, customUserData, userData)
		},
		"ReturnsCustomUserDataScriptWithPersistOnWindows": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			h.Distro.BootstrapSettings.ServiceUser = "user"
			h.Distro.Arch = distro.ArchWindowsAmd64
			customUserData := "<powershell>echo foo</powershell>"
			userData, err := bootstrapUserData(ctx, env, h, customUserData, false)
			require.NoError(t, err)
			assert.Contains(t, userData, customUserData)
			assert.Contains(t, userData, persistTag)
		},
		"MergesUserDataPartsIntoOne": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			customUserData := "foo bar"
			userData, err := bootstrapUserData(ctx, env, h, customUserData, true)
			require.NoError(t, err)

			cmd, err := h.StartAgentMonitorRequest(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			cmd, err = h.MarkUserDataDoneCommands()
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			assert.Contains(t, userData, customUserData)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, h.Id, dbHost.JasperCredentialsID)

			creds, err := h.JasperCredentials(ctx, env)
			require.NoError(t, err)
			assert.NotNil(t, creds)
		},
		"MergesUserDataPartsIntoOneWithPersistOnWindows": func(ctx context.Context, t *testing.T, env evergreen.Environment, h *host.Host) {
			h.Distro.Arch = distro.ArchWindowsAmd64
			h.Distro.BootstrapSettings.ServiceUser = "user"
			customUserData := "echo foo"
			userData, err := bootstrapUserData(ctx, env, h, customUserData, true)
			require.NoError(t, err)

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, h.Id, dbHost.JasperCredentialsID)

			creds, err := h.JasperCredentials(ctx, env)
			require.NoError(t, err)
			assert.NotNil(t, creds)

			assert.Contains(t, userData, persistTag)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, user.Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(host.Collection, user.Collection))
			}()

			h := &host.Host{
				Id: "host_id",
				Distro: distro.Distro{
					Arch: distro.ArchLinuxAmd64,
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						JasperCredentialsPath: "/bar",
						JasperBinaryDir:       "/jasper_binary_dir",
						ClientDir:             "/client_dir",
						ShellPath:             "/bin/bash",
					},
				},
				StartedBy: evergreen.User,
			}
			require.NoError(t, h.Insert())
			ctx, ccancel := context.WithTimeout(tctx, 5*time.Second)
			defer ccancel()
			env := testutil.NewEnvironment(ctx, t)

			testCase(ctx, t, env, h)
		})
	}
}

func TestMergeUserData(t *testing.T) {
	for testName, testCase := range map[string]struct {
		host            host.Host
		bootstrapScript string
		customScript    string
		expectedScript  string
	}{
		"AppendsCustomUserData": {
			host:            host.Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64}},
			bootstrapScript: "#!/bin/bash\necho foo",
			customScript:    "echo bar",
			expectedScript:  "#!/bin/bash\necho foo\necho bar",
		},
		"AddsCustomScriptBeforeClosingTagForWindowsHostsRunningShellScripts": {
			host:            host.Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}},
			bootstrapScript: "<powershell>\r\necho foo</powershell>",
			customScript:    "echo bar",
			expectedScript:  "<powershell>\r\necho foo\r\necho bar\r\n</powershell>",
		},
		"AppendsForWindowsHostsNotRunningShellScripts": {
			host:            host.Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}},
			bootstrapScript: "#cloud-config\r\nruncmd:\r\n  - echo foo",
			customScript:    "runcmd:\r\n  - echo bar",
			expectedScript:  "#cloud-config\r\nruncmd:\r\n  - echo foo\r\nruncmd:\r\n  - echo bar",
		},
	} {
		t.Run(testName, func(t *testing.T) {
			combinedScript := mergeUserDataParts(&testCase.host, testCase.bootstrapScript, testCase.customScript)
			assert.Equal(t, testCase.expectedScript, combinedScript)
		})
	}
}

func TestEnsureWindowsUserDataScriptPersists(t *testing.T) {
	t.Run("NoopsForNonWindowsHosts", func(t *testing.T) {
		h := &host.Host{
			Distro: distro.Distro{Arch: distro.ArchLinuxAmd64},
		}
		assert.Empty(t, ensureWindowsUserDataScriptPersists(h, ""))
		content := "foo bar"
		assert.Equal(t, content, ensureWindowsUserDataScriptPersists(h, content))
	})
	t.Run("WithWindowsHost", func(t *testing.T) {
		for testName, testCase := range map[string]func(t *testing.T, h *host.Host){
			"AddsPersistTags": func(t *testing.T, h *host.Host) {
				script := "<powershell>echo foo bar</powershell>"
				persistedScript := ensureWindowsUserDataScriptPersists(h, script)
				assert.Contains(t, persistedScript, script)
				assert.Contains(t, persistedScript, persistTag)
			},
			"NoopsIfPersistTagsAlreadyPresent": func(t *testing.T, h *host.Host) {
				script := "<powershell>echo foo bar</powershell>\r\n" + persistTag
				assert.Equal(t, script, ensureWindowsUserDataScriptPersists(h, script))
			},
			"NoopsForEmptyScript": func(t *testing.T, h *host.Host) {
				assert.Empty(t, ensureWindowsUserDataScriptPersists(h, ""))
			},
			"NoopsForNonShellScript": func(t *testing.T, h *host.Host) {
				userData := "#cloud-config\nruncmd:\n  - echo foo"
				assert.Equal(t, userData, ensureWindowsUserDataScriptPersists(h, userData))
			},
		} {
			t.Run(testName, func(t *testing.T) {
				h := &host.Host{
					Distro: distro.Distro{Arch: distro.ArchWindowsAmd64},
				}
				testCase(t, h)
			})
		}
	})
}

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
	"github.com/evergreen-ci/evergreen/cloud/userdata"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeUserData(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host){
		"ContainsCommandsToStartHostProvisioning": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			userData, err := makeUserData(ctx, env.Settings(), h, "", false)
			require.NoError(t, err)

			opts, err := h.GenerateFetchProvisioningScriptUserData(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, opts.Content)
		},
		"PassesWithoutCustomUserData": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			userData, err := makeUserData(ctx, env.Settings(), h, "", false)
			require.NoError(t, err)
			assert.NotEmpty(t, userData)
		},
		"PassesWithoutCustomUserDataWithPersistOnWindows": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Distro.Arch = evergreen.ArchWindowsAmd64
			h.Distro.BootstrapSettings.ServiceUser = "user"
			userData, err := makeUserData(ctx, env.Settings(), h, "", false)
			require.NoError(t, err)
			assert.NotEmpty(t, userData)
			assert.Contains(t, userData, persistTag)
		},
		"PassesWithCustomUserData": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			customUserData := "#!/bin/bash\necho foo"
			userData, err := makeUserData(ctx, env.Settings(), h, customUserData, false)
			require.NoError(t, err)

			cmd, err := h.CurlCommandWithDefaultRetry(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)
		},
		"ReturnsUserDataUnmodifiedIfNotProvisioningWithUserData": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			customUserData := "#!/bin/bash\necho foo"
			userData, err := makeUserData(ctx, env.Settings(), h, customUserData, false)
			require.NoError(t, err)
			assert.Equal(t, customUserData, userData)
		},
		"ReturnsCustomUserDataScriptWithPersistOnWindows": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			h.Distro.BootstrapSettings.ServiceUser = "user"
			h.Distro.Arch = evergreen.ArchWindowsAmd64
			customUserData := "<powershell>\necho foo\n</powershell>"
			userData, err := makeUserData(ctx, env.Settings(), h, customUserData, false)
			require.NoError(t, err)
			assert.Contains(t, userData, customUserData)
			assert.Contains(t, userData, persistTag)
		},
		"MergesUserDataPartsIntoOne": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			customUserData := "#!/bin/bash\necho foo"
			userData, err := makeUserData(ctx, env.Settings(), h, customUserData, true)
			require.NoError(t, err)

			cmd, err := h.CurlCommandWithDefaultRetry(env.Settings())
			require.NoError(t, err)
			assert.Contains(t, userData, cmd)

			custom, err := parseUserData(customUserData)
			require.NoError(t, err)
			assert.Contains(t, userData, custom.Content)
		},
		"MergesUserDataPartsIntoOneWithPersistOnWindows": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Distro.Arch = evergreen.ArchWindowsAmd64
			h.Distro.BootstrapSettings.ServiceUser = "user"
			customUserData := "<powershell>\necho foo\n</powershell>\n<persist>true</persist>"
			userData, err := makeUserData(ctx, env.Settings(), h, customUserData, true)
			require.NoError(t, err)

			assert.Contains(t, userData, persistTag)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, user.Collection, evergreen.CredentialsCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(host.Collection, user.Collection, evergreen.CredentialsCollection))
			}()

			h := &host.Host{
				Id: "host_id",
				Distro: distro.Distro{
					Arch: evergreen.ArchLinuxAmd64,
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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			testCase(ctx, t, env, h)
		})
	}
}

func TestUserDataMerge(t *testing.T) {
	for testName, testCase := range map[string]struct {
		provision   userData
		custom      userData
		expected    userData
		expectError bool
	}{
		"Succeeds": {
			provision: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo foo",
				},
			},
			custom: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo bar",
				},
			},
			expected: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo foo\necho bar",
				},
			},
		},
		"FailsForIncompatibleUserDataDirectives": {
			provision: userData{
				Options: userdata.Options{
					Directive: userdata.CloudConfig,
					Content:   "runcmd:\n - echo foo",
				},
			},
			custom: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo bar",
				},
			},
			expectError: true,
		},
		"FailsForUnspecifiedDirective": {
			provision: userData{
				Options: userdata.Options{
					Content: "echo foo",
				},
			},
			custom: userData{
				Options: userdata.Options{
					Content: "echo bar",
				},
			},
			expectError: true,
		},
		"PersistsIfBaseUserDataPersists": {
			provision: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo foo",
					Persist:   true,
				},
			},
			custom: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo bar",
				},
			},
			expected: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo foo\necho bar",
					Persist:   true,
				},
			},
		},
		"PersistsIfOtherUserDataPersists": {
			provision: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo foo",
				},
			},
			custom: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo bar",
					Persist:   true,
				},
			},
			expected: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo foo\necho bar",
					Persist:   true,
				},
			},
		},
		"FailsIfPersistingForInvalidDirective": {
			provision: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo foo",
					Persist:   true,
				},
			},
			custom: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo bar",
					Persist:   true,
				},
			},
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			merged, err := testCase.provision.merge(&testCase.custom)
			if !testCase.expectError {
				require.NoError(t, err)
				assert.Equal(t, testCase.expected.Directive, merged.Directive)
				assert.Equal(t, testCase.expected.Content, merged.Content)
				assert.Equal(t, testCase.expected.Persist, merged.Persist)
			} else {
				assert.Error(t, err)
				assert.Nil(t, merged)
			}
		})
	}
}

func TestEnsureWindowsUserDataScriptPersists(t *testing.T) {
	t.Run("NoopsForNonWindowsHosts", func(t *testing.T) {
		h := &host.Host{
			Distro: distro.Distro{Arch: evergreen.ArchLinuxAmd64},
		}
		in := &userData{
			Options: userdata.Options{
				Directive: userdata.ShellScript + "/bin/bash",
				Content:   "echo foo",
			},
		}
		out := ensureWindowsUserDataScriptPersists(h, in)
		assert.Equal(t, in.Directive, out.Directive)
		assert.Equal(t, in.Content, out.Content)
		assert.Equal(t, in.Persist, out.Persist)
	})
	t.Run("WithWindowsHost", func(t *testing.T) {
		for testName, testCase := range map[string]func(t *testing.T, h *host.Host){
			"SetsPersistForScripts": func(t *testing.T, h *host.Host) {
				in := &userData{
					Options: userdata.Options{
						Directive: userdata.PowerShellScript,
						Content:   "echo foo",
					},
				}
				out := ensureWindowsUserDataScriptPersists(h, in)
				assert.Equal(t, in.Directive, out.Directive)
				assert.Equal(t, in.Content, out.Content)
				assert.True(t, out.Persist)
			},
			"NoopsIfAlreadyPersisted": func(t *testing.T, h *host.Host) {
				in := &userData{
					Options: userdata.Options{
						Directive: userdata.PowerShellScript,
						Content:   "echo foo",
						Persist:   true,
					},
				}
				out := ensureWindowsUserDataScriptPersists(h, in)
				assert.Equal(t, in.Directive, out.Directive)
				assert.Equal(t, in.Content, out.Content)
				assert.Equal(t, in.Persist, out.Persist)
			},
			"NoopsForUnpersistable": func(t *testing.T, h *host.Host) {
				in := &userData{
					Options: userdata.Options{
						Directive: userdata.CloudConfig,
						Content:   "echo foo",
					},
				}
				out := ensureWindowsUserDataScriptPersists(h, in)
				assert.Equal(t, in.Directive, out.Directive)
				assert.Equal(t, in.Content, out.Content)
				assert.False(t, out.Persist)
			},
			"RemovesPersistForUnpersistable": func(t *testing.T, h *host.Host) {
				in := &userData{
					Options: userdata.Options{
						Directive: userdata.CloudConfig,
						Content:   "echo foo",
						Persist:   true,
					},
				}
				out := ensureWindowsUserDataScriptPersists(h, in)
				assert.Equal(t, in.Directive, out.Directive)
				assert.Equal(t, in.Content, out.Content)
				assert.False(t, out.Persist)
			},
		} {
			t.Run(testName, func(t *testing.T) {
				h := &host.Host{
					Distro: distro.Distro{Arch: evergreen.ArchWindowsAmd64},
				}
				testCase(t, h)
			})
		}
	})
}

func TestParseUserData(t *testing.T) {
	for testName, testCase := range map[string]struct {
		userData         string
		expectError      bool
		expectedUserData userData
	}{
		"Succeeds": {
			userData: "#!/bin/bash\necho foo",
			expectedUserData: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo foo",
				},
			},
		},
		"IgnoresLeadingWhitespace": {
			userData: "\n\n\n#!/bin/bash\necho foo",
			expectedUserData: userData{
				Options: userdata.Options{
					Directive: userdata.ShellScript + "/bin/bash",
					Content:   "echo foo",
				},
			},
		},
		"SucceedsWithDirectiveAndClosingTag": {
			userData: "<powershell>\necho foo\n</powershell>",
			expectedUserData: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo foo",
				},
			},
		},
		"SucceedsWithDirectiveAndClosingTagAndPersist": {
			userData: "<powershell>\necho foo\n</powershell>\n<persist>true</persist>",
			expectedUserData: userData{
				Options: userdata.Options{
					Directive: userdata.PowerShellScript,
					Content:   "echo foo",
					Persist:   true,
				},
			},
		},
		"FailsWithoutMatchingClosingTag": {
			userData:    "<powershell>\necho foo",
			expectError: true,
		},
		"FailsForNoDirective": {
			userData:    "echo foo",
			expectError: true,
		},
		"FailsForPersistedUserDataIfUnpersistable": {
			userData:    "<persist>true</persist>\n#!/bin/bash\necho foo",
			expectError: true,
		},
		"FailsForEmpty": {
			userData:    "",
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			userData, err := parseUserData(testCase.userData)
			if !testCase.expectError {
				require.NoError(t, err)
				assert.Equal(t, testCase.expectedUserData.Directive, userData.Directive)
				assert.Equal(t, testCase.expectedUserData.Content, userData.Content)
				assert.Equal(t, testCase.expectedUserData.Persist, userData.Persist)
			} else {
				assert.Error(t, err)
				assert.Empty(t, userData)
			}
		})
	}
}

func TestExtractDirective(t *testing.T) {
	for testName, testCase := range map[string]struct {
		userData          string
		expectError       bool
		expectedDirective userdata.Directive
		expectedUserData  string
	}{
		"Succeeds": {
			userData:          "#!/bin/bash\necho foo",
			expectedDirective: "#!/bin/bash",
			expectedUserData:  "echo foo",
		},
		"IgnoresLeadingWhitespace": {
			userData:          "\n\n\n#!/bin/bash\necho foo",
			expectedDirective: "#!/bin/bash",
			expectedUserData:  "echo foo",
		},
		"FailsForNoDirective": {
			userData:    "echo foo",
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			directive, userData, err := extractDirective(testCase.userData)
			if !testCase.expectError {
				require.NoError(t, err)
				assert.Equal(t, testCase.expectedDirective, directive)
				assert.Equal(t, testCase.expectedUserData, userData)
			} else {
				assert.Error(t, err)
				assert.Empty(t, directive)
				assert.Empty(t, userData)
			}
		})
	}
}

func TestExtractClosingTag(t *testing.T) {
	for testName, testCase := range map[string]struct {
		userData         string
		closingTag       userdata.ClosingTag
		expectError      bool
		expectedUserData string
	}{
		"Succeeds": {
			userData:         "<powershell>\necho foo\n</powershell>",
			closingTag:       userdata.PowerShellScriptClosingTag,
			expectedUserData: "<powershell>\necho foo",
		},
		"FailsForNonexistentClosingTag": {
			userData:    "#!/bin/bash\necho foo",
			closingTag:  userdata.PowerShellScriptClosingTag,
			expectError: true,
		},
		"FailsForNoDirective": {
			userData:    "echo foo",
			closingTag:  userdata.PowerShellScriptClosingTag,
			expectError: true,
		},
		"FailsForMultipleDirectives": {
			userData:    "<powershell>\necho foo\n</powershell>\n</powershell>",
			closingTag:  userdata.PowerShellScriptClosingTag,
			expectError: true,
		},
		"FailsForUnmatchedClosingTag": {
			userData:    "<powershell>\necho foo\n</powershell>",
			closingTag:  userdata.BatchScriptClosingTag,
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			userData, err := extractClosingTag(testCase.userData, testCase.closingTag)
			if !testCase.expectError {
				require.NoError(t, err)
				assert.Equal(t, testCase.expectedUserData, userData)
			} else {
				assert.Error(t, err)
				assert.Empty(t, userData)
			}
		})
	}
}

func TestExtractPersistTags(t *testing.T) {
	for testName, testCase := range map[string]struct {
		userData         string
		expectFound      bool
		expectedUserData string
	}{
		"FindsAndRemovesAllInstancesOfPersist": {
			userData:         "<persist>true</persist>\n<powershell>echo foo</powershell>\n<persist>true</persist>",
			expectFound:      true,
			expectedUserData: "<powershell>echo foo</powershell>",
		},
		"FindsWithWhitespace": {
			userData:         "<persist>   true\n</persist>\n<powershell>echo foo</powershell>",
			expectFound:      true,
			expectedUserData: "<powershell>echo foo</powershell>",
		},
		"NoopsForNoPersistTags": {
			userData:         "<powershell>echo foo</powershell>",
			expectFound:      false,
			expectedUserData: "<powershell>echo foo</powershell>",
		},
	} {
		t.Run(testName, func(t *testing.T) {
			found, userData, err := extractPersistTags(testCase.userData)
			require.NoError(t, err)
			assert.Equal(t, testCase.expectFound, found)
			assert.Equal(t, testCase.expectedUserData, userData)
		})
	}
}

func TestMakeMultipartUserData(t *testing.T) {
	populatedUserData, err := newUserData(userdata.Options{
		Directive: userdata.ShellScript + "/bin/bash",
		Content:   "echo foo",
	})
	require.NoError(t, err)

	fileOne := "1.txt"
	fileTwo := "2.txt"

	multipart, err := makeMultipartUserData(map[string]*userData{})
	require.NoError(t, err)
	assert.NotEmpty(t, multipart)

	multipart, err = makeMultipartUserData(map[string]*userData{
		fileOne: populatedUserData,
		fileTwo: populatedUserData,
	})
	require.NoError(t, err)
	assert.Contains(t, multipart, fileOne)
	assert.Contains(t, multipart, fileTwo)
	assert.Equal(t, 2, strings.Count(multipart, string(populatedUserData.Directive)))
	assert.Equal(t, 2, strings.Count(multipart, populatedUserData.Content))
}

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

func TestWriteUserDataPart(t *testing.T) {
	t.Run("SucceedsWIthValidUserData", func(t *testing.T) {
		buf := &bytes.Buffer{}
		mimeWriter := multipart.NewWriter(buf)
		boundary := "some_boundary"
		require.NoError(t, mimeWriter.SetBoundary(boundary))

		userData, err := newUserData(userdata.Options{
			Directive: userdata.ShellScript + "/bin/bash",
			Content:   "echo foo",
		})
		require.NoError(t, err)
		require.NoError(t, writeUserDataPart(mimeWriter, userData, "foobar.txt"))

		part := strings.ToLower(buf.String())
		assert.Contains(t, part, "mime-version: 1.0")
		assert.Contains(t, part, "content-type: text/x-shellscript")
		assert.Contains(t, part, "content-disposition: attachment; filename=\"foobar.txt\"")
		assert.Contains(t, part, userData.Directive)
		assert.Contains(t, part, userData.Content)
		assert.Equal(t, 1, strings.Count(part, boundary))
	})
	t.Run("FailsForUnspecifiedUserDataDirective", func(t *testing.T) {
		buf := &bytes.Buffer{}
		mimeWriter := multipart.NewWriter(buf)
		userData := &userData{
			Options: userdata.Options{
				Content: "this user data has no cloud-init directive",
			},
		}
		assert.Error(t, writeUserDataPart(mimeWriter, userData, "foo.txt"))
	})
	t.Run("FailsForNonexistentUserDataDirective", func(t *testing.T) {
		buf := &bytes.Buffer{}
		mimeWriter := multipart.NewWriter(buf)
		userData := &userData{
			Options: userdata.Options{
				Directive: userdata.Directive("foo"),
				Content:   "this user data has no cloud-init directive",
			},
		}
		assert.Error(t, writeUserDataPart(mimeWriter, userData, "foo.txt"))
	})
	t.Run("FailsForEmptyFileName", func(t *testing.T) {
		buf := &bytes.Buffer{}
		mimeWriter := multipart.NewWriter(buf)
		userData, err := newUserData(userdata.Options{
			Directive: userdata.ShellScript + "/bin/bash",
			Content:   "echo foo",
		})
		require.NoError(t, err)
		assert.Error(t, writeUserDataPart(mimeWriter, userData, ""))
	})
}

package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanityCheckRsync(t *testing.T) {
	withMockStdin := func(t *testing.T, input string, operation func()) {
		stdin := os.Stdin
		defer func() {
			os.Stdin = stdin
		}()
		tmpFile, err := ioutil.TempFile("", "mock_stdin.txt")
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, tmpFile.Close())
			assert.NoError(t, os.RemoveAll(tmpFile.Name()))
		}()
		_, err = tmpFile.WriteString(input)
		require.NoError(t, err)
		_, err = tmpFile.Seek(0, 0)
		require.NoError(t, err)
		os.Stdin = tmpFile
		operation()
	}
	withYesResponse := func(t *testing.T, operation func()) {
		withMockStdin(t, "yes\n", func() {
			operation()
		})
	}
	withNoResponse := func(t *testing.T, operation func()) {
		withMockStdin(t, "no\n", func() {
			operation()
		})
	}

	for testName, testCase := range map[string]func(t *testing.T){
		"SucceedsIfUserSaysYes": func(t *testing.T) {
			withYesResponse(t, func() {
				ok, err := sanityCheckRsync("local_dir/", "remote_dir", false)
				require.NoError(t, err)
				assert.True(t, ok)
			})
		},
		"NotOkIfUserSaysNo": func(t *testing.T) {
			withNoResponse(t, func() {
				ok, err := sanityCheckRsync("local_dir/", "remote_dir", false)
				require.NoError(t, err)
				assert.False(t, ok)
			})
		},
		"ErrorsIfNoResponse": func(t *testing.T) {
			ok, err := sanityCheckRsync("local_dir/", "remote_dir", false)
			assert.Error(t, err)
			assert.False(t, ok)
		},
		"SucceedsIfUserSaysYesWithPull": func(t *testing.T) {
			withYesResponse(t, func() {
				ok, err := sanityCheckRsync("local_dir", "remote_dir/", true)
				require.NoError(t, err)
				assert.True(t, ok)
			})
		},
		"NotOkIfUserSaysNoWithPull": func(t *testing.T) {
			withNoResponse(t, func() {
				ok, err := sanityCheckRsync("local_dir", "remote_dir/", true)
				require.NoError(t, err)
				assert.False(t, ok)
			})
		},
		"ErrorsIfNoResponseWithPull": func(t *testing.T) {
			ok, err := sanityCheckRsync("local_dir", "remote_dir/", true)
			assert.Error(t, err)
			assert.False(t, ok)
		},
		"DoesNotCheckIfBothAreTreatedAsFiles": func(t *testing.T) {
			ok, err := sanityCheckRsync("local_file", "remote_file", false)
			require.NoError(t, err)
			assert.True(t, ok)
		},
		"DoesNotCheckIfBothAreTreatedAsFilesWithPull": func(t *testing.T) {
			ok, err := sanityCheckRsync("local_file", "remote_file", true)
			require.NoError(t, err)
			assert.True(t, ok)
		},
	} {
		t.Run(testName, testCase)
	}
}

func TestBuildRsyncCommand(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
		"FailsWithNoOptions": func(ctx context.Context, t *testing.T) {
			_, err := buildRsyncCommand(ctx, rsyncOpts{})
			assert.Error(t, err)
		},
		"SucceedsWithPopulatedOptions": func(ctx context.Context, t *testing.T) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{
				local:  "local",
				remote: "remote",
			})
			require.NoError(t, err)
			require.NotNil(t, cmd)

			assert.Equal(t, os.Stdout, cmd.Stdout)
			assert.Equal(t, os.Stderr, cmd.Stderr)
			rsyncPath, err := exec.LookPath("rsync")
			require.NoError(t, err)
			assert.Equal(t, rsyncPath, cmd.Path)
		},
		"WithDryRun": func(ctx context.Context, t *testing.T) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{
				local:  "local",
				remote: "remote",
				dryRun: true,
			})
			require.NoError(t, err)
			require.NotNil(t, cmd)

			assert.Contains(t, cmd.Args, "-n")
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			testCase(ctx, t)
		})
	}
}

func TestHostRsync(t *testing.T) {
	localFileContent := []byte("foo")
	remoteFileContent := []byte("bar")
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string){
		"FailsForNoOptions": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{})
			assert.Error(t, err)
			assert.Nil(t, cmd)
		},
		"FailsWithoutLocalPath": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{remote: remoteFile})
			assert.Error(t, err)
			assert.Nil(t, cmd)
		},
		"FailsWithoutRemotePath": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localFile})
			assert.Error(t, err)
			assert.Nil(t, cmd)
		},
		"RemoteFileCreatedIfNonexistent": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localFile))
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localFile, remote: newRemoteFile})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			localContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := ioutil.ReadFile(newRemoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, remoteContent)
		},
		"ExistingRemoteFileMirrorsLocalFile": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localFile, remote: remoteFile})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			localContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, remoteContent)
		},
		"LocalFileCreatedIfNonexistent": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			newLocalFile := filepath.Join(remoteDir, filepath.Base(localFile))
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: newLocalFile, remote: remoteFile, pull: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			localContent, err := ioutil.ReadFile(newLocalFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, localContent)

			remoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)
		},
		"ExistingLocalFileMirrorsRemoteFile": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localFile, remote: remoteFile, pull: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			remoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)

			localContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, localContent)
		},
		"NoChangesInDryRun": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localFile, remote: remoteFile, dryRun: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			localContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)
		},
		"NoChangesInDryRunWithPull": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localFile, remote: remoteFile, dryRun: true, pull: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			localContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)
		},
		"RemoteDirectoryMirrorsLocalDirectoryExactly": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir + "/", remote: remoteDir + "/", shouldDelete: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			oldRemoteFileExists, err := util.FileExists(remoteFile)
			assert.NoError(t, err)
			assert.False(t, oldRemoteFileExists)

			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localFile))
			newRemoteContent, err := ioutil.ReadFile(newRemoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, newRemoteContent)
		},
		"RemoteDirectoryMirrorsLocalDirectoryWithoutDeletions": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir + "/", remote: remoteDir + "/"})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			oldRemoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, oldRemoteContent)

			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localFile))
			newRemoteContent, err := ioutil.ReadFile(newRemoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, newRemoteContent)
		},
		"LocalDirectoryMirrorsRemoteDirectoryExactly": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir + "/", remote: remoteDir + "/", shouldDelete: true, pull: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			oldLocalFileExists, err := util.FileExists(localFile)
			assert.NoError(t, err)
			assert.False(t, oldLocalFileExists)

			newLocalFile := filepath.Join(localDir, filepath.Base(remoteFile))
			newLocalContent, err := ioutil.ReadFile(newLocalFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, newLocalContent)
		},
		"LocalDirectoryMirrorsRemoteDirectoryWithoutDeletions": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir + "/", remote: remoteDir + "/", pull: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			oldLocalContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, oldLocalContent)

			newLocalFile := filepath.Join(localDir, filepath.Base(remoteFile))
			newLocalContent, err := ioutil.ReadFile(newLocalFile)
			require.NoError(t, err)
			assert.Equal(t, remoteFileContent, newLocalContent)
		},
		"RemoteDirectoryHasLocalSubdirectoryWithoutTrailingSlash": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir, remote: remoteDir})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			oldRemoteContent, err := ioutil.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, oldRemoteContent)

			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localDir), filepath.Base(localFile))
			newRemoteContent, err := ioutil.ReadFile(newRemoteFile)
			require.NoError(t, err)
			assert.Equal(t, localFileContent, newRemoteContent)
		},
		"LocalDirectoryHasRemoteSubdirectoryWithoutTrailingSlash": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir, remote: remoteDir, pull: true})
			require.NoError(t, err)
			require.NoError(t, cmd.Run())

			oldLocalContent, err := ioutil.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, oldLocalContent)

			newLocalFile := filepath.Join(localDir, filepath.Base(remoteDir), filepath.Base(remoteFile))
			newLocalContent, err := ioutil.ReadFile(newLocalFile)
			require.NoError(t, err)
			assert.Equal(t, remoteFileContent, newLocalContent)
		},
		"ParentDirectoriesWillBeCreatedForRemoteFile": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(ctx, rsyncOpts{local: localDir, remote: remoteDir, makeRemoteParentDirs: true})
			require.NoError(t, err)
			require.NotNil(t, cmd)
			assert.Contains(t, cmd.Args, fmt.Sprintf(`--rsync-path=mkdir -p "%s" && rsync`, filepath.Dir(remoteDir)))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			localDir, err := ioutil.TempDir("", "local_dir")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(localDir))
			}()

			localFile, err := ioutil.TempFile(localDir, "local_file")
			require.NoError(t, err)
			n, err := localFile.Write(localFileContent)
			require.NoError(t, err)
			require.Len(t, localFileContent, n)
			require.NoError(t, localFile.Close())

			remoteDir, err := ioutil.TempDir("", "remote_dir")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(remoteDir))
			}()

			remoteFile, err := ioutil.TempFile(remoteDir, "remote_file")
			require.NoError(t, err)
			n, err = remoteFile.Write(remoteFileContent)
			require.NoError(t, err)
			require.Len(t, remoteFileContent, n)
			require.NoError(t, remoteFile.Close())

			testCase(ctx, t, localFile.Name(), remoteFile.Name(), localDir, remoteDir)
		})
	}
}

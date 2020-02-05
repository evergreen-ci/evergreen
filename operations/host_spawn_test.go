package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostRsync(t *testing.T) {
	localFileContent := []byte("foo")
	remoteFileContent := []byte("bar")
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string){
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

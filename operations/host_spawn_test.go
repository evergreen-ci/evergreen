package operations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostRsync(t *testing.T) {
	// makeCompatiblePath converts filepaths into their rsync-compatible paths.
	// On Windows, this must be the cygwin path since we use cygwin rsync. On
	// other platforms, it is a no-op.
	makeCompatiblePath := func(ctx context.Context, t *testing.T, path string) string {
		if runtime.GOOS != "windows" {
			return path
		}

		// Since windows tests use cygwin, we have to convert them to their
		// cygwin (i.e. Unix) path.
		cygpath, err := exec.LookPath("cygpath")
		require.NoError(t, err)

		output := util.NewMBCappedWriter()

		err = jasper.NewCommand().Add([]string{cygpath, "-u", path}).SetCombinedWriter(output).Run(ctx)
		require.NoError(t, err)
		return strings.TrimSpace(output.String())
	}

	localFileContent := []byte("foo")
	remoteFileContent := []byte("bar")
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string){
		"RemoteFileCreatedIfNonexistent": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localFile))
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localFile),
				remote: makeCompatiblePath(ctx, t, newRemoteFile),
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			localContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := os.ReadFile(newRemoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, remoteContent)
		},
		"ExistingRemoteFileMirrorsLocalFile": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localFile),
				remote: makeCompatiblePath(ctx, t, remoteFile),
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			localContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, remoteContent)
		},
		"LocalFileCreatedIfNonexistent": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			newLocalFile := filepath.Join(remoteDir, filepath.Base(localFile))
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, newLocalFile),
				remote: makeCompatiblePath(ctx, t, remoteFile),
				pull:   true,
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			localContent, err := os.ReadFile(newLocalFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, localContent)

			remoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)
		},
		"ExistingLocalFileMirrorsRemoteFile": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localFile),
				remote: makeCompatiblePath(ctx, t, remoteFile),
				pull:   true,
			},
			)
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			remoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)

			localContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, localContent)
		},
		"NoChangesInDryRun": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localFile),
				remote: makeCompatiblePath(ctx, t, remoteFile),
				dryRun: true,
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			localContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)
		},
		"NoChangesInDryRunWithPull": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localFile),
				remote: makeCompatiblePath(ctx, t, remoteFile),
				dryRun: true,
				pull:   true,
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			localContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, localContent)

			remoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, remoteContent)
		},
		"RemoteDirectoryMirrorsLocalDirectoryExactly": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:        makeCompatiblePath(ctx, t, localDir) + "/",
				remote:       makeCompatiblePath(ctx, t, remoteDir) + "/",
				shouldDelete: true,
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			oldRemoteFileExists := utility.FileExists(remoteFile)
			assert.False(t, oldRemoteFileExists)

			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localFile))
			newRemoteContent, err := os.ReadFile(newRemoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, newRemoteContent)
		},
		"RemoteDirectoryMirrorsLocalDirectoryWithoutDeletions": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localDir) + "/",
				remote: makeCompatiblePath(ctx, t, remoteDir) + "/",
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			oldRemoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, oldRemoteContent)

			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localFile))
			newRemoteContent, err := os.ReadFile(newRemoteFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, newRemoteContent)
		},
		"LocalDirectoryMirrorsRemoteDirectoryExactly": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:        makeCompatiblePath(ctx, t, localDir) + "/",
				remote:       makeCompatiblePath(ctx, t, remoteDir) + "/",
				shouldDelete: true,
				pull:         true,
			})
			require.NoError(t, err)

			require.NoError(t, cmd.Run(ctx))

			oldLocalFileExists := utility.FileExists(localFile)
			assert.False(t, oldLocalFileExists)

			newLocalFile := filepath.Join(localDir, filepath.Base(remoteFile))
			newLocalContent, err := os.ReadFile(newLocalFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, newLocalContent)
		},
		"LocalDirectoryMirrorsRemoteDirectoryWithoutDeletions": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localDir) + "/",
				remote: makeCompatiblePath(ctx, t, remoteDir) + "/",
				pull:   true,
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			oldLocalContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, oldLocalContent)

			newLocalFile := filepath.Join(localDir, filepath.Base(remoteFile))
			newLocalContent, err := os.ReadFile(newLocalFile)
			require.NoError(t, err)
			assert.Equal(t, remoteFileContent, newLocalContent)
		},
		"RemoteDirectoryHasLocalSubdirectoryWithoutTrailingSlash": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localDir),
				remote: makeCompatiblePath(ctx, t, remoteDir),
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			oldRemoteContent, err := os.ReadFile(remoteFile)
			assert.NoError(t, err)
			assert.Equal(t, remoteFileContent, oldRemoteContent)

			newRemoteFile := filepath.Join(remoteDir, filepath.Base(localDir), filepath.Base(localFile))
			newRemoteContent, err := os.ReadFile(newRemoteFile)
			require.NoError(t, err)
			assert.Equal(t, localFileContent, newRemoteContent)
		},
		"LocalDirectoryHasRemoteSubdirectoryWithoutTrailingSlash": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd, err := buildRsyncCommand(rsyncOpts{
				local:  makeCompatiblePath(ctx, t, localDir),
				remote: makeCompatiblePath(ctx, t, remoteDir),
				pull:   true,
			})
			require.NoError(t, err)
			require.NoError(t, cmd.Run(ctx))

			oldLocalContent, err := os.ReadFile(localFile)
			assert.NoError(t, err)
			assert.Equal(t, localFileContent, oldLocalContent)

			newLocalFile := filepath.Join(localDir, filepath.Base(remoteDir), filepath.Base(remoteFile))
			newLocalContent, err := os.ReadFile(newLocalFile)
			require.NoError(t, err)
			assert.Equal(t, remoteFileContent, newLocalContent)
		},
		"ParentDirectoriesWillBeCreatedForRemoteFile": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			cmd,
				err := buildRsyncCommand(rsyncOpts{
				local:                makeCompatiblePath(ctx, t, localDir),
				remote:               makeCompatiblePath(ctx, t, remoteDir),
				makeRemoteParentDirs: true,
			})
			require.NoError(t, err)
			require.NotNil(t, cmd)
			exported, err := cmd.Export()
			require.NoError(t, err)
			require.Len(t, exported, 1)
			baseIndex := strings.LastIndex(makeCompatiblePath(ctx, t, localDir), "/")
			require.True(t, baseIndex > 0)
			assert.Contains(t, exported[0].Args, fmt.Sprintf(`--rsync-path=mkdir -p "%s" && rsync`, makeCompatiblePath(ctx, t, remoteDir)[:baseIndex]))
		},
		"AdditionalParametersAreAdded": func(ctx context.Context, t *testing.T, localFile, remoteFile, localDir, remoteDir string) {
			params := []string{"--filter=:- .gitignore"}
			cmd,
				err := buildRsyncCommand(rsyncOpts{
				local:        makeCompatiblePath(ctx, t, localDir),
				remote:       makeCompatiblePath(ctx, t, remoteDir),
				binaryParams: params,
			})
			require.NoError(t, err)
			require.NotZero(t, cmd)
			exported, err := cmd.Export()
			require.NoError(t, err)
			require.Len(t, exported, 1)
			assert.Subset(t, exported[0].Args, params)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			localDir := t.TempDir()

			localFile, err := os.CreateTemp(localDir, "local_file")
			require.NoError(t, err)
			n, err := localFile.Write(localFileContent)
			require.NoError(t, err)
			require.Len(t, localFileContent, n)
			require.NoError(t, localFile.Close())

			remoteDir := t.TempDir()

			remoteFile, err := os.CreateTemp(remoteDir, "remote_file")
			require.NoError(t, err)
			n, err = remoteFile.Write(remoteFileContent)
			require.NoError(t, err)
			require.Len(t, remoteFileContent, n)
			require.NoError(t, remoteFile.Close())

			testCase(ctx, t, localFile.Name(), remoteFile.Name(), localDir, remoteDir)
		})
	}
}

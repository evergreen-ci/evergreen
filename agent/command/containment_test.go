package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainToWorkdir(t *testing.T) {
	workDir := t.TempDir()
	hostTask := &internal.TaskConfig{WorkDir: workDir}

	t.Run("UnrestrictedWithoutIsolation", func(t *testing.T) {
		outside := filepath.Join(filepath.Dir(workDir), "outside")
		assert.NoError(t, containToWorkdir(hostTask, "path", outside))
	})

	t.Run("EscapingPathRejected", func(t *testing.T) {
		conf := isolatedConf(t, workDir)
		outside := filepath.Join(filepath.Dir(workDir), "outside")
		assert.Error(t, containToWorkdir(conf, "path", outside))
		assert.NoError(t, containToWorkdir(conf, "path", filepath.Join(workDir, "inside")))
	})
}

// isolatedConf returns a TaskConfig with container isolation enabled and the
// given work directory.
func isolatedConf(t *testing.T, workDir string) *internal.TaskConfig {
	t.Helper()
	return &internal.TaskConfig{
		WorkDir: workDir,
		Distro: &apimodels.DistroView{
			ContainerIsolation: &apimodels.ContainerIsolationSettings{
				Image: "ubuntu:22.04",
			},
		},
	}
}

func TestBuildArchiveIsolatedSkipsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not supported on Windows")
	}
	ctx := t.Context()
	logger := logging.NewGrip("test.archive.containment")

	srcDir := t.TempDir()
	outsideDir := t.TempDir()
	hostOnly := filepath.Join(outsideDir, "host-only.txt")
	require.NoError(t, os.WriteFile(hostOnly, []byte("HOST-ONLY-SECRET"), 0600))
	require.NoError(t, os.Symlink(hostOnly, filepath.Join(srcDir, "host-leak.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "regular.txt"), []byte("regular"), 0644))

	contents, _, err := findArchiveContents(ctx, srcDir, []string{"**"}, []string{})
	require.NoError(t, err)

	root, err := os.OpenRoot(srcDir)
	require.NoError(t, err)
	defer root.Close()

	target := filepath.Join(t.TempDir(), "leak.tgz")
	f, gz, tarWriter, err := tarGzWriter(target, false)
	require.NoError(t, err)
	_, err = buildArchive(ctx, buildArchiveOptions{
		tarWriter: tarWriter,
		rootPath:  srcDir,
		paths:     contents,
		logger:    logger,
		root:      root,
	})
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	headers := collectTarHeaders(t, target)
	assert.NotContains(t, headers, "host-leak.txt", "escaping symlink must not be packed")
	require.Contains(t, headers, "regular.txt")
}

func TestExtractTarballIsolatedRejectsDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not supported on Windows")
	}
	ctx := t.Context()

	// Build an archive with a single regular entry pivot/overwrite.txt.
	payloadDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(payloadDir, "pivot"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(payloadDir, "pivot", "overwrite.txt"), []byte("EXTRACTION-WRITE"), 0644))
	contents, _, err := findArchiveContents(ctx, payloadDir, []string{"**"}, []string{})
	require.NoError(t, err)
	archivePath := filepath.Join(t.TempDir(), "payload.tgz")
	f, gz, tarWriter, err := tarGzWriter(archivePath, false)
	require.NoError(t, err)
	_, err = buildArchive(ctx, buildArchiveOptions{
		tarWriter: tarWriter,
		rootPath:  payloadDir,
		paths:     contents,
		logger:    logging.NewGrip("test.archive.containment"),
	})
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	// Plant a symlink pivot in the destination that escapes the work
	// directory, pointing at a sentinel outside it.
	destDir := t.TempDir()
	outsideDir := t.TempDir()
	sentinel := filepath.Join(outsideDir, "overwrite.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("ORIGINAL"), 0644))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(destDir, "pivot")))

	archive, err := os.Open(archivePath)
	require.NoError(t, err)
	defer archive.Close()
	root, err := os.OpenRoot(destDir)
	require.NoError(t, err)
	defer root.Close()

	err = extractTarball(ctx, archive, destDir, nil, false, root)
	require.Error(t, err, "extraction through a planted destination symlink must fail")

	written, readErr := os.ReadFile(sentinel)
	require.NoError(t, readErr)
	assert.Equal(t, "ORIGINAL", string(written), "sentinel outside the work directory must be unchanged")
}

// TestExtractTarballIsolatedAllowsInternalSymlinks ensures the bounded
// extraction still restores trees containing in-root symlinks.
func TestExtractTarballIsolatedAllowsInternalSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not supported on Windows")
	}
	ctx := t.Context()

	destDir := t.TempDir()
	// A tarball with a directory, a regular file inside it, and a symlink to
	// the file; the symlink's raw target stays inside the root.
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	writeEntry := func(hdr *tar.Header, body []byte) {
		require.NoError(t, tw.WriteHeader(hdr))
		if body != nil {
			_, err := tw.Write(body)
			require.NoError(t, err)
		}
	}
	writeEntry(&tar.Header{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755}, nil)
	writeEntry(&tar.Header{Name: "dir/real.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 5}, []byte("real!"))
	writeEntry(&tar.Header{Name: "dir/link.txt", Typeflag: tar.TypeSymlink, Linkname: "real.txt", Mode: 0o644}, nil)
	require.NoError(t, tw.Close())
	require.NoError(t, gzw.Close())

	root, err := os.OpenRoot(destDir)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, extractTarball(ctx, &buf, destDir, nil, true, root))

	link := filepath.Join(destDir, "dir", "link.txt")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "real.txt", target)
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, "real!", string(data))
}

// TestTarballCreateExecuteIsolated replays the SECBUG-5643 archive-creation
// PoC through the command: a task-planted symlink to a host-only file must
// not be packed.
func TestTarballCreateExecuteIsolated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not supported on Windows")
	}
	ctx := t.Context()

	workDir := t.TempDir()
	hostOnly := filepath.Join(t.TempDir(), "host-only.txt")
	require.NoError(t, os.WriteFile(hostOnly, []byte("F3-HOST-ONLY-MARKER"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, "artifacts"), 0755))
	require.NoError(t, os.Symlink(hostOnly, filepath.Join(workDir, "artifacts", "host-leak.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "artifacts", "real.txt"), []byte("real"), 0644))

	conf := isolatedConf(t, workDir)
	comm := client.NewMock("url")
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	cmd := &tarballCreate{
		Target:    filepath.Join(workDir, "artifacts.tgz"),
		SourceDir: filepath.Join(workDir, "artifacts"),
		Include:   []string{"**"},
	}
	require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

	headers := collectTarHeaders(t, cmd.Target)
	assert.NotContains(t, headers, "host-leak.txt")
	require.Contains(t, headers, "real.txt")
}

// TestTarballExtractExecuteIsolated replays the SECBUG-5643 extraction PoC
// through the command: extraction through a planted destination symlink must
// fail without touching the host target.
func TestTarballExtractExecuteIsolated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not supported on Windows")
	}
	ctx := t.Context()

	workDir := t.TempDir()
	conf := isolatedConf(t, workDir)
	comm := client.NewMock("url")
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	// Build the payload archive inside the work directory.
	payloadDir := filepath.Join(workDir, "payload-src")
	require.NoError(t, os.MkdirAll(filepath.Join(payloadDir, "pivot"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(payloadDir, "pivot", "overwrite.txt"), []byte("F3-EXTRACTION-WRITE"), 0644))
	createCmd := &tarballCreate{
		Target:    filepath.Join(workDir, "payload.tgz"),
		SourceDir: payloadDir,
		Include:   []string{"**"},
	}
	require.NoError(t, createCmd.Execute(ctx, comm, logger, conf))

	// Plant the pivot symlink pointing outside the work directory.
	outsideDir := t.TempDir()
	sentinel := filepath.Join(outsideDir, "overwrite.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("ORIGINAL"), 0644))
	restoreDir := filepath.Join(workDir, "restore")
	require.NoError(t, os.MkdirAll(restoreDir, 0755))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(restoreDir, "pivot")))

	extractCmd := &tarballExtract{
		ArchivePath:     filepath.Join(workDir, "payload.tgz"),
		TargetDirectory: restoreDir,
	}
	require.Error(t, extractCmd.Execute(ctx, comm, logger, conf))

	written, readErr := os.ReadFile(sentinel)
	require.NoError(t, readErr)
	assert.Equal(t, "ORIGINAL", string(written), "sentinel outside the work directory must be unchanged")
}

// TestTarballExtractExecuteIsolatedRejectsEscapingArchive ensures the archive
// file itself cannot be a symlink escaping the work directory.
func TestTarballExtractExecuteIsolatedRejectsEscapingArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not supported on Windows")
	}
	ctx := t.Context()

	workDir := t.TempDir()
	conf := isolatedConf(t, workDir)
	comm := client.NewMock("url")
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	outsideArchive := filepath.Join(t.TempDir(), "outside.tgz")
	require.NoError(t, os.WriteFile(outsideArchive, []byte("not a real archive"), 0644))
	require.NoError(t, os.Symlink(outsideArchive, filepath.Join(workDir, "link.tgz")))

	extractCmd := &tarballExtract{
		ArchivePath:     filepath.Join(workDir, "link.tgz"),
		TargetDirectory: filepath.Join(workDir, "restore"),
	}
	require.Error(t, extractCmd.Execute(ctx, comm, logger, conf))
}

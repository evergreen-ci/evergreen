package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestCreateEnclosingDirectory(t *testing.T) {
	assert := assert.New(t)

	// create a temp directory and ensure that its cleaned up.
	dirname := t.TempDir()

	// write data to a temp file and then ensure that the directory existing predicate is valid
	fileName := filepath.Join(dirname, "foo")
	assert.False(dirExists(fileName))
	assert.NoError(os.WriteFile(fileName, []byte("hello world"), 0744))
	assert.False(dirExists(fileName))
	_, err := os.Stat(fileName)
	assert.False(os.IsNotExist(err))
	assert.NoError(os.Remove(fileName))
	_, err = os.Stat(fileName)
	assert.True(os.IsNotExist(err))

	// ensure that we create an enclosing directory if needed
	assert.False(dirExists(fileName))
	fileName = filepath.Join(fileName, "bar")
	assert.NoError(createEnclosingDirectoryIfNeeded(fileName))
	assert.True(dirExists(filepath.Join(dirname, "foo")))
}

func TestGetJoinedWithWorkDir(t *testing.T) {
	relativeDir := "bar"
	absoluteDir, err := filepath.Abs("/bar")
	require.NoError(t, err)
	conf := &internal.TaskConfig{
		WorkDir: "/foo",
	}
	expected, err := filepath.Abs("/foo/bar")
	require.NoError(t, err)
	expected = filepath.ToSlash(expected)
	actual, err := filepath.Abs(GetWorkingDirectory(conf, relativeDir))
	require.NoError(t, err)
	actual = filepath.ToSlash(actual)
	assert.Equal(t, expected, actual)

	expected, err = filepath.Abs("/bar")
	require.NoError(t, err)
	expected = filepath.ToSlash(expected)
	assert.Equal(t, expected, filepath.ToSlash(GetWorkingDirectory(conf, absoluteDir)))
}

func TestGetWorkingDirectoryLegacy(t *testing.T) {
	curdir := testutil.GetDirectoryOfFile()

	conf := &internal.TaskConfig{
		WorkDir: curdir,
	}

	// make sure that we fall back to the configured working directory
	out, err := getWorkingDirectoryLegacy(conf, "")
	assert.NoError(t, err)
	assert.Equal(t, conf.WorkDir, out)

	// check for a directory that we know exists
	out, err = getWorkingDirectoryLegacy(conf, "testdata")
	require.NoError(t, err)
	assert.Equal(t, out, filepath.Join(curdir, "testdata"))

	// check for a file not a directory
	out, err = getWorkingDirectoryLegacy(conf, "exec.go")
	assert.Error(t, err)
	assert.Equal(t, "", out)

	// presumably for a directory that doesn't exist
	out, err = getWorkingDirectoryLegacy(conf, "does-not-exist")
	assert.Error(t, err)
	assert.Equal(t, "", out)
}

func TestRelativePathUnderWorkdirIsNotViolation(t *testing.T) {
	conf := &internal.TaskConfig{WorkDir: filepath.Join(t.TempDir(), "work")}
	assert.False(t, IsWorkdirBoundaryViolation(conf, "src/foo"))
}

func TestAbsolutePathUnderWorkdirIsNotViolation(t *testing.T) {
	conf := &internal.TaskConfig{WorkDir: filepath.Join(t.TempDir(), "work")}
	assert.False(t, IsWorkdirBoundaryViolation(conf, filepath.Join(conf.WorkDir, "src", "foo")))
}

func TestAbsolutePathOutsideWorkdirIsViolation(t *testing.T) {
	root := t.TempDir()
	conf := &internal.TaskConfig{WorkDir: filepath.Join(root, "work")}
	assert.True(t, IsWorkdirBoundaryViolation(conf, filepath.Join(root, "outside")))
}

func TestEmptyPathIsNotViolation(t *testing.T) {
	conf := &internal.TaskConfig{WorkDir: "/data/mci/work"}
	assert.False(t, IsWorkdirBoundaryViolation(conf, ""))
}

func TestPathEqualToWorkdirIsNotViolation(t *testing.T) {
	conf := &internal.TaskConfig{WorkDir: filepath.Join(t.TempDir(), "work")}
	assert.False(t, IsWorkdirBoundaryViolation(conf, conf.WorkDir))
}

func TestSiblingPrefixPathIsViolation(t *testing.T) {
	// work-other starts with work, so a naive
	// strings.HasPrefix check would miss this. filepath.Rel correctly
	// identifies it as outside the workdir.
	root := t.TempDir()
	conf := &internal.TaskConfig{WorkDir: filepath.Join(root, "work")}
	assert.True(t, IsWorkdirBoundaryViolation(conf, filepath.Join(root, "work-other")))
}

func TestRelativeTraversalOutsideWorkdirIsViolation(t *testing.T) {
	// ../../etc/passwd joined to /data/mci/work resolves to /data/etc/passwd,
	// which is outside the workdir.
	conf := &internal.TaskConfig{WorkDir: filepath.Join(t.TempDir(), "data", "mci", "work")}
	assert.True(t, IsWorkdirBoundaryViolation(conf, "../../etc/passwd"))
}

func TestTrailingSlashNormalization(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "work")
	inside := filepath.Join(workdir, "foo")
	outside := filepath.Join(root, "work-other")

	// Workdir with trailing slash, path without — should not be a violation.
	conf := &internal.TaskConfig{WorkDir: workdir + string(filepath.Separator)}
	assert.False(t, IsWorkdirBoundaryViolation(conf, inside))

	// Workdir without trailing slash, path with trailing slash — should not
	// be a violation.
	conf.WorkDir = workdir
	assert.False(t, IsWorkdirBoundaryViolation(conf, inside+string(filepath.Separator)))

	// Workdir with trailing slash, sibling-prefix path — should be a
	// violation.
	conf.WorkDir = workdir + string(filepath.Separator)
	assert.True(t, IsWorkdirBoundaryViolation(conf, outside))
}

func TestCrossVolumePathIsViolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		conf := &internal.TaskConfig{WorkDir: `C:\work`}
		assert.True(t, IsWorkdirBoundaryViolation(conf, `D:\outside`))
		return
	}

	conf := &internal.TaskConfig{WorkDir: "relative/workdir"}
	assert.True(t, IsWorkdirBoundaryViolation(conf, "/outside"))
}

func TestSetWorkdirBoundaryAttributeSetsTrueOnViolation(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, tp.Shutdown(ctx))
	})

	ctx, span := tp.Tracer("test").Start(t.Context(), "test")
	root := t.TempDir()
	conf := &internal.TaskConfig{WorkDir: filepath.Join(root, "work")}
	SetWorkdirBoundaryAttribute(ctx, conf, filepath.Join(root, "work-other"))
	span.End()

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)

	found := false
	for _, attr := range ended[0].Attributes() {
		if string(attr.Key) == workdirBoundaryViolationAttribute {
			assert.True(t, attr.Value.AsBool(), "workdir boundary violation attribute should be true")
			found = true
		}
	}
	assert.True(t, found, "workdir boundary violation attribute was not set on the span")
}

func TestSetWorkdirBoundaryAttributeSetsFalseWithoutViolation(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, tp.Shutdown(ctx))
	})

	ctx, span := tp.Tracer("test").Start(t.Context(), "test")
	conf := &internal.TaskConfig{WorkDir: filepath.Join(t.TempDir(), "work")}
	SetWorkdirBoundaryAttribute(ctx, conf, filepath.Join(conf.WorkDir, "src", "foo"))
	span.End()

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)

	found := false
	for _, attr := range ended[0].Attributes() {
		if string(attr.Key) == workdirBoundaryViolationAttribute {
			assert.False(t, attr.Value.AsBool(), "workdir boundary violation attribute should be false")
			found = true
		}
	}
	assert.True(t, found, "workdir boundary violation attribute was not set on the span")
}

func TestSetWorkdirBoundaryAttributePreservesViolationAcrossPaths(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, tp.Shutdown(ctx))
	})

	ctx, span := tp.Tracer("test").Start(t.Context(), "test")
	root := t.TempDir()
	conf := &internal.TaskConfig{WorkDir: filepath.Join(root, "work")}
	SetWorkdirBoundaryAttribute(ctx, conf, filepath.Join(root, "work-other"), filepath.Join(conf.WorkDir, "src", "foo"))
	span.End()

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)

	found := false
	for _, attr := range ended[0].Attributes() {
		if string(attr.Key) == workdirBoundaryViolationAttribute {
			assert.True(t, attr.Value.AsBool(), "workdir boundary violation attribute should remain true")
			found = true
		}
	}
	assert.True(t, found, "workdir boundary violation attribute was not set on the span")
}

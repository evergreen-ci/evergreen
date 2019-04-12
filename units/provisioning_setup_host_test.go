package units

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mholt/archiver"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeFileServer creates a local http server that returns files from
// the given directory.
func makeFileServer(t *testing.T, dir string) *httptest.Server {
	handler := http.FileServer(http.Dir(dir))
	return httptest.NewServer(handler)
}

func TestFetchJasper(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection))
	}()

	// Set up a file system server that can serve the Jasper file to download
	// and extract.
	tmpDir, err := ioutil.TempDir("", "fetch_jasper_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	jasperDownloadFileNameBase := "jasper_dist"
	jasperBinaryName := "jasper_cli"
	jasperVersion := "jasper_version"

	downloadFileName := fmt.Sprintf("%s-%s-%s-%s.tar.gz", jasperDownloadFileNameBase, runtime.GOOS, runtime.GOARCH, jasperVersion)

	binaryFileContents := []byte("foobar")
	tmpBinaryFile, err := os.Create(filepath.Join(tmpDir, jasperBinaryName))
	require.NoError(t, err)
	n, err := tmpBinaryFile.Write(binaryFileContents)
	require.NoError(t, err)
	require.Len(t, binaryFileContents, n)
	require.NoError(t, tmpBinaryFile.Close())

	tmpArchiveFile, err := os.Create(filepath.Join(tmpDir, downloadFileName))
	require.NoError(t, err)
	require.NoError(t, tmpArchiveFile.Close())
	require.NoError(t, archiver.TarGz.Make(tmpArchiveFile.Name(), []string{tmpBinaryFile.Name()}))

	server := makeFileServer(t, tmpDir)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	currentUser, err := user.Current()
	require.NoError(t, err)

	h := &host.Host{
		Id:   "fetch_jasper_test",
		Host: "localhost",
		User: currentUser.Username,
		Distro: distro.Distro{
			Arch: runtime.GOOS + "_" + runtime.GOARCH,
		},
		Provider: evergreen.ProviderNameMock,
	}

	require.NoError(t, distro.ValidateArch(h.Distro.Arch))
	require.NoError(t, h.Insert())

	manager, err := jasper.NewLocalManager(false)
	require.NoError(t, err)
	env := &mock.Environment{
		EvergreenSettings: &evergreen.Settings{
			JasperConfig: evergreen.JasperConfig{
				BinaryName:       jasperBinaryName,
				DownloadFileName: jasperDownloadFileNameBase,
				URL:              server.URL,
				Version:          jasperVersion,
			},
		},
		JasperProcessManager: manager,
	}
	job := NewHostSetupJob(env, *h, "fetch_jasper_test")
	setupJob, ok := job.(*setupHostJob)
	require.True(t, ok)

	outDir, err := ioutil.TempDir("", "out")
	require.NoError(t, err)
	defer os.RemoveAll(outDir)

	sshOptions := []string{}
	require.NoError(t, setupJob.doFetchJasper(ctx, outDir, sshOptions))

	fileInfo, err := os.Stat(filepath.Join(outDir, env.EvergreenSettings.JasperConfig.BinaryName))
	require.NoError(t, err)
	assert.Equal(t, len(binaryFileContents), int(fileInfo.Size()))
	if runtime.GOOS != "windows" {
		assert.NotZero(t, fileInfo.Mode()&0100)
	}

	file, err := os.Open(filepath.Join(outDir, env.EvergreenSettings.JasperConfig.BinaryName))
	require.NoError(t, err)
	fileContents, err := ioutil.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, binaryFileContents, fileContents)
}

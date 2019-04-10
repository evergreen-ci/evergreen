package units

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mholt/archiver"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeTestFileServer creates a local http test server that returns files from
// the given directory.
func makeTestFileServer(t *testing.T, dir string) *httptest.Server {
	handler := http.FileServer(http.Dir(dir))
	return httptest.NewServer(handler)
}

func TestFetchJasper(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection))
	}()

	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tmpFile, err := ioutil.TempFile(tmpDir, "")
	require.NoError(t, err)
	tmpFile.Close()
	require.NoError(t, archiver.TarGz.Make(tmpDir, []string{tmpFile.Name()}))

	server := makeTestFileServer(t, tmpDir)
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

	instance := cloud.MockInstance{
		IsUp:           true,
		IsSSHReachable: true,
	}
	provider := cloud.GetMockProvider()
	provider.Set(h.Id, instance)

	require.NoError(t, distro.ValidateArch(h.Distro.Arch))
	require.NoError(t, h.Insert())

	manager, err := jasper.NewLocalManager(false)
	require.NoError(t, err)
	env := &mock.Environment{
		EvergreenSettings: &evergreen.Settings{
			JasperURL:     server.URL,
			JasperVersion: tmpFile.Name(),
		},
		JasperProcessManager: manager,
	}
	job := NewHostSetupJob(env, *h, "fetch_jasper_test")
	setupJob, ok := job.(*setupHostJob)
	require.True(t, ok)

	sshOptions := []string{}
	require.NoError(t, setupJob.doFetchJasper(ctx, sshOptions))
	defer os.Remove(filepath.Join(currentUser.HomeDir, h.Distro.JasperBinaryName()))
	defer os.Remove(filepath.Join(currentUser.HomeDir, h.Distro.JasperFileName(env.Settings().JasperVersion)))

	_, err = os.Stat(filepath.Join("~", h.Distro.JasperFileName(env.Settings().JasperVersion)))
	require.Equal(t, os.IsNotExist, err)

	info, err := os.Stat(filepath.Join("~", h.Distro.JasperBinaryName()))
	require.NoError(t, err)
	assert.NotZero(t, info.Size())
	if runtime.GOOS != "windows" {
		assert.NotZero(t, info.Mode()|0100)
	}
}

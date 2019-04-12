package operations

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mholt/archiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeFileServer makes a file server for the given directory. Callers are
// responsible for deleting the directory.
func makeFileServer(t *testing.T, dir string) *httptest.Server {
	handler := http.FileServer(http.Dir(dir))
	return httptest.NewServer(handler)
}

// tempDirAndFile creates a temporary directory and temporary files within it
// with the given names.  Callers are responsible for deleting the directory.
func tempDirAndFiles(t *testing.T, fileNames ...string) (string, []string) {
	tempDir, err := ioutil.TempDir("", "evergreen")
	require.NoError(t, err)

	files := make([]string, 0, len(fileNames))
	for _, fileName := range fileNames {
		tempFile, err := os.Create(filepath.Join(tempDir, fileName))
		require.NoError(t, err)
		files = append(files, tempFile.Name())
		require.NoError(t, tempFile.Close())
	}

	return tempDir, files
}

func TestDownload(t *testing.T) {
	tempDir, tempFiles := tempDirAndFiles(t, "download")
	tempFile := tempFiles[0]
	defer os.RemoveAll(tempDir)
	assert.NoError(t, download("http://www.example.com", tempFile))

	fileInfo, err := os.Stat(tempFile)
	require.NoError(t, err)
	assert.NotZero(t, fileInfo.Size())
}

func TestDownloadBadURL(t *testing.T) {
	tempDir, tempFiles := tempDirAndFiles(t, "download_bad_url")
	tempFile := tempFiles[0]
	defer os.RemoveAll(tempDir)

	assert.Error(t, download("not a valid URL", tempFile))

	fileInfo, err := os.Stat(tempFile)
	require.NoError(t, err)
	assert.Zero(t, fileInfo.Size())
}

func TestDownloadAndExtract(t *testing.T) {
	archivedFileName := "download_and_extract_me.txt"
	archiveFileName := "archive.tar.gz"
	tempDir, tempFiles := tempDirAndFiles(t, archivedFileName, archiveFileName)
	defer os.RemoveAll(tempDir)

	archivedFile, err := os.OpenFile(tempFiles[0], os.O_WRONLY, 0222)
	require.NoError(t, err)
	archivedFileContents := "foobar"
	archivedFile.Write([]byte(archivedFileContents))
	archivedFile.Close()

	require.NoError(t, archiver.TarGz.Make(tempFiles[1], []string{tempFiles[0]}))

	extractDir, _ := tempDirAndFiles(t)
	defer os.RemoveAll(extractDir)

	server := makeFileServer(t, tempDir)
	defer server.Close()

	require.NoError(t, downloadAndExtract(fmt.Sprintf("%s/%s", server.URL, archiveFileName), extractDir))

	extractedFilePath := filepath.Join(extractDir, archivedFileName)
	extractedFileContents, err := ioutil.ReadFile(extractedFilePath)
	require.NoError(t, err)
	assert.Equal(t, archivedFileContents, string(extractedFileContents))
}

func TestDownloadAndExtractWithoutArchive(t *testing.T) {
	tempDir, tempFiles := tempDirAndFiles(t, "test")
	defer os.RemoveAll(tempDir)

	destDir, _ := tempDirAndFiles(t)
	defer os.RemoveAll(destDir)

	server := makeFileServer(t, tempDir)
	defer server.Close()

	assert.Error(t, downloadAndExtract(fmt.Sprintf("%s/%s", server.URL, tempFiles[0]), destDir))
}

func TestValidateAndParseMode(t *testing.T) {
	for octal := uint64(0); octal < maxModeValue+1; octal++ {
		parsedVal, err := validateAndParseMode(fmt.Sprintf("%03o", octal))
		require.NoError(t, err)
		assert.Equal(t, octal, parsedVal)
	}
	octal := maxModeValue + 1
	_, err := validateAndParseMode(fmt.Sprintf("%03o", octal))
	assert.Error(t, err)

	octal = -1
	_, err = validateAndParseMode(fmt.Sprintf("%03o", octal))
	assert.Error(t, err)

	octal = 0
	parsedVal, err := validateAndParseMode(fmt.Sprintf("%03o", octal))
	assert.NoError(t, err)
	assert.Equal(t, uint64(octal), parsedVal)
}

func TestSetFileModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot test chmod on Windows")
	}
	tempDir, tempFiles := tempDirAndFiles(t, "test1", "test2")
	defer os.RemoveAll(tempDir)
	for _, tempFile := range tempFiles {
		info, err := os.Stat(tempFile)
		require.NoError(t, err)
		require.NotEqual(t, os.FileMode(0777), info.Mode().Perm())
	}

	require.NoError(t, setFileModes(tempFiles, 0777))
	for _, tempFile := range tempFiles {
		info, err := os.Stat(tempFile)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0777), info.Mode().Perm())
	}
}

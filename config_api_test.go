package evergreen

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/evergreen-ci/pail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopulateClientBinaries(t *testing.T) {
	tmpDir := t.TempDir()
	prefix := "evergreen/clients/evergreen/abcdef"
	platforms := []string{
		ArchDarwinAmd64,
		ArchDarwinArm64,
		ArchLinuxPpc64le,
		ArchLinuxS390x,
		ArchLinuxArm64,
		ArchLinuxAmd64,
		ArchWindowsAmd64,
	}
	for _, platform := range platforms {
		basePath := path.Join(tmpDir, prefix, platform)
		require.NoError(t, os.MkdirAll(basePath, os.ModeDir|os.ModePerm))
		executable := "evergreen"
		if strings.Contains(platform, "windows") {
			executable += ".exe"
		}
		_, err := os.Create(path.Join(basePath, executable))
		require.NoError(t, err)
	}

	bucket, err := pail.NewLocalBucket(pail.LocalOptions{
		Path: tmpDir,
	})
	require.NoError(t, err)

	c := ClientConfig{
		S3URLPrefix: "https://www.foo.com",
	}

	assert.NoError(t, c.populateClientBinaries(context.Background(), bucket, prefix+"/"))
	assert.Len(t, c.ClientBinaries, len(platforms))
	for _, binary := range c.ClientBinaries {
		assert.NotEmpty(t, binary.URL)
		assert.NotEmpty(t, binary.DisplayName)
		assert.Equal(t, ValidArchDisplayNames[fmt.Sprintf("%s_%s", binary.OS, binary.Arch)], binary.DisplayName)
	}
}

package evergreen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/require"
)

func TestKim(t *testing.T) {
	const dir = "archive-input"
	require.NoError(t, os.MkdirAll(dir, 0755))
	const numFiles = 10
	for i := 0; i < numFiles; i++ {
		// 1 MB string
		content := utility.MakeRandomString(1024 * 1024)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("file-%d", i)), []byte(content), 0755))
	}
}

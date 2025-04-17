package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
)

// ConsistentFilepath returns a filepath that always uses forward slashes ('/')
// rather than platform-dependent file path separators.
func ConsistentFilepath(parts ...string) string {
	return strings.Replace(filepath.Join(parts...), "\\", "/", -1)
}

func GetUserHome() (string, error) {
	userHome, err := homedir.Dir()
	if err != nil {
		// workaround for cygwin if we're on windows but couldn't get a homedir
		if runtime.GOOS == "windows" && len(os.Getenv("HOME")) > 0 {
			userHome = os.Getenv("HOME")
		}
	}
	return userHome, err
}

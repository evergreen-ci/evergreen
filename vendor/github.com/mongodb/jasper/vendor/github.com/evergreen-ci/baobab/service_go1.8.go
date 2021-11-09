//+build go1.8

package baobab

import (
	"os"
	"path/filepath"
)

func (c *Config) execPath() (string, error) {
	if len(c.Executable) != 0 {
		return filepath.Abs(c.Executable)
	}
	return os.Executable()
}

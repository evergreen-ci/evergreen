// +build linux solaris

package subprocess

import (
	"io"
	"os"
	"strconv"
)

// listProc() returns a list of active pids on the system, by listing the contents of /proc
// and looking for entries that appear to be valid pids. Only usable on systems with a /proc
// filesystem (Solaris and UNIX/Linux)
func listProc() ([]int, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make([]int, 0, 50)
	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, fi := range fis {
			// Pid must be a directory with a numeric name
			if !fi.IsDir() {
				continue
			}

			// Using Atoi here will also filter out . and ..
			pid, err := strconv.Atoi(fi.Name())
			if err != nil {
				continue
			}
			results = append(results, pid)
		}
	}
	return results, nil
}

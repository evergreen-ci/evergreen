package shell

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mongodb/grip/slogger"
)

func trackProcess(key string, pid int, log plugin.Logger) {
	// trackProcess is a noop on linux, because we detect all the processes to be killed in
	// cleanup() and we don't need to do any special bookkeeping up-front.
}

// getEnv returns a slice of environment variables for the given pid, in the form
// []string{"VAR1=FOO", "VAR2=BAR", ...}
// This function works by reading from /proc/$PID/environ, so the values returned only reflect
// the values of the environment variables at the time that the process was started.
func getEnv(pid int) ([]string, error) {
	env, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		// This is probably either "permission denied" because we do not own the process,
		// or the process simply doesn't exist anymore.
		return nil, err
	}
	parts := bytes.Split(env, []byte{0})
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		results = append(results, string(part))
	}
	return results, nil
}

func cleanup(key string, log plugin.Logger) error {
	pids, err := listProc()
	if err != nil {
		return err
	}

	for _, pid := range pids {
		env, err := getEnv(pid)
		if err != nil {
			continue
		}
		if envHasMarkers(env) {
			p := os.Process{}
			p.Pid = pid
			if err := p.Kill(); err != nil {
				log.LogTask(slogger.INFO, "Killing %v failed: %v", pid, err)
			} else {
				log.LogTask(slogger.INFO, "Killed process %v", pid)
			}
		}
	}
	return nil
}

package shell

import (
	"10gen.com/mci/plugin"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// This is a regex used to extract environment variables from the output of pargs -e
// so that we can detect the 'tracer' strings.
var pargsEnvPattern = regexp.MustCompile("^\\s*envp\\[\\d+\\]:\\s*(.*)$")

func trackProcess(key string, pid int, log plugin.Logger) {
	// trackProcess is a noop on solaris, because we detect all the processes to be killed in
	// cleanup() and we don't need to do any special bookkeeping up-front.
}

// getEnv returns a slice of environment variables for the given pid, in the form
// []string{"VAR1=FOO", "VAR2=BAR", ...}
// This function works by calling "pargs -e $pid" and parsing its output.
func getEnv(pid int) ([]string, error) {
	/* In Solaris we extract envrionment variables by calling 'pargs -e $pid'
	on each process in the system. The output of pargs looks like:
	$ pargs -e 499
	499:    /usr/perl5/bin/perl /usr/lib/intrd
	envp[0]: PATH=/usr/sbin:/usr/bin
	envp[1]: PWD=/
	envp[2]: SHLVL=1
	*/
	out, err := exec.Command("pargs", "-e", fmt.Sprintf("%d", pid)).CombinedOutput()
	if err != nil {
		// Probably permission denied or process is gone away.
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	results := make([]string, 0, len(lines))
	for _, line := range lines {
		if matches := pargsEnvPattern.FindStringSubmatch(line); matches != nil {
			results = append(results, matches[1])
		}
	}
	return results, nil
}

func cleanup(key string, log plugin.Logger) error {
	pids, err := listProc()
	if err != nil {
		return err
	}
	pidMarker := fmt.Sprintf("EVR_AGENT_PID=%v", os.Getpid())
	taskMarker := fmt.Sprintf("EVR_TASK_ID=%v", key)
	for _, pid := range pids {
		env, err := getEnv(pid)
		if err != nil {
			continue
		}
		if envHasMarkers(env, pidMarker, taskMarker) {
			p := os.Process{}
			p.Pid = pid
			if err := p.Kill(); err != nil {
				log.LogSystem(slogger.INFO, "Cleanup killing %v failed: %v", pid, err)
			} else {
				log.LogTask(slogger.INFO, "Cleanup killed process %v", pid)
			}
		}
	}
	return nil
}

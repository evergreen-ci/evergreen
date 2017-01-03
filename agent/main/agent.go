package main

import (
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen/agent"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s pulls tasks from the API server and runs them.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "This program is designed to be started by the Evergreen taskrunner, not manually.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Supported flags are:\n")
		flag.PrintDefaults()
	}
}

func main() {
	// Get the basic info needed to run the agent from command line flags.
	taskId := flag.String("task_id", "", "id of task to run")
	taskSecret := flag.String("task_secret", "", "secret of task to run")
	apiServer := flag.String("api_server", "", "URL of API server")
	httpsCertFile := flag.String("https_cert", "", "path to a self-signed private cert")
	logPrefix := flag.String("log_prefix", "", "prefix for the agent's log filename")
	pidFile := flag.String("pid_file", "", "path to pid file")
	flag.Parse()

	httpsCert, err := getHTTPSCertFile(*httpsCertFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not decode https certificate file: %v\n", err)
		os.Exit(1)
	}

	logFile := *logPrefix + logSuffix()

	agt, err := agent.New(*apiServer, *taskId, *taskSecret, logFile, httpsCert, *pidFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not create new agent: %v\n", err)
		os.Exit(1)
	}
	if err := agt.CreatePidFile(*pidFile); err != nil {
		fmt.Fprintf(os.Stderr, "error creating pidFile: %v\n", err)
		os.Exit(1)
	}

	// enable debug traces on SIGQUIT signaling
	go agent.DumpStackOnSIGQUIT(&agt)

	exitCode := 0
	// run all tasks until an API server's response has RunNext set to false
	for {
		resp, err := agt.RunTask()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error running task: %v\n", err)
			exitCode = 1
			break
		}

		if resp == nil {
			fmt.Fprintf(os.Stderr, "received nil response from API server\n")
			exitCode = 1
			break
		}

		if !resp.RunNext {
			break
		}

		agt, err = agent.New(*apiServer, resp.TaskId, resp.TaskSecret, logFile, httpsCert, *pidFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not create new agent for next task '%v': %v\n", resp.TaskId, err)
			exitCode = 1
			break
		}
	}
	agent.ExitAgent(nil, exitCode, *pidFile)
}

// logSuffix generates a unique log filename suffic that is namespaced
// to the PID and Date of the agent's execution.
func logSuffix() string {
	return fmt.Sprintf("_%v_pid_%v.log", time.Now().Format(agent.FilenameTimestamp), os.Getpid())
}

// getHTTPSCertFile fetches the contents of the file at httpsCertFile and
// attempts to decode the pem encoded data contained therein. Returns the
// decoded data.
func getHTTPSCertFile(httpsCertFile string) (string, error) {
	var httpsCert []byte
	var err error

	if httpsCertFile != "" {
		httpsCert, err = ioutil.ReadFile(httpsCertFile)
		if err != nil {
			return "", fmt.Errorf("error reading certficate file %v: %v", httpsCertFile, err)
		}
		// If we don't test the cert here, we won't know if
		// the cert is invalid unil much later
		decoded_cert, _ := pem.Decode([]byte(httpsCert))
		if decoded_cert == nil {
			return "", fmt.Errorf("could not decode certficate file (%v)", httpsCertFile)
		}
	}
	return string(httpsCert), nil
}

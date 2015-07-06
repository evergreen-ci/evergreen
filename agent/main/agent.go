package main

import (
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/evergreen-ci/evergreen/agent"
	"io/ioutil"
	"os"
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
	logFile := flag.String("log_file", "", "log file for agent")
	flag.Parse()

	httpsCert, err := getHTTPSCertFile(*httpsCertFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could decode https certificate file: %v\n", err)
		os.Exit(1)
	}

	agt, err := agent.New(*apiServer, *taskId, *taskSecret, *logFile, httpsCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create new agent: %v\n", err)
		os.Exit(1)
	}

	// run all tasks until an API server's response has RunNext set to false
	for {
		resp, err := agt.RunTask()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error running task: %v\n", err)
			os.Exit(1)
		}

		if resp == nil {
			fmt.Fprintf(os.Stderr, "received nil response from API server\n")
			os.Exit(1)
		}

		if !resp.RunNext {
			os.Exit(0)
		}

		agt, err = agent.New(*apiServer, resp.TaskId, resp.TaskSecret, *logFile, httpsCert)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not create new agent: %v\n", err)
			os.Exit(1)
		}
	}
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

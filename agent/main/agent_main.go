package main

import (
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"io/ioutil"
	"os"
)

// getHTTPSCertFile fetches the contents of the file at httpsCertFile and
// attempts to decode the pem encoded data contained therein. A panic occurs if
// the decoded data is nil
func getHTTPSCertFile(httpsCertFile string) string {
	var httpsCert []byte
	var err error

	if httpsCertFile != "" {
		httpsCert, err = ioutil.ReadFile(httpsCertFile)
		if err == nil {
			// If we don't test the cert here, we won't know if the cert is
			// invalid unil much later
			decoded_cert, _ := pem.Decode([]byte(httpsCert))
			if decoded_cert == nil {
				panic(fmt.Sprintf("Could not decode cert file (%v)",
					httpsCertFile))
			}
		}
	}
	return string(httpsCert)
}

func main() {
	// Get the basic info needed to run the agent from command line flags.
	taskId := flag.String("task_id", "", "id of task to run")
	taskSecret := flag.String("task_secret", "", "secret of task to run")
	motuURL := flag.String("motu_url", "", "URL of motu server")
	configDir := flag.String("config_dir", "", "directory containing task instructions")
	workDir := flag.String("work_dir", evergreen.RemoteShell, "working directory to run task from")
	httpsCertFile := flag.String("https_cert", "", "path to a self-signed private cert")
	flag.Parse()

	httpsCert := getHTTPSCertFile(*httpsCertFile)
	taskAgent, err := agent.NewAgent(
		*motuURL,
		*taskId,
		*taskSecret,
		true,
		httpsCert,
	)
	if err != nil {
		panic(fmt.Sprintf("Could not initialize agent: %v", err))
	}

	taskEndResponse, err := agent.RunTask(
		taskAgent,
		*configDir,
		*workDir)

	// run all tasks until an API server's TaskEndResponse has RunNext set to
	// false
	for {
		if err != nil {
			panic(fmt.Sprintf("Error running task: %v", err))
		}

		if taskEndResponse == nil {
			panic("Received nil response from API server")
		}

		if !taskEndResponse.RunNext {
			os.Exit(0)
		}

		taskAgent, err = agent.NewAgent(
			*motuURL,
			taskEndResponse.TaskId,
			taskEndResponse.TaskSecret,
			true,
			httpsCert,
		)
		if err != nil {
			panic(fmt.Sprintf("Could not initialize agent: %v", err))
		}
		taskEndResponse, err = agent.RunTask(
			taskAgent,
			taskEndResponse.ConfigDir,
			taskEndResponse.WorkDir,
		)
	}
}

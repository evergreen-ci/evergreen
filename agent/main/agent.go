package main

import (
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
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

const (
	filenameTimestamp = "2006-01-02_15_04_05"
	statsPort         = 2285
)

func main() {
	// Get the basic info needed to run the agent from command line flags.
	taskId := flag.String("task_id", "", "id of task to run")
	taskSecret := flag.String("task_secret", "", "secret of task to run")
	hostId := flag.String("host_id", "", "id of machine agent is running on")
	hostSecret := flag.String("host_secret", "", "secret for the current host")
	apiServer := flag.String("api_server", "", "URL of API server")
	httpsCertFile := flag.String("https_cert", "", "path to a self-signed private cert")
	logPrefix := flag.String("log_prefix", "", "prefix for the agent's log filename")
	port := flag.Int("status_port", statsPort, "port to run the status server on")
	flag.Parse()

	logFile := *logPrefix + logSuffix()

	sender, err := send.MakeFileLogger(logFile)
	grip.CatchEmergencyFatal(err)
	defer util.RecoverAndLogStackTrace()

	defer sender.Close()
	grip.CatchEmergencyFatal(grip.SetSender(sender))
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)
	grip.SetName("evg-agent")
	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	httpsCert, err := getHTTPSCertFile(*httpsCertFile)
	if err != nil {
		grip.EmergencyFatalf("could not decode https certificate file: %+v", err)
	}

	initialOptions := agent.Options{
		APIURL:      *apiServer,
		TaskId:      *taskId,
		TaskSecret:  *taskSecret,
		HostId:      *hostId,
		HostSecret:  *hostSecret,
		Certificate: httpsCert,
	}

	// Start a small HTTP server that has a single status endpoint
	go runStatusServer(*port, initialOptions)

	agt, err := agent.New(initialOptions)
	if err != nil {
		grip.EmergencyFatalf("could not create new agent: %+v", err)
	}

	var exitCode int
	var lastTaskId string

	// run all tasks until an API server's response has RunNext set to false
	for {
		resp, err := agt.RunTask()
		if err != nil {
			grip.Criticalf("error running task: %+v", err)
			exitCode = 1
			break
		}

		if resp == nil {
			grip.Criticalf("received nil response from API server")
			exitCode = 1
			break
		}

		if !resp.RunNext {
			break
		}

		lastTaskId = resp.TaskId

		agt, err = agent.New(agent.Options{
			APIURL:      *apiServer,
			TaskId:      resp.TaskId,
			TaskSecret:  resp.TaskSecret,
			HostId:      *hostId,
			HostSecret:  *hostSecret,
			Certificate: httpsCert,
		})

		if err != nil {
			grip.Criticalf("could not create new agent for next task '%s': %+v", resp.TaskId, err)
			exitCode = 1
			break
		}
	}

	agent.ExitAgent(nil, lastTaskId, exitCode)
}

// logSuffix a unique log filename suffix that is namespaced
// to the PID and Date of the agent's execution.
func logSuffix() string {
	return fmt.Sprintf("_%v_pid_%v.log", time.Now().Format(filenameTimestamp), os.Getpid())
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

// runStatusServer starts an http server ruining on the specified
// port. This will exit (e.g. os.Exit()) if the service fails to start.
func runStatusServer(port int, opts agent.Options) {
	// TODO (EVG-1440) eventually this should be a method on the agent
	// object, when the loop (currently in main) is in the agent
	// implementation itself.
	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.UseHandler(agent.GetStatusRouter(opts))

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	grip.Infoln("starting status service on:", addr)
	grip.CatchEmergencyFatal(http.ListenAndServe(addr, n))
}

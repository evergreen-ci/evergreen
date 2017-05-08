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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
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
	agentSleep        = time.Minute * 1
)

// getHTTPSCertFile fetches the contents of the file at httpsCertFile and
// attempts to decode the pem encoded data contained therein. Returns the
// decoded data.
func getHTTPSCertFile(httpsCertFile string) (string, error) {
	var httpsCert []byte
	var err error

	if httpsCertFile != "" {
		httpsCert, err = ioutil.ReadFile(httpsCertFile)
		if err != nil {
			return "", errors.Wrapf(err, "error reading certficate file %v", httpsCertFile)
		}
		// If we don't test the cert here, we won't know if
		// the cert is invalid unil much later
		decoded_cert, _ := pem.Decode(httpsCert)
		if decoded_cert == nil {
			return "", errors.Errorf("could not decode certficate file (%v)", httpsCertFile)
		}
	}
	return string(httpsCert), nil
}

func main() {
	// Get the basic info needed to run the agent from command line flags.
	hostId := flag.String("host_id", "", "id of machine agent is running on")
	hostSecret := flag.String("host_secret", "", "secret for the current host")
	apiServer := flag.String("api_server", "", "URL of API server")
	httpsCertFile := flag.String("https_cert", "", "path to a self-signed private cert")
	logPrefix := flag.String("log_prefix", "", "prefix for the agent's log filename")
	port := flag.Int("status_port", statsPort, "port to run the status server on")
	flag.Parse()

	grip.CatchEmergencyFatal(agent.SetupLogging(logfile))

	grip.CatchEmergencyFatal()
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)
	grip.SetName("evg-agent")

	httpsCert, err := getHTTPSCertFile(*httpsCertFile)
	if err != nil {
		grip.EmergencyFatalf("could not decode https certificate file: %+v", err)
	}

	// all we need is the host id and host secret
	initialOptions := agent.Options{
		APIURL:      *apiServer,
		Certificate: httpsCert,
		HostId:      *hostId,
		HostSecret:  *hostSecret,
		StatusPort:  *port,
		LogPrefix:   *logPrefix,
	}

	agt, err := agent.New(initialOptions)
	if err != nil {
		grip.EmergencyFatalf("could not create new agent: %+v", err)
	}
	grip.CatchEmergencyFatal(agt.Run())
}

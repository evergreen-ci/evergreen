package client

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func (c *podCommunicator) GetAgentSetupData(ctx context.Context) (*apimodels.AgentSetupData, error) {
	return nil, errors.New("TODO: implement")
}

// StartTask marks the task as started.
func (c *podCommunicator) StartTask(ctx context.Context, taskData TaskData) error {
	return errors.New("TODO: implement")
}

// EndTask marks the task as finished with the given status
func (c *podCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	return nil, errors.New("TODO: implement")
}

// GetTask returns the active task.
func (c *podCommunicator) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	return nil, errors.New("TODO: implement")
}

// GetDisplayTaskInfoFromExecution returns the display task info associated
// with the execution task.
func (c *podCommunicator) GetDisplayTaskInfoFromExecution(ctx context.Context, td TaskData) (*apimodels.DisplayTaskInfo, error) {
	return nil, errors.New("TODO: implement")
}

// GetProjectRef loads the task's project.
func (c *podCommunicator) GetProjectRef(ctx context.Context, taskData TaskData) (*model.ProjectRef, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetDistroView(ctx context.Context, taskData TaskData) (*apimodels.DistroView, error) {
	return nil, errors.New("TODO: implement")
}

// GetDistroAMI returns the distro for the task.
func (c *podCommunicator) GetDistroAMI(ctx context.Context, distro, region string, taskData TaskData) (string, error) {
	return "", errors.New("TODO: implement")
}

func (c *podCommunicator) GetProject(ctx context.Context, taskData TaskData) (*model.Project, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetExpansions(ctx context.Context, taskData TaskData) (util.Expansions, error) {
	return nil, errors.New("TODO: implement")
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// an "abort" response. This function returns true if the agent should abort.
func (c *podCommunicator) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	return false, errors.New("TODO: implement")
}

// FetchExpansionVars loads expansions for a communicator's task from the API server.
func (c *podCommunicator) FetchExpansionVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionVars, error) {
	return nil, errors.New("TODO: implement")
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (c *podCommunicator) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	return nil, errors.New("TODO: implement")
}

// GetCedarConfig returns the cedar service information including the base URL,
// URL, RPC port, and credentials.
func (c *podCommunicator) GetCedarConfig(ctx context.Context) (*apimodels.CedarConfig, error) {
	return nil, errors.New("TODO: implement")
}

// GetCedarGRPCConn returns the client connection to cedar if it exists, or
// creates it if it doesn't exist.
func (c *podCommunicator) GetCedarGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	if err := c.createCedarGRPCConn(ctx); err != nil {
		return nil, errors.Wrap(err, "setting up cedar grpc connection")
	}
	return c.cedarGRPCClient, nil
}

func (c *podCommunicator) createCedarGRPCConn(ctx context.Context) error {
	if c.cedarGRPCClient == nil {
		cc, err := c.GetCedarConfig(ctx)
		if err != nil {
			return errors.Wrap(err, "getting cedar config")
		}

		// TODO (EVG-14557): Remove TLS dial option fallback once cedar
		// gRPC is on API auth.
		catcher := grip.NewBasicCatcher()
		dialOpts := timber.DialCedarOptions{
			BaseAddress: cc.BaseURL,
			RPCPort:     cc.RPCPort,
			Username:    cc.Username,
			APIKey:      cc.APIKey,
			Retries:     10,
		}
		if runtime.GOOS == "windows" {
			cas, err := c.getAWSCACerts(ctx)
			if err != nil {
				return errors.Wrap(err, "getting AWS root CA certs for cedar gRPC client connections on Windows")
			}
			dialOpts.CACerts = [][]byte{cas}
		}
		c.cedarGRPCClient, err = timber.DialCedar(ctx, c.cedarHTTPClient, dialOpts)
		if err != nil {
			catcher.Wrap(err, "creating cedar grpc client connection with API auth.")
		} else {
			healthClient := gopb.NewHealthClient(c.cedarGRPCClient)
			_, err = healthClient.Check(ctx, &gopb.HealthCheckRequest{})
			if err == nil {
				return nil
			}
			catcher.Wrap(err, "checking cedar grpc health with API auth")
		}

		// Try again, this time with TLS auth.
		dialOpts.TLSAuth = true
		c.cedarGRPCClient, err = timber.DialCedar(ctx, c.cedarHTTPClient, dialOpts)
		if err == nil {
			return nil
		}
		catcher.Wrap(err, "creating cedar grpc client connection with TLS auth")

		return catcher.Resolve()
	}

	return nil
}

// getAWSCACerts fetches AWS's root CA certificates stored in S3. This is a
// workaround for the fact that Go cannot access the system certificate pool on
// Windows (which would have these certificates).
// TODO: If and when the Windows system cert issue is fixed, we can get rid of
// this workaround. See https://github.com/golang/go/issues/16736.
func (c *podCommunicator) getAWSCACerts(ctx context.Context) ([]byte, error) {
	setupData, err := c.GetAgentSetupData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting setup data")
	}

	// We are hardcoding this magic object in S3 because these certificates
	// are not set to expire for another 20 years. Also, we are hopeful
	// that this Windows system cert issue will go away in future versions
	// of Go.
	bucket, err := pail.NewS3Bucket(pail.S3Options{
		Name:        "boxes.10gen.com",
		Prefix:      "build/amazontrust",
		Region:      endpoints.UsEast1RegionID,
		Credentials: pail.CreateAWSCredentials(setupData.S3Key, setupData.S3Secret, ""),
		MaxRetries:  10,
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating pail bucket")
	}

	r, err := bucket.Get(ctx, "AmazonRootCA_all.pem")
	if err != nil {
		return nil, errors.Wrap(err, "getting AWS root CA certificates")
	}

	catcher := grip.NewBasicCatcher()
	cas, err := ioutil.ReadAll(r)
	catcher.Wrap(err, "reading AWS root CA certificates")
	catcher.Wrap(r.Close(), "closing the ReadCloser")

	return cas, catcher.Resolve()
}

func (c *podCommunicator) GetLoggerProducer(ctx context.Context, td TaskData, config *LoggerConfig) (LoggerProducer, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) makeSender(ctx context.Context, td TaskData, opts []LogOpts, prefix string, logType string) (send.Sender, []send.Sender, error) {
	return nil, nil, errors.New("TODO: implement")
}

// SendLogMessages posts a group of log messages for a task.
func (c *podCommunicator) SendLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	return errors.New("TODO: implement")
}

// SendTaskResults posts a task's results, used by the attach results operations.
func (c *podCommunicator) SendTaskResults(ctx context.Context, taskData TaskData, r *task.LocalTestResults) error {
	return errors.New("TODO: implement")
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure. If patchId is not specified, the task's
// patch is returned
func (c *podCommunicator) GetTaskPatch(ctx context.Context, taskData TaskData, patchId string) (*patchmodel.Patch, error) {
	return nil, errors.New("TODO: implement")
}

// GetPatchFiles is used by the git.get_project plugin and fetches
// patches from the database, used in patch builds.
func (c *podCommunicator) GetPatchFile(ctx context.Context, taskData TaskData, patchFileID string) (string, error) {
	return "", errors.New("TODO: implement")
}

// SendTestLog is used by the attach plugin to add to the test_logs
// collection for log data associated with a test.
func (c *podCommunicator) SendTestLog(ctx context.Context, taskData TaskData, log *model.TestLog) (string, error) {
	return "", errors.New("TODO: implement")
}

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *podCommunicator) SendTestResults(ctx context.Context, taskData TaskData, results *task.LocalTestResults) error {
	return errors.New("TODO: implement")
}

// SetHasCedarResults sets the HasCedarResults flag to true in the given task
// in the database.
func (c *podCommunicator) SetHasCedarResults(ctx context.Context, taskData TaskData, failed bool) error {
	return errors.New("TODO: implement")
}

// AttachFiles attaches task files.
func (c *podCommunicator) AttachFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) SetDownstreamParams(ctx context.Context, downstreamParams []patchmodel.Parameter, taskId string) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) GetManifest(ctx context.Context, taskData TaskData) (*manifest.Manifest, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) S3Copy(ctx context.Context, taskData TaskData, req *apimodels.S3CopyRequest) (string, error) {
	return "", errors.New("TODO: implement")
}

func (c *podCommunicator) KeyValInc(ctx context.Context, taskData TaskData, kv *model.KeyVal) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) PostJSONData(ctx context.Context, taskData TaskData, path string, data interface{}) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) GetJSONData(ctx context.Context, taskData TaskData, taskName, dataName, variantName string) ([]byte, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetJSONHistory(ctx context.Context, taskData TaskData, tags bool, taskName, dataName string) ([]byte, error) {
	return nil, errors.New("TODO: implement")
}

// GenerateTasks posts new tasks for the `generate.tasks` command.
func (c *podCommunicator) GenerateTasks(ctx context.Context, td TaskData, jsonBytes []json.RawMessage) error {
	return errors.New("TODO: implement")
}

// GenerateTasksPoll posts new tasks for the `generate.tasks` command.
func (c *podCommunicator) GenerateTasksPoll(ctx context.Context, td TaskData) (*apimodels.GeneratePollResponse, error) {
	return nil, errors.New("TODO: implement")
}

// CreateHost requests a new host be created
func (c *podCommunicator) CreateHost(ctx context.Context, td TaskData, options apimodels.CreateHost) ([]string, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) ListHosts(ctx context.Context, td TaskData) (restmodel.HostListResults, error) {
	return restmodel.HostListResults{}, errors.New("TODO: implement")
}

func (c *podCommunicator) GetDistroByName(ctx context.Context, id string) (*restmodel.APIDistro, error) {
	return nil, errors.New("TODO: implement")
}

// GetDockerStatus returns status of the container for the given host
func (c *podCommunicator) GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) ConcludeMerge(ctx context.Context, patchId, status string, td TaskData) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) GetAdditionalPatches(ctx context.Context, patchId string, td TaskData) ([]string, error) {
	return nil, errors.New("TODO: implement")
}

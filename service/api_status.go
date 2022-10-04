package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
)

func (as *APIServer) serviceStatusSimple(w http.ResponseWriter, r *http.Request) {
	out := struct {
		BuildID      string `json:"build_revision"`
		AgentVersion string `json:"agent_version"`
	}{
		BuildID:      evergreen.BuildRevision,
		AgentVersion: evergreen.AgentVersion,
	}

	env := evergreen.GetEnvironment()
	if env.ShutdownSequenceStarted() {
		gimlet.WriteJSONInternalError(w, &out)
		return
	}

	gimlet.WriteJSON(w, &out)
}

func (as *APIServer) agentSetup(w http.ResponseWriter, r *http.Request) {
	out := &apimodels.AgentSetupData{
		SplunkServerURL:   as.Settings.Splunk.SplunkConnectionInfo.ServerURL,
		SplunkClientToken: as.Settings.Splunk.SplunkConnectionInfo.Token,
		SplunkChannel:     as.Settings.Splunk.SplunkConnectionInfo.Channel,
		S3Key:             as.Settings.Providers.AWS.S3.Key,
		S3Secret:          as.Settings.Providers.AWS.S3.Secret,
		S3Bucket:          as.Settings.Providers.AWS.S3.Bucket,
		TaskSync:          as.Settings.Providers.AWS.TaskSync,
		EC2Keys:           as.Settings.Providers.AWS.EC2Keys,
		LogkeeperURL:      as.Settings.LoggerConfig.LogkeeperURL,
	}
	gimlet.WriteJSON(w, out)
}

package s3copy

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

func init() {
	plugin.Publish(&S3CopyPlugin{})
}

const (
	s3CopyCmd         = "copy"
	s3CopyPluginName  = "s3Copy"
	s3CopyAPIEndpoint = "s3Copy"
	s3baseURL         = "https://s3.amazonaws.com/"

	s3CopyRetrySleepTimeSec = 5
	s3CopyRetryNumRetries   = 5
	s3ResultsPostRetries    = 5
	s3ResultsRetrySleepSec  = 10 * time.Second
)

// S3CopyRequest holds information necessary for the API server to
// complete an S3 copy request; namely, an S3 key/secret, a source and
// a destination path
type S3CopyRequest struct {
	AwsKey              string `json:"aws_key"`
	AwsSecret           string `json:"aws_secret"`
	S3SourceBucket      string `json:"s3_source_bucket"`
	S3SourcePath        string `json:"s3_source_path"`
	S3DestinationBucket string `json:"s3_destination_bucket"`
	S3DestinationPath   string `json:"s3_destination_path"`
	S3DisplayName       string `json:"display_name"`
}

// The S3CopyPlugin consists of zero or more files that are to be copied
// from one location in S3 to the other.
type S3CopyCommand struct {
	// AwsKey & AwsSecret are provided to make it possible to transfer
	// files to/from any bucket using the appropriate keys for each
	AwsKey    string `mapstructure:"aws_key" plugin:"expand" json:"aws_key"`
	AwsSecret string `mapstructure:"aws_secret" plugin:"expand" json:"aws_key"`

	// An array of file copy configurations
	S3CopyFiles []*s3CopyFile `mapstructure:"s3_copy_files" plugin:"expand"`
}

// S3CopyPlugin is used to copy files around in s3
type S3CopyPlugin struct{}

type s3CopyFile struct {
	// Each source and destination is specified in the
	// following manner:
	//  bucket: <s3 bucket>
	//  path: <path to file>
	//
	// e.g.
	//  bucket: mciuploads
	//  path: linux-64/x86_64/artifact.tgz
	Source      s3Loc `mapstructure:"source" plugin:"expand"`
	Destination s3Loc `mapstructure:"destination" plugin:"expand"`

	// BuildVariants is a slice of build variants for which
	// a specified file is to be copied. An empty slice indicates it is to be
	// copied for all build variants
	BuildVariants []string `mapstructure:"build_variants" plugin:"expand"`

	//DisplayName is the name of the file
	DisplayName string `mapstructure:"display_name" plugin:"expand"`

	// Optional, when true suppresses the error state for the file.
	Optional bool `mapstructure:"optional"`
}

// s3Loc is a format for describing the location of a file in
// Amazon's S3. It contains an entry for the bucket name and another
// describing the path name of the file within the bucket
type s3Loc struct {
	// the s3 bucket for the file
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// the file path within the bucket
	Path string `mapstructure:"path" plugin:"expand"`
}

// Name returns the name of this plugin - it serves to satisfy
// the 'Plugin' interface
func (scp *S3CopyPlugin) Name() string {
	return s3CopyPluginName
}

func (scp *S3CopyPlugin) GetAPIHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc(fmt.Sprintf("/%v", s3CopyAPIEndpoint), S3CopyHandler) // POST
	r.HandleFunc("/", http.NotFound)
	return r
}

func (self *S3CopyPlugin) Configure(map[string]interface{}) error {
	return nil
}

// NewCommand returns the S3CopyPlugin - this is to satisfy the
// 'Plugin' interface
func (scp *S3CopyPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName != s3CopyCmd {
		return nil, fmt.Errorf("No such %v command: %v",
			s3CopyPluginName, cmdName)
	}
	return &S3CopyCommand{}, nil
}

func (scc *S3CopyCommand) Name() string {
	return s3CopyCmd
}

func (scc *S3CopyCommand) Plugin() string {
	return s3CopyPluginName
}

// ParseParams decodes the S3 push command parameters that are
// specified as part of an S3CopyPlugin command; this is required
// to satisfy the 'Command' interface
func (scc *S3CopyCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, scc); err != nil {
		return fmt.Errorf("error decoding %v params: %v", scc.Name(), err)
	}
	if err := scc.validateParams(); err != nil {
		return fmt.Errorf("error validating %v params: %v", scc.Name(), err)
	}
	return nil
}

// validateParams is a helper function that ensures all
// the fields necessary for carrying out an S3 copy operation are present
func (scc *S3CopyCommand) validateParams() (err error) {
	if scc.AwsKey == "" {
		return fmt.Errorf("s3 AWS key cannot be blank")
	}
	if scc.AwsSecret == "" {
		return fmt.Errorf("s3 AWS secret cannot be blank")
	}
	for _, s3CopyFile := range scc.S3CopyFiles {
		if s3CopyFile.Source.Bucket == "" {
			return fmt.Errorf("s3 source bucket cannot be blank")
		}
		if s3CopyFile.Destination.Bucket == "" {
			return fmt.Errorf("s3 destination bucket cannot be blank")
		}
		if s3CopyFile.Source.Path == "" {
			return fmt.Errorf("s3 source path cannot be blank")
		}
		if s3CopyFile.Destination.Path == "" {
			return fmt.Errorf("s3 destination path cannot be blank")
		}
	}

	// validate the S3 copy parameters before running the task
	if err := scc.validateS3CopyParams(); err != nil {
		return err
	}
	return nil
}

// validateS3CopyParams validates the s3 copy params right before executing
func (scc *S3CopyCommand) validateS3CopyParams() (err error) {
	for _, s3CopyFile := range scc.S3CopyFiles {
		err := validateS3BucketName(s3CopyFile.Source.Bucket)
		if err != nil {
			return fmt.Errorf("source bucket '%v' is invalid: %v",
				s3CopyFile.Source.Bucket, err)
		}

		err = validateS3BucketName(s3CopyFile.Destination.Bucket)
		if err != nil {
			return fmt.Errorf("destination bucket '%v' is invalid: %v",
				s3CopyFile.Destination.Bucket, err)
		}
	}
	return nil
}

// Execute carries out the S3CopyCommand command - this is required
// to satisfy the 'Command' interface
func (scc *S3CopyCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	taskConfig *model.TaskConfig,
	stop chan bool) error {

	// expand the S3 copy parameters before running the task
	if err := plugin.ExpandValues(scc, taskConfig.Expansions); err != nil {
		return err
	}

	// validate the S3 copy parameters before running the task
	if err := scc.validateS3CopyParams(); err != nil {
		return err
	}

	errChan := make(chan error)
	go func() {
		errChan <- scc.S3Copy(taskConfig, pluginLogger, pluginCom)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of S3 copy command")
		return nil
	}
}

// S3Copy is responsible for carrying out the core of the S3CopyPlugin's
// function - it makes an API calls to copy a given staged file to it's final
// production destination
func (scc *S3CopyCommand) S3Copy(taskConfig *model.TaskConfig,
	pluginLogger plugin.Logger, pluginCom plugin.PluginCommunicator) error {
	for _, s3CopyFile := range scc.S3CopyFiles {
		if len(s3CopyFile.BuildVariants) > 0 && !util.SliceContains(
			s3CopyFile.BuildVariants, taskConfig.BuildVariant.Name) {
			continue
		}

		pluginLogger.LogExecution(slogger.INFO, "Making API push copy call to "+
			"transfer %v/%v => %v/%v", s3CopyFile.Source.Bucket,
			s3CopyFile.Source.Path, s3CopyFile.Destination.Bucket,
			s3CopyFile.Destination.Path)

		s3CopyReq := S3CopyRequest{
			AwsKey:              scc.AwsKey,
			AwsSecret:           scc.AwsSecret,
			S3SourceBucket:      s3CopyFile.Source.Bucket,
			S3SourcePath:        s3CopyFile.Source.Path,
			S3DestinationBucket: s3CopyFile.Destination.Bucket,
			S3DestinationPath:   s3CopyFile.Destination.Path,
			S3DisplayName:       s3CopyFile.DisplayName,
		}
		resp, err := pluginCom.TaskPostJSON(s3CopyAPIEndpoint, s3CopyReq)
		if resp != nil {
			defer resp.Body.Close()
		}

		if resp != nil && resp.StatusCode != http.StatusOK {
			body, _ := ioutil.ReadAll(resp.Body)
			err := fmt.Errorf("S3 push copy failed (%v): %v",
				resp.StatusCode, string(body))
			if s3CopyFile.Optional {
				pluginLogger.LogExecution(slogger.ERROR,
					"ignoring optional file, which encountered error: %+v",
					err.Error())
				continue
			}

			return err
		}
		if err != nil {
			body, _ := ioutil.ReadAll(resp.Body)
			err = errors.Wrapf(err, "S3 push copy failed (%v): %v",
				resp.StatusCode, string(body))
			if s3CopyFile.Optional {
				pluginLogger.LogExecution(slogger.ERROR,
					"ignoring optional file, which encountered error: %+v",
					err.Error())
				continue
			}

			return err
		}
		pluginLogger.LogExecution(slogger.INFO, "API push copy call succeeded")
		err = scc.AttachTaskFiles(pluginLogger, pluginCom, s3CopyReq)
		if err != nil {
			body, readAllErr := ioutil.ReadAll(resp.Body)
			if readAllErr != nil {
				return fmt.Errorf("Error: %v", err)
			}
			return fmt.Errorf("Error: %v, (%v): %v",
				resp.StatusCode, err, string(body))
		}
	}
	return nil
}

// Takes a request for a task's file to be copied from
// one s3 location to another. Ensures that if the destination
// file path already exists, no file copy is performed.
func S3CopyHandler(w http.ResponseWriter, r *http.Request) {
	task := plugin.GetTask(r)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	s3CopyReq := &S3CopyRequest{}
	err := util.ReadJSONInto(r.Body, s3CopyReq)
	if err != nil {
		grip.Errorln("error reading push request:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the version for this task, so we can check if it has
	// any already-done pushes
	v, err := version.FindOne(version.ById(task.Version))
	if err != nil {
		grip.Errorf("error querying task %s with version id %s: %v",
			task.Id, task.Version, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check for an already-pushed file with this same file path,
	// but from a conflicting or newer commit sequence num
	if v == nil {
		grip.Errorln("no version found for build", task.BuildId)
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}

	copyFromLocation := strings.Join([]string{s3CopyReq.S3SourceBucket, s3CopyReq.S3SourcePath}, "/")
	copyToLocation := strings.Join([]string{s3CopyReq.S3DestinationBucket, s3CopyReq.S3DestinationPath}, "/")

	newestPushLog, err := model.FindPushLogAfter(copyToLocation, v.RevisionOrderNumber)
	if err != nil {
		grip.Errorf("error querying for push log at %s: %+v", copyToLocation, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if newestPushLog != nil {
		grip.Warningln("conflict with existing pushed file:", copyToLocation)
		return
	}

	// It's now safe to put the file in its permanent location.
	newPushLog := model.NewPushLog(v, task, copyToLocation)
	err = newPushLog.Insert()
	if err != nil {
		grip.Errorf("failed to create new push log: %+v %+v", newPushLog, err)
		http.Error(w, fmt.Sprintf("failed to create push log: %v", err), http.StatusInternalServerError)
		return
	}

	// Now copy the file into the permanent location
	auth := &aws.Auth{
		AccessKey: s3CopyReq.AwsKey,
		SecretKey: s3CopyReq.AwsSecret,
	}

	grip.Infof("performing S3 copy: '%s' => '%s'", copyFromLocation, copyToLocation)

	_, err = util.RetryArithmeticBackoff(func() error {
		err := thirdparty.S3CopyFile(auth,
			s3CopyReq.S3SourceBucket,
			s3CopyReq.S3SourcePath,
			s3CopyReq.S3DestinationBucket,
			s3CopyReq.S3DestinationPath,
			string(s3.PublicRead),
		)
		if err != nil {
			grip.Errorf("S3 copy failed for task %s, retrying: %+v", task.Id, err)
			return util.RetriableError{err}
		} else {
			err := newPushLog.UpdateStatus(model.PushLogSuccess)
			if err != nil {
				grip.Errorf("updating pushlog status failed for task %s: %+v", task.Id, err)
			}
			return err
		}
	}, s3CopyRetryNumRetries, s3CopyRetrySleepTimeSec*time.Second)

	if err != nil {
		message := fmt.Sprintf("S3 copy failed for task %v: %v", task.Id, err)
		grip.Error(message)
		err = newPushLog.UpdateStatus(model.PushLogFailed)
		if err != nil {
			grip.Errorln("updating pushlog status failed:", err)
		}
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, "S3 copy Successful")
}

// AttachTaskFiles is responsible for sending the
// specified file to the API Server
func (c *S3CopyCommand) AttachTaskFiles(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, request S3CopyRequest) error {

	remotePath := filepath.ToSlash(request.S3DestinationPath)
	fileLink := s3baseURL + request.S3DestinationBucket + "/" + remotePath

	displayName := request.S3DisplayName

	if displayName == "" {
		displayName = filepath.Base(request.S3SourcePath)
	}

	pluginLogger.LogExecution(slogger.INFO, "attaching file with name %v", displayName)
	file := artifact.File{
		Name: displayName,
		Link: fileLink,
	}

	files := []*artifact.File{&file}

	err := pluginCom.PostTaskFiles(files)
	if err != nil {
		return fmt.Errorf("Attach files failed: %v", err)
	}
	pluginLogger.LogExecution(slogger.INFO, "API attach files call succeeded")
	return nil
}

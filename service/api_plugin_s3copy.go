package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	s3CopyRetryMinDelay = 5 * time.Second
	s3CopyAttempts      = 5
)

// Takes a request for a task's file to be copied from
// one s3 location to another. Ensures that if the destination
// file path already exists, no file copy is performed.
func (as *APIServer) s3copyPlugin(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	s3CopyReq := &apimodels.S3CopyRequest{}
	err := utility.ReadJSON(util.NewRequestReader(r), s3CopyReq)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	// Get the version for this task, so we can check if it has
	// any already-done pushes
	v, err := model.VersionFindOne(model.VersionById(task.Version))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "problem querying task %s with version id %s",
				task.Id, task.Version))
		return
	}

	// Check for an already-pushed file with this same file path,
	// but from a conflicting or newer commit sequence num
	if v == nil {
		as.LoggedError(w, r, http.StatusNotFound,
			errors.Errorf("no version found for build '%s'", task.BuildId))
		return
	}

	copyFromLocation := strings.Join([]string{s3CopyReq.S3SourceBucket, s3CopyReq.S3SourcePath}, "/")
	copyToLocation := strings.Join([]string{s3CopyReq.S3DestinationBucket, s3CopyReq.S3DestinationPath}, "/")

	newestPushLog, err := model.FindPushLogAfter(copyToLocation, v.RevisionOrderNumber)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "problem querying for push log at %s (build=%s)",
				copyToLocation, task.BuildId))
		return
	}

	if newestPushLog != nil {
		grip.Warningln("conflict with existing pushed file:", copyToLocation)
		gimlet.WriteJSON(w, gimlet.ErrorResponse{
			StatusCode: http.StatusOK,
			Message:    fmt.Sprintf("noop, file %s exists", copyToLocation),
		})
		return
	}

	// It's now safe to put the file in its permanent location.
	newPushLog := model.NewPushLog(v, task, copyToLocation)
	if err = newPushLog.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "failed to create new push log: %+v", newPushLog))
	}

	// Now copy the file into the permanent location
	client := utility.GetHTTPClient()
	client.Timeout = 10 * time.Minute
	defer utility.PutHTTPClient(client)
	srcOpts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(s3CopyReq.AwsKey, s3CopyReq.AwsSecret, ""),
		Region:      s3CopyReq.S3SourceRegion,
		Name:        s3CopyReq.S3SourceBucket,
		Permissions: pail.S3Permissions(s3CopyReq.S3Permissions),
	}
	srcBucket, err := pail.NewS3MultiPartBucket(srcOpts)
	if err != nil {
		grip.Error(errors.Wrap(err, "S3 copy failed, could not establish connection to source bucket"))
	}
	destOpts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(s3CopyReq.AwsKey, s3CopyReq.AwsSecret, ""),
		Region:      s3CopyReq.S3DestinationRegion,
		Name:        s3CopyReq.S3DestinationBucket,
		Permissions: pail.S3Permissions(s3CopyReq.S3Permissions),
	}
	destBucket, err := pail.NewS3MultiPartBucket(destOpts)
	if err != nil {
		grip.Error(errors.Wrap(err, "S3 copy failed, could not establish connection to destination bucket"))
	}

	grip.Infof("performing S3 copy: '%s' => '%s'", copyFromLocation, copyToLocation)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = utility.Retry(
		ctx,
		func() (bool, error) {
			copyOpts := pail.CopyOptions{
				SourceKey:         s3CopyReq.S3SourcePath,
				DestinationKey:    s3CopyReq.S3DestinationPath,
				DestinationBucket: destBucket,
			}
			err = srcBucket.Copy(ctx, copyOpts)
			if err != nil {
				grip.Errorf("S3 copy failed for task %s, retrying: %+v", task.Id, err)
				return true, err
			}

			err = errors.Wrapf(newPushLog.UpdateStatus(model.PushLogSuccess),
				"updating pushlog status failed for task %s", task.Id)

			grip.Error(err)

			return false, err
		}, utility.RetryOptions{
			MaxAttempts: s3CopyAttempts,
			MinDelay:    s3CopyRetryMinDelay,
		})

	if err != nil {
		grip.Error(errors.Wrap(errors.WithStack(newPushLog.UpdateStatus(model.PushLogFailed)), "updating pushlog status failed"))

		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "S3 copy failed for task %s", task.Id))
		return
	}

	gimlet.WriteJSON(w, "S3 copy Successful")
}

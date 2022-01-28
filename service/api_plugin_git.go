package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func (as *APIServer) gitServePatch(w http.ResponseWriter, r *http.Request) {
	var patchId string
	if patchParam, exists := r.URL.Query()["patch"]; exists {
		patchId = patchParam[0]
	} else {
		t := MustHaveTask(r)
		patchId = t.Version
	}
	p, err := patch.FindOne(patch.ByVersion(patchId))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "problem fetching patch '%s'", patchId))
		return
	}
	if p == nil {
		as.LoggedError(w, r, http.StatusNotFound,
			errors.Errorf("no patch with ID '%s' found", patchId))
		return
	}

	// add on the merge status for the patch, if applicable
	if p.GetRequester() == evergreen.MergeTestRequester {
		builds, err := build.Find(build.ByVersion(p.Version))
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error retrieving builds for task"))
			return
		}
		tasks, err := task.FindWithFields(task.ByVersion(p.Version), task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "problem finding tasks for version"))
			return
		}

		status := evergreen.PatchSucceeded
		for _, b := range builds {
			if b.BuildVariant == evergreen.MergeTaskVariant {
				continue
			}
			complete, buildStatus, err := b.AllUnblockedTasksFinished(tasks)
			if err != nil {
				as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "problem checking build tasks"))
				return
			}
			if !complete {
				status = evergreen.PatchStarted
				break
			}
			if buildStatus == evergreen.BuildFailed {
				status = evergreen.PatchFailed
				break
			}
		}
		p.MergeStatus = status
	}
	p.MergeStatus = evergreen.PatchSucceeded

	gimlet.WriteJSON(w, p)
}

func (as *APIServer) gitServePatchFile(w http.ResponseWriter, r *http.Request) {
	fileId := gimlet.GetVars(r)["patchfile_id"]
	patchContents, err := patch.FetchPatchContents(fileId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file from db: %v", err), http.StatusInternalServerError)
		return
	}
	gimlet.WriteText(w, patchContents)
}

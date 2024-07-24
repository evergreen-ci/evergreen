package units

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// SchedulePatch schedules a patch. It returns an error and an HTTP status code.
func SchedulePatch(ctx context.Context, env evergreen.Environment, patchId string, version *model.Version, patchUpdateReq model.PatchUpdate) (int, error) {
	var err error
	p, err := patch.FindOneId(patchId)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "loading patch '%s'", patchId)
	}
	if p == nil {
		return http.StatusBadRequest, errors.Errorf("patch '%s' not found", patchId)
	}

	if p.IsCommitQueuePatch() {
		return http.StatusBadRequest, errors.New("can't schedule commit queue patch")
	}
	projectRef, err := model.FindMergedProjectRef(p.Project, p.Version, true)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "finding project ref '%s' for version '%s'", p.Project, p.Version)
	}
	if projectRef == nil {
		return http.StatusInternalServerError, errors.Errorf("project '%s' for version '%s' not found", p.Project, p.Version)
	}
	project, err := model.FindProjectFromVersionID(version.Id)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "finding project for version '%s'", version.Id)
	}
	if project == nil {
		return http.StatusInternalServerError, errors.Errorf("project not found for version '%s'", version.Id)
	}
	taskGroupTasksToAddToVariant := map[string]string{}
	for _, vt := range patchUpdateReq.VariantsTasks {
		for _, t := range vt.Tasks {
			tg := project.FindTaskGroup(t)
			if tg == nil || tg.MaxHosts != 1 {
				continue
			}
			for _, tgt := range tg.Tasks {
				if tgt == t {
					break
				}
				taskGroupTasksToAddToVariant[t] = vt.Variant
			}
		}
	}
	for t, v := range taskGroupTasksToAddToVariant {
		for _, vt := range patchUpdateReq.VariantsTasks {
			if vt.Variant == v && !utility.StringSliceContains(vt.Tasks, t) {
				vt.Tasks = append(vt.Tasks, t)
			}
		}
	}

	statusCode, err := model.ConfigurePatch(ctx, env.Settings(), p, version, projectRef, patchUpdateReq)
	if err != nil {
		return statusCode, err
	}
	if p.Version != "" { // if the version already exists, no more to do
		return http.StatusOK, nil
	}

	// create a separate context from the one the caller has so that the caller
	// can't interrupt the db operations here
	newCxt := context.Background()
	// Process additional patch trigger aliases added via UI.
	// Child patches created with the CLI --trigger-alias flag go through a separate flow, so ensure that new child patches are also created before the parent is finalized.
	if err := ProcessTriggerAliases(ctx, p, projectRef, env, patchUpdateReq.PatchTriggerAliases); err != nil {
		return http.StatusInternalServerError, errors.Wrap(err, "processing patch trigger aliases")
	}
	if len(patchUpdateReq.PatchTriggerAliases) > 0 {
		p.Triggers.Aliases = patchUpdateReq.PatchTriggerAliases
		if err = p.SetTriggerAliases(); err != nil {
			return http.StatusInternalServerError, errors.Wrapf(err, "attaching trigger aliases '%s'", p.Id.Hex())
		}
	}
	_, err = model.FinalizePatch(newCxt, p, p.GetRequester(), "")
	if err != nil {
		return http.StatusInternalServerError, errors.Wrap(err, "finalizing patch")
	}

	if p.IsGithubPRPatch() {
		job := NewGithubStatusUpdateJobForNewPatch(p.Id.Hex())
		if err := evergreen.GetEnvironment().LocalQueue().Put(newCxt, job); err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "adding GitHub status update job to queue")
		}
	}
	return http.StatusOK, nil
}

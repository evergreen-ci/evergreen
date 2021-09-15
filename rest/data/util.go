package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// This file is used to combine operations across data connectors, to avoid
// duplicated connector usage across the codebase.

func CopyProject(ctx context.Context, sc Connector, oldProject, newProject string) (*restModel.APIProjectRef, error) {
	projectToCopy, err := sc.FindProjectById(oldProject, false)
	if err != nil {
		return nil, errors.Wrapf(err, "Database error finding project '%s'", oldProject)
	}
	if projectToCopy == nil {
		return nil, errors.Errorf("project '%s' doesn't exist", oldProject)
	}

	apiErr, ok := err.(gimlet.ErrorResponse)
	if !ok {
		return nil, errors.Errorf("Type assertion failed: type %T does not hold an error", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		return nil, errors.Wrapf(err, "Database error finding project '%s'", newProject)
	}

	// copy project, disable necessary settings
	oldId := projectToCopy.Id
	projectToCopy.Id = "" // this will be regenerated during Create
	projectToCopy.Identifier = newProject
	projectToCopy.Enabled = utility.FalsePtr()
	projectToCopy.PRTestingEnabled = nil
	projectToCopy.CommitQueue.Enabled = nil
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = sc.CreateProject(projectToCopy, u); err != nil {
		return nil, errors.Wrapf(err, "Database error creating project for id '%s'", newProject)
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return nil, errors.Wrap(err, "error building API project from service")
	}

	// copy variables, aliases, and subscriptions
	if err = sc.CopyProjectVars(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying project vars from project '%s'", oldProject)
	}
	if err = sc.CopyProjectAliases(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying aliases from project '%s'", oldProject)
	}
	if err = sc.CopyProjectSubscriptions(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying subscriptions from project '%s'", oldProject)
	}
	// set the same admin roles from the old project on the newly copied project.
	if err = sc.UpdateAdminRoles(projectToCopy, projectToCopy.Admins, nil); err != nil {
		return nil, errors.Wrapf(err, "Database error updating admins for project '%s'", newProject)
	}
	return apiProjectRef, nil
}

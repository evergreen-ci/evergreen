package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// This file is used to combine operations across data connectors, to avoid
// duplicated connector usage across the codebase.

func CopyProject(ctx context.Context, sc Connector, projectToCopy *model.ProjectRef, newProject string) (*restModel.APIProjectRef, error) {
	// copy project, disable necessary settings
	oldId := projectToCopy.Id
	oldIdentifier := projectToCopy.Identifier
	projectToCopy.Id = "" // this will be regenerated during Create
	projectToCopy.Identifier = newProject
	projectToCopy.Enabled = utility.FalsePtr()
	projectToCopy.PRTestingEnabled = nil
	projectToCopy.CommitQueue.Enabled = nil
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err := sc.CreateProject(projectToCopy, u); err != nil {
		return nil, errors.Wrapf(err, "Database error creating project for id '%s'", newProject)
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err := apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return nil, errors.Wrap(err, "error building API project from service")
	}

	// copy variables, aliases, and subscriptions
	if err := sc.CopyProjectVars(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying project vars from project '%s'", oldIdentifier)
	}
	if err := sc.CopyProjectAliases(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying aliases from project '%s'", oldIdentifier)
	}
	if err := sc.CopyProjectSubscriptions(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying subscriptions from project '%s'", oldIdentifier)
	}
	// set the same admin roles from the old project on the newly copied project.
	if err := sc.UpdateAdminRoles(projectToCopy, projectToCopy.Admins, nil); err != nil {
		return nil, errors.Wrapf(err, "Database error updating admins for project '%s'", newProject)
	}
	return apiProjectRef, nil
}

func CreateProject(ctx context.Context, sc Connector, projectRef *model.ProjectRef) (*model.ProjectRef, error) {
	u := gimlet.GetUser(ctx).(*user.DBUser)

	if err := sc.CreateProject(projectRef, u); err != nil {
		return nil, errors.Wrapf(err, "error creating project '%s'", projectRef.Id)
	}

	//should this return an apiProject instead?
	return projectRef, nil
}

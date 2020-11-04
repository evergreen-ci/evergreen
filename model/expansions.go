package model

import (
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// UpdateExpansions first updates expansions with project variables.
func UpdateExpansions(expansions *util.Expansions, projectId string, params []patch.Parameter) error {
	projVars, err := FindOneProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "error finding project vars")
	}
	if projVars == nil {
		return errors.New("project vars not found")
	}

	expansions.Update(projVars.GetUnrestrictedVars())

	for _, param := range params {
		expansions.Put(param.Key, param.Value)
	}
	return nil
}

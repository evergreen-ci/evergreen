package manifest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
)

// ManifestLoadCommand
type ManifestLoadCommand struct{}

func (mfc *ManifestLoadCommand) Name() string {
	return ManifestLoadCmd
}

func (mfc *ManifestLoadCommand) Plugin() string {
	return ManifestPluginName
}

// ManifestLoadCommand-specific implementation of ParseParams.
func (mfc *ManifestLoadCommand) ParseParams(params map[string]interface{}) error {
	return nil
}

// updateExpansions adds the expansions for the manifest's modules into the TaskConfig's Expansions.
func (mfc *ManifestLoadCommand) updateExpansions(manifest *manifest.Manifest,
	conf *model.TaskConfig) {
	for moduleName, _ := range manifest.Modules {
		// put the url for the module in the expansions
		conf.Expansions.Put(fmt.Sprintf("%v_rev", moduleName), manifest.Modules[moduleName].Revision)
	}
}

// Load performs a GET on /manifest/load
func (mfc *ManifestLoadCommand) Load(log plugin.Logger, pluginCom plugin.PluginCommunicator, conf *model.TaskConfig) error {
	var loadedManifest *manifest.Manifest
	var err error

	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := pluginCom.TaskGetJSON(ManifestLoadAPIEndpoint)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				log.LogExecution(slogger.WARN, "Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}
			if resp != nil && resp.StatusCode != http.StatusOK {
				log.LogExecution(slogger.WARN, "Unexpected status code %v, retrying", resp.StatusCode)
				return util.RetriableError{fmt.Errorf("Unexpected status code %v", resp.StatusCode)}
			}
			err = util.ReadJSONInto(resp.Body, &loadedManifest)
			if err != nil {
				return err
			}
			return nil
		})

	_, err = util.RetryArithmeticBackoff(retriableGet, 5, 5*time.Second)
	if err != nil {
		return err
	}
	if loadedManifest == nil {
		return fmt.Errorf("Manifest is empty")
	}
	mfc.updateExpansions(loadedManifest, conf)
	return nil
}

// Implementation of Execute.
func (mfc *ManifestLoadCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig,
	stop chan bool) error {

	errChan := make(chan error)
	go func() {
		errChan <- mfc.Load(pluginLogger, pluginCom, conf)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of manifest load command")
		return nil
	}

}

// ManifestLoadHandler attempts to get the manifest, if it exists it updates the expansions and returns
// If it does not exist it performs GitHub API calls for each of the project's modules and gets
// the head revision of the branch and inserts it into the manifest collection.
// If there is a duplicate key error, then do a find on the manifest again.
func (mp *ManifestPlugin) ManifestLoadHandler(w http.ResponseWriter, r *http.Request) {
	task := plugin.GetTask(r)
	projectRef, err := model.FindOneProjectRef(task.Project)
	if err != nil {
		http.Error(w, fmt.Sprintf("projectRef not found for project %v: %v", task.Project, err), http.StatusNotFound)
		return
	}
	project, err := model.FindProject("", projectRef)
	if err != nil {
		http.Error(w, fmt.Sprintf("project not found for ProjectRef %v: %v", projectRef.Identifier, err),
			http.StatusNotFound)
		return
	}
	if project == nil {
		http.Error(w, fmt.Sprintf("empty project not found for ProjectRef %v: %v", projectRef.Identifier, err),
			http.StatusNotFound)
		return
	}

	// try to get the manifest
	currentManifest, err := manifest.FindOne(manifest.ById(task.Version))
	if err != nil {
		http.Error(w, fmt.Sprintf("error retrieving manifest with version id %v: %v", task.Version, err),
			http.StatusNotFound)
		return
	}
	if currentManifest != nil {
		plugin.WriteJSON(w, http.StatusOK, currentManifest)
		return
	}

	if task.Version == "" {
		http.Error(w, fmt.Sprintf("versionId is empty"), http.StatusNotFound)
		return
	}

	// attempt to insert a manifest after making GitHub API calls
	newManifest := &manifest.Manifest{
		Id:          task.Version,
		Revision:    task.Revision,
		ProjectName: task.Project,
		Branch:      project.Branch,
	}

	// populate modules
	modules := make(map[string]*manifest.Module)
	for _, module := range project.Modules {
		owner, repo := module.GetRepoOwnerAndName()
		gitBranch, err := thirdparty.GetBranchEvent(mp.OAuthCredentials, owner, repo, module.Branch)
		if err != nil {
			http.Error(w, fmt.Sprintf("error retrieving getting git branch for module %v: %v", module.Name, err),
				http.StatusNotFound)
			return
		}

		modules[module.Name] = &manifest.Module{
			Branch:   module.Branch,
			Revision: gitBranch.Commit.SHA,
			Repo:     module.Repo,
			Owner:    owner,
			URL:      gitBranch.Commit.Url,
		}
	}
	newManifest.Modules = modules
	duplicate, err := newManifest.TryInsert()
	if err != nil {
		http.Error(w, fmt.Sprintf("error inserting manifest for %v: %v", newManifest.ProjectName, err),
			http.StatusNotFound)
		return
	}
	// if it is a duplicate, load the manifest again`
	if duplicate {
		// try to get the manifest
		m, err := manifest.FindOne(manifest.ById(task.Version))
		if err != nil {
			http.Error(w, fmt.Sprintf("error getting latest manifest for %v: %v", newManifest.ProjectName, err),
				http.StatusNotFound)
			return
		}
		if m != nil {
			plugin.WriteJSON(w, http.StatusOK, m)
			return
		}
	}
	// no duplicate key error, use the manifest just created.

	plugin.WriteJSON(w, http.StatusOK, newManifest)
	return
}

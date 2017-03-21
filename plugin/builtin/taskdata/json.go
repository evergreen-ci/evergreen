package taskdata

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	plugin.Publish(&TaskJSONPlugin{})
}

const (
	TaskJSONPluginName = "json"
	TaskJSONSend       = "send"
	TaskJSONGet        = "get"
	TaskJSONGetHistory = "get_history"
	TaskJSONHistory    = "history"
)

// TaskJSONPlugin handles thet
type TaskJSONPlugin struct{}

// Name implements Plugin Interface.
func (jsp *TaskJSONPlugin) Name() string {
	return TaskJSONPluginName
}

// GetRoutes returns an API route for serving patch data.
func (jsp *TaskJSONPlugin) GetAPIHandler() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/tags/{task_name}/{name}", apiGetTagsForTask)
	r.HandleFunc("/history/{task_name}/{name}", apiGetTaskHistory)

	r.HandleFunc("/data/{name}", apiInsertTask)
	r.HandleFunc("/data/{task_name}/{name}", apiGetTaskByName)
	r.HandleFunc("/data/{task_name}/{name}/{variant}", apiGetTaskForVariant)
	return r
}

func (hwp *TaskJSONPlugin) GetUIHandler() http.Handler {
	r := mux.NewRouter()

	// version routes
	r.HandleFunc("/version", getVersion)
	r.HandleFunc("/version/{version_id}/{name}/", uiGetTasksForVersion)
	r.HandleFunc("/version/latest/{project_id}/{name}", uiGetTasksForLatestVersion)

	// task routes
	r.HandleFunc("/task/{task_id}/{name}/", uiGetTaskById)
	r.HandleFunc("/task/{task_id}/{name}/tags", uiGetTags)
	r.HandleFunc("/task/{task_id}/{name}/tag", uiHandleTaskTag).Methods("POST", "DELETE")

	r.HandleFunc("/tag/{project_id}/{tag}/{variant}/{task_name}/{name}", uiGetTaskJSONByTag)
	r.HandleFunc("/commit/{project_id}/{revision}/{variant}/{task_name}/{name}", uiGetCommit)
	r.HandleFunc("/history/{task_id}/{name}", uiGetTaskHistory)
	return r
}

func fixPatchInHistory(taskId string, base *task.Task, history []TaskJSON) ([]TaskJSON, error) {
	var jsonForTask *TaskJSON
	err := db.FindOneQ(collection, db.Query(bson.M{"task_id": taskId}), &jsonForTask)
	if err != nil {
		return nil, err
	}
	if base != nil {
		jsonForTask.RevisionOrderNumber = base.RevisionOrderNumber
	}
	if jsonForTask == nil {
		return history, nil
	}

	found := false
	for i, item := range history {
		if item.Revision == base.Revision {
			history[i] = *jsonForTask
			found = true
		}
	}
	// if found is false, it means we don't have json on the base commit, so it was
	// not replaced and we must add it explicitly
	if !found {
		history = append(history, *jsonForTask)
	}
	return history, nil
}

func (jsp *TaskJSONPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required to fulfill the Plugin interface. This plugin
// does not have any UI hooks.
func (jsp *TaskJSONPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{}, nil
}

// NewCommand returns requested commands by name. Fulfills the Plugin interface.
func (jsp *TaskJSONPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == TaskJSONSend {
		return &TaskJSONSendCommand{}, nil
	} else if cmdName == TaskJSONGet {
		return &TaskJSONGetCommand{}, nil
	} else if cmdName == TaskJSONGetHistory {
		return &TaskJSONHistoryCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}

type TaskJSONSendCommand struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
}

func (tjsc *TaskJSONSendCommand) Name() string {
	return "send"
}

func (tjsc *TaskJSONSendCommand) Plugin() string {
	return "json"
}

func (tjsc *TaskJSONSendCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, tjsc); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", tjsc.Name(), err)
	}
	return nil
}

func (tjsc *TaskJSONSendCommand) Execute(log plugin.Logger, com plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	if tjsc.File == "" {
		return fmt.Errorf("'file' param must not be blank")
	}
	if tjsc.DataName == "" {
		return fmt.Errorf("'name' param must not be blank")
	}

	errChan := make(chan error)
	go func() {
		// attempt to open the file
		fileLoc := filepath.Join(conf.WorkDir, tjsc.File)
		jsonFile, err := os.Open(fileLoc)
		if err != nil {
			errChan <- fmt.Errorf("Couldn't open json file: '%v'", err)
			return
		}

		jsonData := map[string]interface{}{}
		err = util.ReadJSONInto(jsonFile, &jsonData)
		if err != nil {
			errChan <- fmt.Errorf("File contained invalid json: %v", err)
			return
		}

		retriablePost := util.RetriableFunc(
			func() error {
				log.LogTask(slogger.INFO, "Posting JSON")
				resp, err := com.TaskPostJSON(fmt.Sprintf("data/%v", tjsc.DataName), jsonData)
				if resp != nil {
					defer resp.Body.Close()
				}
				if err != nil {
					return util.RetriableError{err}
				}
				if resp.StatusCode != http.StatusOK {
					return util.RetriableError{fmt.Errorf("unexpected status code %v", resp.StatusCode)}
				}
				return nil
			},
		)

		_, err = util.Retry(retriablePost, 10, 3*time.Second)
		errChan <- err
	}()

	select {
	case err := <-errChan:
		if err != nil {
			log.LogTask(slogger.ERROR, "Sending json data failed: %v", err)
		}
		return err
	case <-stop:
		log.LogExecution(slogger.INFO, "Received abort signal, stopping.")
		return nil
	}
}

type TaskJSONGetCommand struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	TaskName string `mapstructure:"task" plugin:"expand"`
	Variant  string `mapstructure:"variant" plugin:"expand"`
}

type TaskJSONHistoryCommand struct {
	Tags     bool   `mapstructure:"tags"`
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	TaskName string `mapstructure:"task" plugin:"expand"`
}

func (jgc *TaskJSONGetCommand) Name() string {
	return TaskJSONGet
}

func (jgc *TaskJSONHistoryCommand) Name() string {
	return TaskJSONHistory
}

func (jgc *TaskJSONGetCommand) Plugin() string {
	return TaskJSONPluginName
}

func (jgc *TaskJSONHistoryCommand) Plugin() string {
	return TaskJSONPluginName
}

func (jgc *TaskJSONGetCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, jgc); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", jgc.Name(), err)
	}
	if jgc.File == "" {
		return fmt.Errorf("JSON 'get' command must not have blank 'file' parameter")
	}
	return nil
}

func (jgc *TaskJSONHistoryCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, jgc); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", jgc.Name(), err)
	}
	if jgc.File == "" {
		return fmt.Errorf("JSON 'history' command must not have blank 'file' parameter")
	}
	return nil
}

func (jgc *TaskJSONGetCommand) Execute(log plugin.Logger, com plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {

	err := plugin.ExpandValues(jgc, conf.Expansions)
	if err != nil {
		return err
	}

	if jgc.File == "" {
		return fmt.Errorf("'file' param must not be blank")
	}
	if jgc.DataName == "" {
		return fmt.Errorf("'name' param must not be blank")
	}
	if jgc.TaskName == "" {
		return fmt.Errorf("'task' param must not be blank")
	}

	if jgc.File != "" && !filepath.IsAbs(jgc.File) {
		jgc.File = filepath.Join(conf.WorkDir, jgc.File)
	}

	retriableGet := util.RetriableFunc(
		func() error {
			dataUrl := fmt.Sprintf("data/%s/%s", jgc.TaskName, jgc.DataName)
			if jgc.Variant != "" {
				dataUrl = fmt.Sprintf("data/%s/%s/%s", jgc.TaskName, jgc.DataName, jgc.Variant)
			}
			resp, err := com.TaskGetJSON(dataUrl)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				log.LogExecution(slogger.WARN, "Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}

			if resp.StatusCode == http.StatusOK {
				jsonBytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				return ioutil.WriteFile(jgc.File, jsonBytes, 0755)
			}
			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					return fmt.Errorf("No JSON data found")
				}
				return util.RetriableError{fmt.Errorf("unexpected status code %v", resp.StatusCode)}
			}
			return nil
		},
	)
	_, err = util.Retry(retriableGet, 10, 3*time.Second)
	return err
}

func (jgc *TaskJSONHistoryCommand) Execute(log plugin.Logger, com plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	err := plugin.ExpandValues(jgc, conf.Expansions)
	if err != nil {
		return err
	}

	if jgc.File == "" {
		return fmt.Errorf("'file' param must not be blank")
	}
	if jgc.DataName == "" {
		return fmt.Errorf("'name' param must not be blank")
	}
	if jgc.TaskName == "" {
		return fmt.Errorf("'task' param must not be blank")
	}

	if jgc.File != "" && !filepath.IsAbs(jgc.File) {
		jgc.File = filepath.Join(conf.WorkDir, jgc.File)
	}

	endpoint := fmt.Sprintf("history/%s/%s", jgc.TaskName, jgc.DataName)
	if jgc.Tags {
		endpoint = fmt.Sprintf("tags/%s/%s", jgc.TaskName, jgc.DataName)
	}

	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := com.TaskGetJSON(endpoint)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				log.LogExecution(slogger.WARN, "Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}

			if resp.StatusCode == http.StatusOK {
				jsonBytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				return ioutil.WriteFile(jgc.File, jsonBytes, 0755)
			}
			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					return fmt.Errorf("No JSON data found")
				}
				return util.RetriableError{fmt.Errorf("unexpected status code %v", resp.StatusCode)}
			}
			return nil
		},
	)
	_, err = util.Retry(retriableGet, 10, 3*time.Second)
	return err
}

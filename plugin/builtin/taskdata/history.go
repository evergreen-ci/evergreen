package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
)

func GetTaskHistory(t *task.Task, name string) ([]TaskJSON, error) {
	var t2 *task.Task = t
	var err error
	if t.Requester == evergreen.PatchVersionRequester {
		t2, err = t.FindTaskOnBaseCommit()
		if err != nil {
			return nil, err
		}
		t.RevisionOrderNumber = t2.RevisionOrderNumber
	}

	before := []TaskJSON{}
	jsonQuery := db.Query(bson.M{
		ProjectIdKey:           t.Project,
		VariantKey:             t.BuildVariant,
		RevisionOrderNumberKey: bson.M{"$lte": t.RevisionOrderNumber},
		TaskNameKey:            t.DisplayName,
		IsPatchKey:             false,
		NameKey:                name,
	})
	jsonQuery = jsonQuery.Sort([]string{"-order"}).Limit(100)
	err = db.FindAllQ(collection, jsonQuery, &before)
	if err != nil {
		return nil, err
	}
	//reverse order of "before" because we had to sort it backwards to apply the limit correctly:
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}

	after := []TaskJSON{}
	jsonAfterQuery := db.Query(bson.M{
		ProjectIdKey:           t.Project,
		VariantKey:             t.BuildVariant,
		RevisionOrderNumberKey: bson.M{"$gt": t.RevisionOrderNumber},
		TaskNameKey:            t.DisplayName,
		IsPatchKey:             false,
		NameKey:                name}).Sort([]string{"order"}).Limit(100)
	err = db.FindAllQ(collection, jsonAfterQuery, &after)
	if err != nil {
		return nil, err
	}

	//concatenate before + after
	before = append(before, after...)

	// if our task was a patch, replace the base commit's info in the history with the patch
	if t.Requester == evergreen.PatchVersionRequester {
		before, err = fixPatchInHistory(t.Id, t2, before)
		if err != nil {
			return nil, err
		}
	}
	return before, nil
}

// getTaskHistory finds previous tasks by task name.
func apiGetTaskHistory(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	history, err := GetTaskHistory(t, mux.Vars(r)["name"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, history)
}

func uiGetTaskHistory(w http.ResponseWriter, r *http.Request) {
	t, err := task.FindOne(task.ById(mux.Vars(r)["task_id"]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}

	history, err := GetTaskHistory(t, mux.Vars(r)["name"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, history)
}

// Code generated by rest/model/codegen.go. DO NOT EDIT.

package model

import "github.com/evergreen-ci/evergreen/model/artifact"

type APIEntry struct {
	TaskId string    `json:"task_id"`
	Files  []APIFile `json:"files"`
}

func APIEntryBuildFromService(t artifact.Entry) *APIEntry {
	m := APIEntry{}
	m.Files = ArrartifactFileArrAPIFile(t.Files)
	m.TaskId = StringString(t.TaskId)
	return &m
}

func APIEntryToService(m APIEntry) *artifact.Entry {
	out := &artifact.Entry{}
	out.Files = ArrAPIFileArrartifactFile(m.Files)
	out.TaskId = StringString(m.TaskId)
	return out
}

// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package graphql

import (
	"fmt"
	"io"
	"strconv"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/rest/model"
)

type BaseTaskMetadata struct {
	BaseTaskDuration *model.APIDuration `json:"baseTaskDuration"`
	BaseTaskLink     string             `json:"baseTaskLink"`
}

type Dependency struct {
	Name           string         `json:"name"`
	MetStatus      MetStatus      `json:"metStatus"`
	RequiredStatus RequiredStatus `json:"requiredStatus"`
	BuildVariant   string         `json:"buildVariant"`
	UILink         string         `json:"uiLink"`
}

type DisplayTask struct {
	Name      string   `json:"Name"`
	ExecTasks []string `json:"ExecTasks"`
}

type GroupedFiles struct {
	TaskName *string          `json:"taskName"`
	Files    []*model.APIFile `json:"files"`
}

type GroupedProjects struct {
	Name     string                   `json:"name"`
	Projects []*model.UIProjectFields `json:"projects"`
}

type PatchBuildVariant struct {
	Variant string                   `json:"variant"`
	Tasks   []*PatchBuildVariantTask `json:"tasks"`
}

type PatchBuildVariantTask struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type PatchDuration struct {
	Makespan  *string    `json:"makespan"`
	TimeTaken *string    `json:"timeTaken"`
	Time      *PatchTime `json:"time"`
}

type PatchMetadata struct {
	Author string `json:"author"`
}

type PatchReconfigure struct {
	Description   string          `json:"description"`
	VariantsTasks []*VariantTasks `json:"variantsTasks"`
}

type PatchTime struct {
	Started     *string `json:"started"`
	Finished    *string `json:"finished"`
	SubmittedAt string  `json:"submittedAt"`
}

type Projects struct {
	Favorites     []*model.UIProjectFields `json:"favorites"`
	OtherProjects []*GroupedProjects       `json:"otherProjects"`
}

type RecentTaskLogs struct {
	EventLogs  []*model.APIEventLogEntry `json:"eventLogs"`
	TaskLogs   []*apimodels.LogMessage   `json:"taskLogs"`
	SystemLogs []*apimodels.LogMessage   `json:"systemLogs"`
	AgentLogs  []*apimodels.LogMessage   `json:"agentLogs"`
}

type TaskFiles struct {
	FileCount    int             `json:"fileCount"`
	GroupedFiles []*GroupedFiles `json:"groupedFiles"`
}

type TaskResult struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	BaseStatus   string `json:"baseStatus"`
	BuildVariant string `json:"buildVariant"`
}

type VariantTasks struct {
	Variant      string         `json:"variant"`
	Tasks        []string       `json:"tasks"`
	DisplayTasks []*DisplayTask `json:"displayTasks"`
}

type MetStatus string

const (
	MetStatusUnmet   MetStatus = "UNMET"
	MetStatusMet     MetStatus = "MET"
	MetStatusPending MetStatus = "PENDING"
)

var AllMetStatus = []MetStatus{
	MetStatusUnmet,
	MetStatusMet,
	MetStatusPending,
}

func (e MetStatus) IsValid() bool {
	switch e {
	case MetStatusUnmet, MetStatusMet, MetStatusPending:
		return true
	}
	return false
}

func (e MetStatus) String() string {
	return string(e)
}

func (e *MetStatus) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = MetStatus(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid MetStatus", str)
	}
	return nil
}

func (e MetStatus) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type RequiredStatus string

const (
	RequiredStatusMustFail    RequiredStatus = "MUST_FAIL"
	RequiredStatusMustFinish  RequiredStatus = "MUST_FINISH"
	RequiredStatusMustSucceed RequiredStatus = "MUST_SUCCEED"
)

var AllRequiredStatus = []RequiredStatus{
	RequiredStatusMustFail,
	RequiredStatusMustFinish,
	RequiredStatusMustSucceed,
}

func (e RequiredStatus) IsValid() bool {
	switch e {
	case RequiredStatusMustFail, RequiredStatusMustFinish, RequiredStatusMustSucceed:
		return true
	}
	return false
}

func (e RequiredStatus) String() string {
	return string(e)
}

func (e *RequiredStatus) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = RequiredStatus(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid RequiredStatus", str)
	}
	return nil
}

func (e RequiredStatus) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type SortDirection string

const (
	SortDirectionAsc  SortDirection = "ASC"
	SortDirectionDesc SortDirection = "DESC"
)

var AllSortDirection = []SortDirection{
	SortDirectionAsc,
	SortDirectionDesc,
}

func (e SortDirection) IsValid() bool {
	switch e {
	case SortDirectionAsc, SortDirectionDesc:
		return true
	}
	return false
}

func (e SortDirection) String() string {
	return string(e)
}

func (e *SortDirection) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = SortDirection(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid SortDirection", str)
	}
	return nil
}

func (e SortDirection) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type TaskSortCategory string

const (
	TaskSortCategoryName       TaskSortCategory = "NAME"
	TaskSortCategoryStatus     TaskSortCategory = "STATUS"
	TaskSortCategoryBaseStatus TaskSortCategory = "BASE_STATUS"
	TaskSortCategoryVariant    TaskSortCategory = "VARIANT"
)

var AllTaskSortCategory = []TaskSortCategory{
	TaskSortCategoryName,
	TaskSortCategoryStatus,
	TaskSortCategoryBaseStatus,
	TaskSortCategoryVariant,
}

func (e TaskSortCategory) IsValid() bool {
	switch e {
	case TaskSortCategoryName, TaskSortCategoryStatus, TaskSortCategoryBaseStatus, TaskSortCategoryVariant:
		return true
	}
	return false
}

func (e TaskSortCategory) String() string {
	return string(e)
}

func (e *TaskSortCategory) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TaskSortCategory(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TaskSortCategory", str)
	}
	return nil
}

func (e TaskSortCategory) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type TestSortCategory string

const (
	TestSortCategoryStatus   TestSortCategory = "STATUS"
	TestSortCategoryDuration TestSortCategory = "DURATION"
	TestSortCategoryTestName TestSortCategory = "TEST_NAME"
)

var AllTestSortCategory = []TestSortCategory{
	TestSortCategoryStatus,
	TestSortCategoryDuration,
	TestSortCategoryTestName,
}

func (e TestSortCategory) IsValid() bool {
	switch e {
	case TestSortCategoryStatus, TestSortCategoryDuration, TestSortCategoryTestName:
		return true
	}
	return false
}

func (e TestSortCategory) String() string {
	return string(e)
}

func (e *TestSortCategory) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TestSortCategory(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TestSortCategory", str)
	}
	return nil
}

func (e TestSortCategory) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

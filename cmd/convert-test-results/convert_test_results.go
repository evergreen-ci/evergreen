package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"go.mongodb.org/mongo-driver/bson"
)

type oldTestResult struct {
	TaskID        string     `json:"task_id" bson:"task_id"`
	TaskExecution int        `json:"task_execution" bson:"task_execution"`
	TestFile      string     `json:"test_file" bson:"test_file"`
	Status        string     `json:"status" bson:"status"`
	GroupID       string     `json:"group_id" bson:"group_id"`
	URL           string     `json:"url" bson:"url"`
	URLRaw        string     `json:"url_raw" bson:"url_raw"`
	LineNum       int        `json:"line_num" bson:"line_num"`
	Start         float64    `json:"start" bson:"start"`
	End           float64    `json:"end" bson:"end"`
	TestStartTime *time.Time `json:"test_start_time,omitempty" bson:"test_start_time"`
	TestEndTime   *time.Time `json:"test_end_time,omitempty" bson:"test_end_time"`
}

func main() {
	f, err := os.Open("../../testdata/smoke/testresults.json")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	var oldResults []oldTestResult
	for scanner.Scan() {
		var oldResult oldTestResult
		if err := bson.UnmarshalExtJSON(scanner.Bytes(), false, &oldResult); err != nil {
			fmt.Println(err)
			return
		}

		oldResults = append(oldResults, oldResult)
	}

	newResults := make([]testresult.TestResult, len(oldResults))
	for i, oldResult := range oldResults {
		newResults[i].TaskID = oldResult.TaskID
		newResults[i].Execution = oldResult.TaskExecution
		newResults[i].TestName = oldResult.TestFile
		newResults[i].Status = oldResult.Status
		newResults[i].GroupID = oldResult.GroupID
		newResults[i].LogURL = oldResult.URL
		newResults[i].RawLogURL = oldResult.URLRaw
		newResults[i].LineNum = oldResult.LineNum
		if oldResult.TestStartTime != nil {
			newResults[i].Start = oldResult.TestStartTime.UTC()
		} else {
			newResults[i].Start = utility.FromPythonTime(oldResult.Start).UTC()
		}
		if oldResult.TestEndTime != nil {
			newResults[i].End = oldResult.TestEndTime.UTC()
		} else {
			newResults[i].End = utility.FromPythonTime(oldResult.End).UTC()
		}
	}

	data, err := json.MarshalIndent(&newResults, "", "  ")
	if err != nil {
		fmt.Println(err)
		return
	}
	if err = os.WriteFile("out.json", data, 0777); err != nil {
		fmt.Println(err)
	}

	fmt.Printf("successfully converted %d results\n", len(newResults))
}

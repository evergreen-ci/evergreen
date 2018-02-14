package model

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

var taskHistoryTestConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(taskHistoryTestConfig.SessionFactory())
}

func TestTaskHistory(t *testing.T) {

	Convey("With a task history iterator", t, func() {

		buildVariants := []string{"bv_0", "bv_1", "bv_2"}
		projectName := "project"
		taskHistoryIterator := NewTaskHistoryIterator(evergreen.CompileStage,
			buildVariants, projectName)

		Convey("when finding task history items", func() {

			testutil.HandleTestingErr(db.ClearCollections(version.Collection, task.Collection),
				t, "Error clearing test collections")

			for i := 10; i < 20; i++ {
				projectToUse := projectName
				if i == 14 {
					projectToUse = "otherBranch"
				}

				vid := fmt.Sprintf("v%v", i)
				ver := &version.Version{
					Id:                  vid,
					RevisionOrderNumber: i,
					Revision:            vid,
					Requester:           evergreen.RepotrackerVersionRequester,
					Identifier:          projectToUse,
				}

				testutil.HandleTestingErr(ver.Insert(), t,
					"Error inserting version")
				for j := 0; j < 3; j++ {
					newTask := &task.Task{
						Id:                  fmt.Sprintf("t%v_%v", i, j),
						BuildVariant:        fmt.Sprintf("bv_%v", j),
						DisplayName:         evergreen.CompileStage,
						RevisionOrderNumber: i,
						Revision:            vid,
						Requester:           evergreen.RepotrackerVersionRequester,
						Project:             projectToUse,
					}
					testutil.HandleTestingErr(newTask.Insert(), t,
						"Error inserting task")
				}

			}

			Convey("the specified number of task history items should be"+
				" fetched, starting at the specified version", func() {

				taskHistoryChunk, err := taskHistoryIterator.GetChunk(nil, 5, 0, false)
				versions := taskHistoryChunk.Versions
				tasks := taskHistoryChunk.Tasks
				So(err, ShouldBeNil)
				So(taskHistoryChunk.Exhausted.Before, ShouldBeFalse)
				So(taskHistoryChunk.Exhausted.After, ShouldBeTrue)
				So(len(versions), ShouldEqual, 5)
				So(len(tasks), ShouldEqual, len(versions))
				So(versions[0].Id, ShouldEqual, tasks[0]["_id"])
				So(versions[len(versions)-1].Id, ShouldEqual, "v15")
				So(tasks[len(tasks)-1]["_id"], ShouldEqual,
					versions[len(versions)-1].Id)

			})

			Convey("tasks from a different project should be filtered"+
				" out", func() {

				vBefore, err := version.FindOne(version.ById("v15"))
				So(err, ShouldBeNil)

				taskHistoryChunk, err := taskHistoryIterator.GetChunk(vBefore, 5, 0, false)
				versions := taskHistoryChunk.Versions
				tasks := taskHistoryChunk.Tasks
				So(err, ShouldBeNil)
				So(taskHistoryChunk.Exhausted.Before, ShouldBeTrue)
				So(taskHistoryChunk.Exhausted.After, ShouldBeFalse)
				// Should skip 14 because its in another project
				So(versions[0].Id, ShouldEqual, "v13")
				So(versions[0].Id, ShouldEqual, tasks[0]["_id"])
				So(len(tasks), ShouldEqual, 4)
				So(len(tasks), ShouldEqual, len(versions))
				So(tasks[len(tasks)-1]["_id"], ShouldEqual,
					versions[len(versions)-1].Id)

			})

		})

	})

}

func TestMergeResults(t *testing.T) {
	Convey("With a list of two different test history results", t, func() {
		currentTestHistory := []TestHistoryResult{
			{TestFile: "abc", TaskId: "test1", OldTaskId: ""},
			{TestFile: "def", TaskId: "test1", OldTaskId: ""},
			{TestFile: "ghi", TaskId: "test3", OldTaskId: ""},
		}

		oldTestHistory := []TestHistoryResult{
			{TestFile: "abc", TaskId: "test1_1", OldTaskId: "test1"},
			{TestFile: "abc", TaskId: "test1_2", OldTaskId: "test1"},
			{TestFile: "def", TaskId: "test1_1", OldTaskId: "test1"},
		}

		testHistoryWithEmpty := []TestHistoryResult{
			TestHistoryResult{},
			TestHistoryResult{},
		}
		Convey("current and old test history are merged properly", func() {
			allResults := mergeResults(currentTestHistory, oldTestHistory)
			So(len(allResults), ShouldEqual, 6)
			So(allResults[0].TaskId, ShouldEqual, "test1")
			So(allResults[0].TestFile, ShouldEqual, "abc")

			So(allResults[1].TaskId, ShouldEqual, "test1_1")
			So(allResults[1].TestFile, ShouldEqual, "abc")

			So(allResults[2].TaskId, ShouldEqual, "test1_2")
			So(allResults[2].TestFile, ShouldEqual, "abc")

			So(allResults[3].TaskId, ShouldEqual, "test1")
			So(allResults[3].TestFile, ShouldEqual, "def")

			So(allResults[4].TaskId, ShouldEqual, "test1_1")
			So(allResults[4].TestFile, ShouldEqual, "def")

			So(allResults[5].TaskId, ShouldEqual, "test3")
			So(allResults[5].TestFile, ShouldEqual, "ghi")

		})
		Convey("merging in empty results should only produce ones for the results that exist", func() {
			allResults := mergeResults(currentTestHistory, testHistoryWithEmpty)
			So(len(allResults), ShouldEqual, 3)
			So(allResults[0].TaskId, ShouldEqual, "test1")
			So(allResults[1].TaskId, ShouldEqual, "test1")
			So(allResults[2].TaskId, ShouldEqual, "test3")
		})

	})
}

func TestSetDefaultsAndValidate(t *testing.T) {
	Convey("With various test parameters", t, func() {
		Convey("an empty test history parameters struct should not be valid", func() {
			params := TestHistoryParameters{}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct without a project should not be valid", func() {
			params := TestHistoryParameters{
				TestNames: []string{"blah"},
				TaskNames: []string{"blah"},
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct without a set of test or task names should not be valid", func() {
			params := TestHistoryParameters{
				Project: "p",
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct without a task status should have a default set", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TestStatuses: []string{evergreen.TestFailedStatus, evergreen.TestSucceededStatus},
				Limit:        10,
			}
			So(len(params.TaskStatuses), ShouldEqual, 0)
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			So(len(params.TaskStatuses), ShouldEqual, 1)
			So(params.TaskStatuses[0], ShouldEqual, evergreen.TaskFailed)

		})
		Convey("a test history parameters struct without a test status should have a default set", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TaskStatuses: []string{evergreen.TaskFailed},
				Limit:        10,
			}
			So(len(params.TestStatuses), ShouldEqual, 0)
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			So(len(params.TestStatuses), ShouldEqual, 1)
			So(params.TestStatuses[0], ShouldEqual, evergreen.TestFailedStatus)
		})
		Convey("a test history parameters struct with an invalid test status should not be valid", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TestStatuses: []string{"blah"},
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct with an invalid task status should not be valid", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TaskStatuses: []string{"blah"},
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct with both a date and revision should not be valid", func() {
			params := TestHistoryParameters{
				Project:       "project",
				TestNames:     []string{"test"},
				AfterRevision: "abc",
				BeforeDate:    time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:        "project",
				TestNames:      []string{"test"},
				BeforeRevision: "abc",
				BeforeDate:     time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:       "project",
				TestNames:     []string{"test"},
				AfterRevision: "abc",
				AfterDate:     time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:        "project",
				TestNames:      []string{"test"},
				BeforeRevision: "abc",
				AfterDate:      time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:        "project",
				TestNames:      []string{"test"},
				AfterRevision:  "abc",
				BeforeDate:     time.Now(),
				BeforeRevision: "abc",
				AfterDate:      time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})

	})
}

func TestBuildTestHistoryQuery(t *testing.T) {
	Convey("With a version", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, version.Collection),
			t, "Error clearing task collections")
		testVersion := version.Version{
			Id:                  "testVersion",
			Revision:            "abc",
			RevisionOrderNumber: 1,
			Identifier:          "project",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(testVersion.Insert(), ShouldBeNil)
		Convey("with setting the defaults of the test history parameters and having task names in the parameters", func() {
			params := TestHistoryParameters{
				Project:   "project",
				TaskNames: []string{"task1", "task2"},
				Limit:     20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			Convey("the pipeline created should be properly created with relevant fields", func() {
				pipeline, err := buildTestHistoryQuery(&params)
				So(err, ShouldBeNil)
				So(len(pipeline), ShouldEqual, 8)
				Convey("the $match task query should have task names included", func() {
					So(pipeline[0], ShouldContainKey, "$match")
					taskMatchQuery := bson.M{
						"$or": []bson.M{
							bson.M{
								task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
								task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
									"$ne": true,
								},
								task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
									"$ne": "system",
								}}},
						task.ProjectKey:     "project",
						task.DisplayNameKey: bson.M{"$in": []string{"task1", "task2"}},
					}
					So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
				})
			})
			Convey("with setting the test names, the task names and adding a build variant", func() {
				params.TestNames = []string{"test1"}
				params.TaskNames = []string{"task1", "task2"}
				params.BuildVariants = []string{"osx"}
				Convey("with adding in a before revision and validating", func() {
					params.BeforeRevision = "abc"
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have the test names included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}}},
								task.ProjectKey:             "project",
								task.DisplayNameKey:         bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:        bson.M{"$in": []string{"osx"}},
								task.RevisionOrderNumberKey: bson.M{"$lte": 1},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[4], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								testResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								testResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[4]["$match"], ShouldResemble, testMatchQuery)
						})
					})
				})
				Convey("with adding in an after revision and validating", func() {
					params.AfterRevision = "abc"
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have the test names included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}}},
								task.ProjectKey:             "project",
								task.DisplayNameKey:         bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:        bson.M{"$in": []string{"osx"}},
								task.RevisionOrderNumberKey: bson.M{"$gt": 1},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[4], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								testResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								testResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[4]["$match"], ShouldResemble, testMatchQuery)
						})
					})
				})
				Convey("with adding in a before and after revision and validating", func() {
					params.AfterRevision = "abc"
					params.BeforeRevision = "abc"
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have the test names included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}}},
								task.ProjectKey:             "project",
								task.DisplayNameKey:         bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:        bson.M{"$in": []string{"osx"}},
								task.RevisionOrderNumberKey: bson.M{"$lte": 1, "$gt": 1},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[4], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								testResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								testResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[4]["$match"], ShouldResemble, testMatchQuery)
						})
					})
				})
				Convey("with adding in an after date and validating", func() {
					now := time.Now()
					params.AfterDate = now
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have the test names included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}}},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
								task.StartTimeKey:    bson.M{"$gte": now},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[4], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								testResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								testResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[4]["$match"], ShouldResemble, testMatchQuery)
						})
					})
				})
				Convey("with adding in a before date and validating", func() {
					now := time.Now()
					params.BeforeDate = now
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have the test names included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}}},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
								task.StartTimeKey:    bson.M{"$lte": now},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[4], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								testResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								testResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[4]["$match"], ShouldResemble, testMatchQuery)
						})
					})
				})
				Convey("with adding in an after date and before date and validating", func() {
					now := time.Now()
					params.AfterDate = now
					params.BeforeDate = now
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have the test names included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}}},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
								task.StartTimeKey:    bson.M{"$gte": now, "$lte": now},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[4], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								testResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								testResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[4]["$match"], ShouldResemble, testMatchQuery)
						})
					})
				})
				Convey("with timeout in task status in the test history parameters", func() {
					params.TaskStatuses = []string{TaskTimeout}
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created without any errors", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have timeouts included", func() {
							So(pipeline[0], ShouldContainKey, "$match")

							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{
										task.StatusKey:                                     evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: true,
									}},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
					})
				})
				Convey("with system failure in task status in the test history parameters", func() {
					params.TaskStatuses = []string{TaskSystemFailure}
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created without any errors", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have timeouts included", func() {
							So(pipeline[0], ShouldContainKey, "$match")

							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{
										task.StatusKey:                                 evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailType: "system",
									}},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
					})
				})
				Convey("with system failure and task timeout in task status in the test history parameters", func() {
					params.TaskStatuses = []string{TaskSystemFailure, TaskTimeout}
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created without any errors", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have timeouts included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{
										task.StatusKey:                                     evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: true,
									},
									bson.M{
										task.StatusKey:                                 evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailType: "system",
									},
								},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
					})
				})
				Convey("with normal task statuses, system failure and task timeout in task status in the test history parameters", func() {
					params.TaskStatuses = []string{TaskSystemFailure, TaskTimeout, evergreen.TaskFailed}
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created without any errors", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 8)
						Convey("the $match task query should have timeouts included", func() {
							So(pipeline[0], ShouldContainKey, "$match")
							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{task.StatusKey: bson.M{"$in": []string{evergreen.TaskFailed}},
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
											"$ne": true,
										},
										task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
											"$ne": "system",
										}},
									bson.M{
										task.StatusKey:                                     evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: true,
									},
									bson.M{
										task.StatusKey:                                 evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailType: "system",
									},
								},
								task.ProjectKey:      "project",
								task.DisplayNameKey:  bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey: bson.M{"$in": []string{"osx"}},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
					})
				})
			})
		})
	})
}

func TestGetTestHistory(t *testing.T) {
	assert := assert.New(t)
	testFuncs := []func(*TestHistoryParameters) ([]TestHistoryResult, error){
		GetTestHistory,
		GetTestHistoryV2,
	}
	for _, testFunc := range testFuncs {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, version.Collection, testresult.Collection),
			t, "Error clearing task collections")
		project := "proj"
		now := time.Now()

		testVersion := version.Version{
			Id:                  "testVersion",
			Revision:            "fgh",
			RevisionOrderNumber: 1,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(testVersion.Insert())
		testVersion2 := version.Version{
			Id:                  "anotherVersion",
			Revision:            "def",
			RevisionOrderNumber: 2,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(testVersion2.Insert())
		testVersion3 := version.Version{
			Id:                  "testV",
			Revision:            "abcd",
			RevisionOrderNumber: 4,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(testVersion3.Insert())

		task1 := task.Task{
			Id:                  "task1",
			DisplayName:         "test",
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(task1.Insert())
		testresults1 := []testresult.TestResult{
			testresult.TestResult{
				TaskID:    task1.Id,
				Execution: task1.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
			},
			testresult.TestResult{
				TaskID:    task1.Id,
				Execution: task1.Execution,
				Status:    evergreen.TestSucceededStatus,
				TestFile:  "test2",
			},
		}
		assert.NoError(testresult.InsertMany(testresults1))
		task2 := task.Task{
			Id:                  "task2",
			DisplayName:         "test",
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now.Add(time.Duration(30 * time.Minute)),
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(task2.Insert())
		testresults2 := []testresult.TestResult{
			testresult.TestResult{
				TaskID:    task2.Id,
				Execution: task2.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
			},
			testresult.TestResult{
				TaskID:    task2.Id,
				Execution: task2.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test2",
			},
		}
		assert.NoError(testresult.InsertMany(testresults2))
		task3 := task.Task{
			Id:                  "task3",
			DisplayName:         "test2",
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
			LocalTestResults: []task.TestResult{
				task.TestResult{
					Status:   evergreen.TestFailedStatus,
					TestFile: "test1",
				},
				task.TestResult{
					Status:   evergreen.TestSucceededStatus,
					TestFile: "test3",
				},
				task.TestResult{
					Status:   evergreen.TestSilentlyFailedStatus,
					TestFile: "test4",
				},
			},
		}
		assert.NoError(task3.Insert())
		testresults3 := []testresult.TestResult{
			testresult.TestResult{
				TaskID:    task3.Id,
				Execution: task3.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
			},
			testresult.TestResult{
				TaskID:    task3.Id,
				Execution: task3.Execution,
				Status:    evergreen.TestSucceededStatus,
				TestFile:  "test3",
			},
			testresult.TestResult{
				TaskID:    task3.Id,
				Execution: task3.Execution,
				Status:    evergreen.TestSilentlyFailedStatus,
				TestFile:  "test4",
			},
		}
		assert.NoError(testresult.InsertMany(testresults3))
		// retrieving the task history with just a task name in the parameters should return relevant results
		params := TestHistoryParameters{
			TaskNames:    []string{"test"},
			Project:      project,
			Sort:         1,
			TaskStatuses: []string{evergreen.TaskFailed},
			TestStatuses: []string{evergreen.TestSucceededStatus, evergreen.TestFailedStatus},
			Limit:        20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err := testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 4)
		// the order of the test results should be in sorted order
		assert.Equal("task1", testResults[0].TaskId)
		assert.Equal("task1", testResults[1].TaskId)
		assert.Equal("task2", testResults[2].TaskId)
		assert.Equal("task2", testResults[3].TaskId)
		// with a sort of -1, the order should be in reverse revision order number order
		params.Sort = -1
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 4)
		//the order of the test results should be in reverse revision number order
		assert.Equal("task2", testResults[0].TaskId)
		assert.Equal("task1", testResults[3].TaskId)
		// retrieving the task history for just a set of test names in the parameters should return relevant results
		params = TestHistoryParameters{
			TestNames: []string{"test1"},
			Project:   project,
			Limit:     20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 3)
		// including a filter on a before revision should return inclusive results
		params = TestHistoryParameters{
			TaskNames:      []string{"test"},
			Project:        project,
			Sort:           1,
			BeforeRevision: testVersion2.Revision,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 3)
		assert.Equal("task1", testResults[0].TaskId)
		// including a filter on an after revision, should only return exclusive results
		params = TestHistoryParameters{
			TaskNames:     []string{"test"},
			Project:       project,
			Sort:          1,
			AfterRevision: testVersion.Revision,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 2)
		assert.Equal("task2", testResults[0].TaskId)
		// including a filter on both before and after revision should return relevant results
		params = TestHistoryParameters{
			TaskNames:      []string{"test"},
			Project:        project,
			Sort:           1,
			BeforeRevision: testVersion2.Revision,
			AfterRevision:  testVersion.Revision,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 2)
		assert.Equal("task2", testResults[0].TaskId)
		// including a filter on a before start time should return relevant results
		params = TestHistoryParameters{
			TaskNames:  []string{"test"},
			Project:    project,
			BeforeDate: now.Add(time.Duration(15 * time.Minute)),
			Limit:      20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 1)
		// including a filter on an after start time should return relevant results
		params = TestHistoryParameters{
			TaskNames: []string{"test"},
			Project:   project,
			AfterDate: now,
			Limit:     20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 3)
		// including a filter on test status of 'silentfail' should return relevant results
		params = TestHistoryParameters{
			Project:      project,
			TaskNames:    []string{"test2"},
			TestStatuses: []string{evergreen.TestSilentlyFailedStatus},
			Limit:        20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 1)
		// with a task with a different build variant
		anotherBV := task.Task{
			Id:                  "task5",
			DisplayName:         "test",
			BuildVariant:        "bv2",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(anotherBV.Insert())
		anotherBVresults := []testresult.TestResult{
			testresult.TestResult{
				TaskID:    anotherBV.Id,
				Execution: anotherBV.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
			},
			testresult.TestResult{
				TaskID:    anotherBV.Id,
				Execution: anotherBV.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test2",
			},
		}
		assert.NoError(testresult.InsertMany(anotherBVresults))
		// including a filter on build variant should only return test results with that build variant
		params = TestHistoryParameters{
			TaskNames:     []string{"test"},
			Project:       project,
			BuildVariants: []string{"bv2"},
			Limit:         20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 2)
		// not having the filter should return all results
		params = TestHistoryParameters{
			TaskNames: []string{"test"},
			Project:   project,
			Limit:     20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 5)
		// using a task with no test results
		noResults := task.Task{
			Id:                  "noResults",
			DisplayName:         "anothertest",
			BuildVariant:        "bv2",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(noResults.Insert())
		params = TestHistoryParameters{
			TaskNames: []string{"anothertest"},
			Project:   project,
			Limit:     20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Empty(testResults)
		// with tasks with different ordered test results
		diffOrder := task.Task{
			Id:                  "anotherTaskId",
			DisplayName:         "testTask",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(diffOrder.Insert())
		diffOrderResults := []testresult.TestResult{
			testresult.TestResult{
				TaskID:    diffOrder.Id,
				Execution: diffOrder.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test2",
			},
			testresult.TestResult{
				TaskID:    diffOrder.Id,
				Execution: diffOrder.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
			},
		}
		assert.NoError(testresult.InsertMany(diffOrderResults))
		diffOrder2 := task.Task{
			Id:                  "anotherTaskId2",
			DisplayName:         "testTask",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(diffOrder2.Insert())
		diffOrder2Results := []testresult.TestResult{
			testresult.TestResult{
				TaskID:    diffOrder2.Id,
				Execution: diffOrder2.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
			},
			testresult.TestResult{
				TaskID:    diffOrder2.Id,
				Execution: diffOrder2.Execution,
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test2",
			},
		}
		assert.NoError(testresult.InsertMany(diffOrder2Results))

		// the order of the tests should be the same
		params = TestHistoryParameters{
			TaskNames: []string{"testTask"},
			Project:   project,
			Limit:     20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 4)
		assert.Equal("anotherTaskId", testResults[0].TaskId)
		assert.Equal("test2", testResults[0].TestFile)
		assert.Equal("anotherTaskId", testResults[1].TaskId)
		assert.Equal("test1", testResults[1].TestFile)
		assert.Equal("anotherTaskId2", testResults[2].TaskId)
		assert.Equal("test2", testResults[2].TestFile)
		assert.Equal("anotherTaskId2", testResults[3].TaskId)
		assert.Equal("test1", testResults[3].TestFile)
		// using test parameter with a task status with timeouts
		timedOutTask := task.Task{
			Id:                  "timeout",
			DisplayName:         "test",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,

			Status: evergreen.TaskFailed,
			Details: apimodels.TaskEndDetail{
				TimedOut: true,
			},
		}
		assert.NoError(timedOutTask.Insert())
		timedOutResults := testresult.TestResult{
			TaskID:    timedOutTask.Id,
			Execution: timedOutTask.Execution,
			Status:    evergreen.TestFailedStatus,
			TestFile:  "test2",
		}
		assert.NoError(testresult.InsertMany([]testresult.TestResult{timedOutResults}))
		params = TestHistoryParameters{
			Project:      project,
			TaskNames:    []string{"test"},
			TaskStatuses: []string{TaskTimeout},
			Limit:        20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 1)
		// using test parameter with a task status with system failures
		systemFailureTask := task.Task{
			Id:                  "systemfailed",
			DisplayName:         "test",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,

			Status: evergreen.TaskFailed,
			Details: apimodels.TaskEndDetail{
				Type: "system",
			},
		}
		assert.NoError(systemFailureTask.Insert())
		systemFailureResult := testresult.TestResult{
			TaskID:    systemFailureTask.Id,
			Execution: systemFailureTask.Execution,
			Status:    evergreen.TestFailedStatus,
			TestFile:  "test2",
		}
		assert.NoError(testresult.InsertMany([]testresult.TestResult{systemFailureResult}))
		params = TestHistoryParameters{
			Project:      project,
			TaskNames:    []string{"test"},
			TaskStatuses: []string{TaskSystemFailure},
			Limit:        20,
		}
		assert.NoError(params.SetDefaultsAndValidate())
		testResults, err = testFunc(&params)
		assert.NoError(err)
		assert.Len(testResults, 1)
		assert.Equal("test2", testResults[0].TestFile)
		assert.Equal("system", testResults[0].TaskDetailsType)
	}
}

func TestCompareQueryRunTimes(t *testing.T) {
	assert := assert.New(t)
	rand.Seed(time.Now().UnixNano())
	numTasks := 1000  // # of tasks to insert into the db
	maxNumTests := 50 // max # of tests per task to insert (randomized per task)
	taskStatuses := []string{evergreen.TaskFailed, evergreen.TaskSucceeded}
	testStatuses := []string{evergreen.TestFailedStatus, evergreen.TestSucceededStatus, evergreen.TestSkippedStatus, evergreen.TestSilentlyFailedStatus}
	systemTypes := []string{"test", "system"}
	testutil.HandleTestingErr(db.ClearCollections(task.Collection, version.Collection, testresult.Collection),
		t, "Error clearing collections")
	project := "proj"
	now := time.Now()

	testVersion := version.Version{
		Id:                  "testVersion",
		Revision:            "fgh",
		RevisionOrderNumber: 1,
		Identifier:          project,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(testVersion.Insert())
	testVersion2 := version.Version{
		Id:                  "anotherVersion",
		Revision:            "def",
		RevisionOrderNumber: 2,
		Identifier:          project,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(testVersion2.Insert())
	testVersion3 := version.Version{
		Id:                  "testV",
		Revision:            "abcd",
		RevisionOrderNumber: 4,
		Identifier:          project,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(testVersion3.Insert())

	// insert tasks and tests
	for i := 0; i < numTasks; i++ {
		t := task.Task{
			Id:                  fmt.Sprintf("task_%d", i),
			DisplayName:         fmt.Sprintf("task_%d", i),
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: rand.Intn(100),
			Execution:           0,
			Status:              taskStatuses[rand.Intn(1)],
			Details: apimodels.TaskEndDetail{
				Type:     systemTypes[rand.Intn(1)],
				TimedOut: (rand.Intn(1) == 1),
			},
		}

		assert.NoError(t.Insert())
		numTests := rand.Intn(maxNumTests)
		tests := []testresult.TestResult{}
		for j := 0; j < numTests; j++ {
			tests = append(tests, testresult.TestResult{
				TaskID:    t.Id,
				Execution: t.Execution,
				TestFile:  fmt.Sprintf("test_%d", j),
				Status:    testStatuses[rand.Intn(3)],
			})
		}
		assert.NoError(testresult.InsertMany(tests))
	}

	// test querying on task names
	tasksToFind := []string{}
	for i := 0; i < numTasks; i++ {
		tasksToFind = append(tasksToFind, fmt.Sprintf("task_%d", i))
	}
	params := &TestHistoryParameters{
		TaskNames: tasksToFind,
		Project:   project,
		Sort:      1,
		Limit:     5000,
	}
	assert.NoError(params.SetDefaultsAndValidate())
	startTime := time.Now()
	resultsV1, err := GetTestHistory(params)
	elapsedV1 := time.Since(startTime)
	assert.NoError(err)
	startTime = time.Now()
	resultsV2, err := GetTestHistoryV2(params)
	elapsedV2 := time.Since(startTime)
	assert.NoError(err)
	assert.Equal(len(resultsV1), len(resultsV2))
	grip.Infof("elapsed time for aggregation test history query on task names: %s", elapsedV1.String())
	grip.Infof("elapsed time for non-aggregation test history query on task names: %s", elapsedV2.String())

	// test querying on test names
	testsToFind := []string{}
	for i := 0; i < maxNumTests; i++ {
		testsToFind = append(tasksToFind, fmt.Sprintf("test_%d", i))
	}
	params = &TestHistoryParameters{
		TestNames: testsToFind,
		Project:   project,
		Sort:      1,
		Limit:     5000,
	}
	assert.NoError(params.SetDefaultsAndValidate())
	startTime = time.Now()
	resultsV1, err = GetTestHistory(params)
	elapsedV1 = time.Since(startTime)
	assert.NoError(err)
	startTime = time.Now()
	resultsV2, err = GetTestHistoryV2(params)
	elapsedV2 = time.Since(startTime)
	assert.NoError(err)
	assert.Equal(len(resultsV1), len(resultsV2))
	grip.Infof("elapsed time for aggregation test history query on test names: %s", elapsedV1.String())
	grip.Infof("elapsed time for non-aggregation test history query on test names: %s", elapsedV2.String())

	testutil.HandleTestingErr(db.ClearCollections(task.Collection, version.Collection, testresult.Collection),
		t, "Error clearing collections")
}

func TestTaskHistoryPickaxe(t *testing.T) {
	testutil.HandleTestingErr(db.ClearCollections(task.Collection, testresult.Collection), t, "error clearing collections")
	assert := assert.New(t)
	proj := Project{
		Identifier: "proj",
	}
	t1 := task.Task{
		Id:                  "t1",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 1,
	}
	t2 := task.Task{
		Id:                  "t2",
		Project:             proj.Identifier,
		DisplayName:         "notMatchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 2,
	}
	t3 := task.Task{
		Id:                  "t3",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 3,
	}
	t4 := task.Task{
		Id:                  "t4",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 4,
	}
	assert.NoError(t1.Insert())
	assert.NoError(t2.Insert())
	assert.NoError(t3.Insert())
	assert.NoError(t4.Insert())
	r1 := testresult.TestResult{
		TaskID:   t1.Id,
		TestFile: "test",
		Status:   evergreen.TestFailedStatus,
	}
	r2 := testresult.TestResult{
		TaskID:   t2.Id,
		TestFile: "test",
		Status:   evergreen.TestFailedStatus,
	}
	r3 := testresult.TestResult{
		TaskID:   t3.Id,
		TestFile: "test",
		Status:   evergreen.TestFailedStatus,
	}
	r4 := testresult.TestResult{
		TaskID:   t4.Id,
		TestFile: "test",
		Status:   evergreen.TestFailedStatus,
	}
	assert.NoError(r1.Insert())
	assert.NoError(r2.Insert())
	assert.NoError(r3.Insert())
	assert.NoError(r4.Insert())

	// test that a basic case returns the correct results
	params := PickaxeParams{
		Project:       &proj,
		TaskName:      "matchingName",
		NewestOrder:   4,
		OldestOrder:   1,
		BuildVariants: []string{"bv"},
		Tests:         make(map[string]string),
	}
	params.Tests["test"] = evergreen.TestFailedStatus
	results, err := TaskHistoryPickaxe(params)
	assert.NoError(err)
	assert.Len(results, 3)
	for _, r := range results {
		assert.Equal("test", r.LocalTestResults[0].TestFile)
		assert.Equal(evergreen.TestFailedStatus, r.LocalTestResults[0].Status)
	}

	// test that a suite-style test result is found
	r5 := testresult.TestResult{
		TaskID:   t4.Id,
		TestFile: "foo/bar/test",
		Status:   evergreen.TestFailedStatus,
	}
	assert.NoError(r5.Insert())
	results, err = TaskHistoryPickaxe(params)
	assert.NoError(err)
	assert.Len(results, 3)
	for _, r := range results {
		if r.Id == t4.Id {
			assert.Len(r.LocalTestResults, 2)
		}
	}

	// test that the only matching tasks param works
	t5 := task.Task{
		Id:                  "t5",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 5,
	}
	assert.NoError(t5.Insert())
	params.NewestOrder = 5
	params.OnlyMatchingTasks = true
	results, err = TaskHistoryPickaxe(params)
	assert.NoError(err)
	assert.Len(results, 3)
}

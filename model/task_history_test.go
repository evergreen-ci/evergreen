package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
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
				So(len(pipeline), ShouldEqual, 7)
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
				Convey("the $project and $unwind bson.M should project the correct fields", func() {
					So(pipeline[1], ShouldContainKey, "$project")
					So(pipeline[1]["$project"], ShouldResemble, bson.M{
						task.DisplayNameKey:         1,
						task.BuildVariantKey:        1,
						task.ProjectKey:             1,
						task.StatusKey:              1,
						task.TestResultsKey:         1,
						task.RevisionKey:            1,
						task.IdKey:                  1,
						task.ExecutionKey:           1,
						task.RevisionOrderNumberKey: 1,
						task.OldTaskIdKey:           1,
						task.StartTimeKey:           1,
						task.DetailsKey:             1,
					})
					So(pipeline[2], ShouldContainKey, "$unwind")
					So(pipeline[2]["$unwind"], ShouldResemble, "$"+task.TestResultsKey)
				})
				Convey("the $match test query should only have a status query", func() {
					So(pipeline[3], ShouldContainKey, "$match")
					testMatchQuery := bson.M{
						task.TestResultsKey + "." + task.TestResultStatusKey: bson.M{"$in": []string{evergreen.TestFailedStatus}},
					}
					So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
				})
				Convey("the $sort and $project stages should have the correct fields", func() {
					So(pipeline[5], ShouldContainKey, "$sort")
					So(pipeline[5]["$sort"], ShouldResemble, bson.D{
						{Name: task.RevisionOrderNumberKey, Value: -1},
						{Name: task.TestResultsKey + "." + task.TestResultTestFileKey, Value: -1},
					})
					So(pipeline[6], ShouldContainKey, "$project")
					So(pipeline[6]["$project"], ShouldResemble, bson.M{
						TestFileKey:        "$" + task.TestResultsKey + "." + task.TestResultTestFileKey,
						TaskIdKey:          "$" + task.IdKey,
						TaskStatusKey:      "$" + task.StatusKey,
						TestStatusKey:      "$" + task.TestResultsKey + "." + task.TestResultStatusKey,
						RevisionKey:        "$" + task.RevisionKey,
						ProjectKey:         "$" + task.ProjectKey,
						TaskNameKey:        "$" + task.DisplayNameKey,
						BuildVariantKey:    "$" + task.BuildVariantKey,
						StartTimeKey:       "$" + task.TestResultsKey + "." + task.TestResultStartTimeKey,
						EndTimeKey:         "$" + task.TestResultsKey + "." + task.TestResultEndTimeKey,
						ExecutionKey:       "$" + task.ExecutionKey + "." + task.ExecutionKey,
						OldTaskIdKey:       "$" + task.OldTaskIdKey,
						UrlKey:             "$" + task.TestResultsKey + "." + task.TestResultURLKey,
						UrlRawKey:          "$" + task.TestResultsKey + "." + task.TestResultURLRawKey,
						TaskTimedOutKey:    "$" + task.DetailsKey + "." + task.TaskEndDetailTimedOut,
						TaskDetailsTypeKey: "$" + task.DetailsKey + "." + task.TaskEndDetailType,
						LogIdKey:           "$" + task.TestResultsKey + "." + task.TestResultLogIdKey,
					})
				})

			})
			Convey("with setting the test names and unsetting the task names", func() {
				params.TestNames = []string{"test1"}
				params.TaskNames = []string{}
				Convey("the pipeline should be created properly", func() {
					pipeline, err := buildTestHistoryQuery(&params)
					So(err, ShouldBeNil)
					So(len(pipeline), ShouldEqual, 7)
					Convey("the $match task query should have the test names included", func() {
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
									}}}, task.ProjectKey: "project",
							task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
						}
						So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
					})
					Convey("the $match test query should have the test names included", func() {
						So(pipeline[3], ShouldContainKey, "$match")
						testMatchQuery := bson.M{
							task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
							task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
						}
						So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
					})

				})
			})
			Convey("with setting the test names and the task names", func() {
				params.TestNames = []string{"test1"}
				params.TaskNames = []string{"task1", "task2"}
				Convey("the pipeline should be created properly", func() {
					pipeline, err := buildTestHistoryQuery(&params)
					So(err, ShouldBeNil)
					So(len(pipeline), ShouldEqual, 7)
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
							task.ProjectKey:                                        "project",
							task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
						}
						So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
					})
					Convey("the $match test query should have the test names included", func() {
						So(pipeline[3], ShouldContainKey, "$match")
						testMatchQuery := bson.M{
							task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
							task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
						}
						So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
					})

				})
			})
			Convey("with setting the test names, the task names and adding a build variant", func() {
				params.TestNames = []string{"test1"}
				params.TaskNames = []string{"task1", "task2"}
				params.BuildVariants = []string{"osx"}
				Convey("the pipeline should be created properly", func() {
					pipeline, err := buildTestHistoryQuery(&params)
					So(err, ShouldBeNil)
					So(len(pipeline), ShouldEqual, 7)
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
							task.ProjectKey:                                        "project",
							task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
							task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
						}
						So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
					})
					Convey("the $match test query should have the test names included", func() {
						So(pipeline[3], ShouldContainKey, "$match")
						testMatchQuery := bson.M{
							task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
							task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
						}
						So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
					})

				})
				Convey("with adding in a before revision and validating", func() {
					params.BeforeRevision = "abc"
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
								task.RevisionOrderNumberKey:                            bson.M{"$lte": 1},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[3], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
						})

					})

				})
				Convey("with adding in an after revision and validating", func() {
					params.AfterRevision = "abc"
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created properly", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
								task.RevisionOrderNumberKey:                            bson.M{"$gt": 1},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[3], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
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
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
								task.RevisionOrderNumberKey:                            bson.M{"$lte": 1, "$gt": 1},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[3], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
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
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
								task.StartTimeKey:                                      bson.M{"$gte": now},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[3], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
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
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
								task.StartTimeKey:                                      bson.M{"$lte": now},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[3], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
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
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
								task.StartTimeKey:                                      bson.M{"$gte": now, "$lte": now},
							}
							So(pipeline[0]["$match"], ShouldResemble, taskMatchQuery)
						})
						Convey("the $match test query should have the test names included", func() {
							So(pipeline[3], ShouldContainKey, "$match")
							testMatchQuery := bson.M{
								task.TestResultsKey + "." + task.TestResultStatusKey:   bson.M{"$in": []string{evergreen.TestFailedStatus}},
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
							}
							So(pipeline[3]["$match"], ShouldResemble, testMatchQuery)
						})

					})

				})
				Convey("with timeout in task status in the test history parameters", func() {
					params.TaskStatuses = []string{TaskTimeout}
					So(params.SetDefaultsAndValidate(), ShouldBeNil)
					Convey("the pipeline should be created without any errors", func() {
						pipeline, err := buildTestHistoryQuery(&params)
						So(err, ShouldBeNil)
						So(len(pipeline), ShouldEqual, 7)
						Convey("the $match task query should have timeouts included", func() {
							So(pipeline[0], ShouldContainKey, "$match")

							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{
										task.StatusKey:                                     evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailTimedOut: true,
									}},
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
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
						So(len(pipeline), ShouldEqual, 7)
						Convey("the $match task query should have timeouts included", func() {
							So(pipeline[0], ShouldContainKey, "$match")

							taskMatchQuery := bson.M{
								"$or": []bson.M{
									bson.M{
										task.StatusKey:                                 evergreen.TaskFailed,
										task.DetailsKey + "." + task.TaskEndDetailType: "system",
									}},
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
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
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
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
						So(len(pipeline), ShouldEqual, 7)
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
								task.ProjectKey:                                        "project",
								task.TestResultsKey + "." + task.TestResultTestFileKey: bson.M{"$in": []string{"test1"}},
								task.DisplayNameKey:                                    bson.M{"$in": []string{"task1", "task2"}},
								task.BuildVariantKey:                                   bson.M{"$in": []string{"osx"}},
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
	Convey("With a set of tasks and versions", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, version.Collection),
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
		So(testVersion.Insert(), ShouldBeNil)
		testVersion2 := version.Version{
			Id:                  "anotherVersion",
			Revision:            "def",
			RevisionOrderNumber: 2,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(testVersion2.Insert(), ShouldBeNil)
		testVersion3 := version.Version{
			Id:                  "testV",
			Revision:            "abcd",
			RevisionOrderNumber: 4,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(testVersion3.Insert(), ShouldBeNil)

		task1 := task.Task{
			Id:                  "task1",
			DisplayName:         "test",
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
			TestResults: []task.TestResult{
				task.TestResult{
					Status:   evergreen.TestFailedStatus,
					TestFile: "test1",
				},
				task.TestResult{
					Status:   evergreen.TestSucceededStatus,
					TestFile: "test2",
				},
			},
		}
		So(task1.Insert(), ShouldBeNil)
		task2 := task.Task{
			Id:                  "task2",
			DisplayName:         "test",
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now.Add(time.Duration(30 * time.Minute)),
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			TestResults: []task.TestResult{
				task.TestResult{
					Status:   evergreen.TestFailedStatus,
					TestFile: "test1",
				},
				task.TestResult{
					Status:   evergreen.TestFailedStatus,
					TestFile: "test2",
				},
			},
		}
		So(task2.Insert(), ShouldBeNil)

		task3 := task.Task{
			Id:                  "task3",
			DisplayName:         "test2",
			BuildVariant:        "osx",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
			TestResults: []task.TestResult{
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
		So(task3.Insert(), ShouldBeNil)

		Convey("retrieving the task history with just a task name in the parameters should return relevant results", func() {
			params := TestHistoryParameters{
				TaskNames:    []string{"test"},
				Project:      project,
				Sort:         1,
				TaskStatuses: []string{evergreen.TaskFailed},
				TestStatuses: []string{evergreen.TestSucceededStatus, evergreen.TestFailedStatus},
				Limit:        20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 4)
			Convey("the order of the test results should be in sorted order", func() {
				So(testResults[0].TaskId, ShouldEqual, "task1")
				So(testResults[1].TaskId, ShouldEqual, "task1")
				So(testResults[2].TaskId, ShouldEqual, "task2")
				So(testResults[3].TaskId, ShouldEqual, "task2")
			})
			Convey("with a sort of -1, the order should be in reverse revision order number order", func() {
				params.Sort = -1
				testResults, err := GetTestHistory(&params)
				So(err, ShouldBeNil)
				So(len(testResults), ShouldEqual, 4)
				Convey("the order of the test results should be in reverse revision number order", func() {
					So(testResults[0].TaskId, ShouldEqual, "task2")
					So(testResults[3].TaskId, ShouldEqual, "task1")
				})
			})
		})
		Convey("retrieving the task history for just a set of test names in the parameters should return relevant results", func() {
			params := TestHistoryParameters{
				TestNames: []string{"test1"},
				Project:   project,
				Limit:     20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 3)
		})
		Convey("including a filter on a before revision should return inclusive results", func() {
			params := TestHistoryParameters{
				TaskNames:      []string{"test"},
				Project:        project,
				Sort:           1,
				BeforeRevision: testVersion2.Revision,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 3)
			So(testResults[0].TaskId, ShouldEqual, "task1")
		})
		Convey("including a filter on an after revision, should only return exclusive results", func() {
			params := TestHistoryParameters{
				TaskNames:     []string{"test"},
				Project:       project,
				Sort:          1,
				AfterRevision: testVersion.Revision,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 2)
			So(testResults[0].TaskId, ShouldEqual, "task2")
		})
		Convey("including a filter on both before and after revision should return relevant results", func() {
			params := TestHistoryParameters{
				TaskNames:      []string{"test"},
				Project:        project,
				Sort:           1,
				BeforeRevision: testVersion2.Revision,
				AfterRevision:  testVersion.Revision,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 2)
			So(testResults[0].TaskId, ShouldEqual, "task2")
		})
		Convey("including a filter on a before start time should return relevant results", func() {
			params := TestHistoryParameters{
				TaskNames:  []string{"test"},
				Project:    project,
				BeforeDate: now.Add(time.Duration(15 * time.Minute)),
				Limit:      20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 1)

		})
		Convey("including a filter on an after start time should return relevant results", func() {
			params := TestHistoryParameters{
				TaskNames: []string{"test"},
				Project:   project,
				AfterDate: now,
				Limit:     20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 3)
		})
		Convey("including a filter on test status of 'silentfail' should return relevant results", func() {
			params := TestHistoryParameters{
				Project:      project,
				TaskNames:    []string{"test2"},
				TestStatuses: []string{evergreen.TestSilentlyFailedStatus},
				Limit:        20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 1)
		})
		Convey("with a task with a different build variant,", func() {
			anotherBV := task.Task{
				Id:                  "task5",
				DisplayName:         "test",
				BuildVariant:        "bv2",
				Project:             project,
				StartTime:           now,
				RevisionOrderNumber: 2,
				Status:              evergreen.TaskFailed,
				TestResults: []task.TestResult{
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test1",
					},
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test2",
					},
				},
			}
			So(anotherBV.Insert(), ShouldBeNil)
			Convey("including a filter on build variant should only return test results with that build variant", func() {
				params := TestHistoryParameters{
					TaskNames:     []string{"test"},
					Project:       project,
					BuildVariants: []string{"bv2"},
					Limit:         20,
				}
				So(params.SetDefaultsAndValidate(), ShouldBeNil)
				testResults, err := GetTestHistory(&params)
				So(err, ShouldBeNil)
				So(len(testResults), ShouldEqual, 2)
			})
			Convey("not having the filter should return all results", func() {
				params := TestHistoryParameters{
					TaskNames: []string{"test"},
					Project:   project,
					Limit:     20,
				}
				So(params.SetDefaultsAndValidate(), ShouldBeNil)
				testResults, err := GetTestHistory(&params)
				So(err, ShouldBeNil)
				So(len(testResults), ShouldEqual, 5)
			})
		})
		Convey("using a task with no test results", func() {
			noResults := task.Task{
				Id:                  "noResults",
				DisplayName:         "anothertest",
				BuildVariant:        "bv2",
				Project:             project,
				StartTime:           now,
				RevisionOrderNumber: 2,
				Status:              evergreen.TaskFailed,
			}
			So(noResults.Insert(), ShouldBeNil)
			params := TestHistoryParameters{
				TaskNames: []string{"anothertest"},
				Project:   project,
				Limit:     20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(testResults, ShouldBeEmpty)
		})
		Convey("with tasks with different ordered test results", func() {
			diffOrder := task.Task{
				Id:                  "anotherTaskId",
				DisplayName:         "testTask",
				Project:             project,
				StartTime:           now,
				RevisionOrderNumber: 2,
				Status:              evergreen.TaskFailed,
				TestResults: []task.TestResult{
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test2",
					},
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test1",
					},
				},
			}
			So(diffOrder.Insert(), ShouldBeNil)
			diffOrder2 := task.Task{
				Id:                  "anotherTaskId2",
				DisplayName:         "testTask",
				Project:             project,
				StartTime:           now,
				RevisionOrderNumber: 1,
				Status:              evergreen.TaskFailed,
				TestResults: []task.TestResult{
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test1",
					},
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test2",
					},
				},
			}
			So(diffOrder2.Insert(), ShouldBeNil)

			Convey("the order of the tests should be the same", func() {
				params := TestHistoryParameters{
					TaskNames: []string{"testTask"},
					Project:   project,
					Limit:     20,
				}
				So(params.SetDefaultsAndValidate(), ShouldBeNil)
				testResults, err := GetTestHistory(&params)
				So(err, ShouldBeNil)
				So(len(testResults), ShouldEqual, 4)
				So(testResults[0].TaskId, ShouldEqual, "anotherTaskId")
				So(testResults[0].TestFile, ShouldEqual, "test2")
				So(testResults[1].TaskId, ShouldEqual, "anotherTaskId")
				So(testResults[1].TestFile, ShouldEqual, "test1")
				So(testResults[2].TaskId, ShouldEqual, "anotherTaskId2")
				So(testResults[2].TestFile, ShouldEqual, "test2")
				So(testResults[3].TaskId, ShouldEqual, "anotherTaskId2")
				So(testResults[3].TestFile, ShouldEqual, "test1")

			})

		})
		Convey("using test parameter with a task status with timeouts", func() {
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
				TestResults: []task.TestResult{
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test2",
					},
				},
			}
			So(timedOutTask.Insert(), ShouldBeNil)
			params := TestHistoryParameters{
				Project:      project,
				TaskNames:    []string{"test"},
				TaskStatuses: []string{TaskTimeout},
				Limit:        20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 1)
		})
		Convey("using test parameter with a task status with system failures", func() {
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
				TestResults: []task.TestResult{
					task.TestResult{
						Status:   evergreen.TestFailedStatus,
						TestFile: "test2",
					},
				},
			}
			So(systemFailureTask.Insert(), ShouldBeNil)
			params := TestHistoryParameters{
				Project:      project,
				TaskNames:    []string{"test"},
				TaskStatuses: []string{TaskSystemFailure},
				Limit:        20,
			}
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			testResults, err := GetTestHistory(&params)
			So(err, ShouldBeNil)
			So(len(testResults), ShouldEqual, 1)
			So(testResults[0].TestFile, ShouldEqual, "test2")
			So(testResults[0].TaskDetailsType, ShouldEqual, "system")
		})

	})
}

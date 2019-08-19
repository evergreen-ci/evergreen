package cli

import (
	"fmt"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractResponse(t *testing.T) {
	const (
		errMsg = "fail"
		s1     = "foo"
		s2     = "bar"
		n1     = 1
		n2     = 2
	)
	for outcomeName, outcome := range map[string]OutcomeResponse{
		"Success": OutcomeResponse{
			Success: true,
		},
		"Unsuccessful": OutcomeResponse{
			Success: false,
			Message: errMsg,
		},
		"UnsuccessfulDefaultError": OutcomeResponse{
			Success: false,
		},
	} {
		t.Run(outcomeName, func(t *testing.T) {
			for testName, testCase := range map[string]struct {
				input           string
				extractAndCheck func(*testing.T, []byte)
			}{
				"OperationOutcome": {
					input: fmt.Sprintf(`{
						"success": %t,
						"message": "%s"
					}`, outcome.Success, outcome.Message),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractOutcomeResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}
					},
				},
				"InfoResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"info": {
						"id": "%s"
					}
					}`, outcome.Success, outcome.Message, s1),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractInfoResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						assert.Equal(t, s1, resp.Info.ID)
					},
				},
				"InfosResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"infos": [{
						"id": "%s"
					}, {
						"id": "%s"
					}]
					}`, outcome.Success, outcome.Message, s1, s2),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractInfosResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						info1, info2 := jasper.ProcessInfo{ID: s1}, jasper.ProcessInfo{ID: s2}
						info1Found, info2Found := false, false
						for _, info := range resp.Infos {
							if info.ID == info1.ID {
								info1Found = true
							}
							if info.ID == info2.ID {
								info2Found = true
							}
						}
						assert.True(t, info1Found && info2Found)
					},
				},
				"TagsResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"tags": ["%s", "%s"]
					}`, outcome.Success, outcome.Message, s1, s2),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractTagsResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						assert.Contains(t, resp.Tags, s1)
						assert.Contains(t, resp.Tags, s2)
					},
				},
				"WaitResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"exit_code": %d,
					"error": "%s"
					}`, outcome.Success, outcome.Message, n1, errMsg),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractWaitResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
							assert.Equal(t, n1, resp.ExitCode)
							assert.Contains(t, resp.Error, errMsg)
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}
					},
				},
				"RunningResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"running": %t
					}`, outcome.Success, outcome.Message, true),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractRunningResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						assert.True(t, resp.Running)
					},
				},
				"CompleteResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"complete": %t
					}`, outcome.Success, outcome.Message, true),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractCompleteResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						assert.True(t, resp.Complete)
					},
				},
				"ServiceStatusResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"status": "%s"
					}`, outcome.Success, outcome.Message, ServiceRunning),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractServiceStatusResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						assert.Equal(t, ServiceRunning, resp.Status)
					},
				},
				"LogStreamResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"log_stream": {
						"logs": ["%s"],
						"done": %t
					}
					}`, outcome.Success, outcome.Message, "foo", true),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractLogStreamResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						require.Len(t, resp.LogStream.Logs, 1)
						assert.Equal(t, "foo", resp.LogStream.Logs[0])
						assert.True(t, resp.LogStream.Done)
					},
				},
				"BuildloggerURLsResponse": {
					input: fmt.Sprintf(`{
					"outcome": {
						"success": %t,
						"message": "%s"
					},
					"urls": ["%s"]
					}`, outcome.Success, outcome.Message, "foo"),
					extractAndCheck: func(t *testing.T, input []byte) {
						resp, err := ExtractBuildloggerURLsResponse(input)
						if outcome.Success {
							require.NoError(t, err)
							assert.True(t, resp.Successful())
						} else {
							require.Error(t, err)
							assert.False(t, resp.Successful())

							if outcome.Message != "" {
								assert.Contains(t, resp.ErrorMessage(), outcome.Message)
							} else {
								assert.Contains(t, resp.ErrorMessage(), unspecifiedRequestFailure)
							}
						}

						require.Len(t, resp.URLs, 1)
						assert.Equal(t, "foo", resp.URLs[0])
					},
				},
			} {
				t.Run(testName, func(t *testing.T) {
					testCase.extractAndCheck(t, []byte(testCase.input))
				})
			}
		})
	}
}

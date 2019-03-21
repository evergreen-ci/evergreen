package message

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestPopulatedMessageComposerConstructors(t *testing.T) {
	const testMsg = "hello"
	assert := assert.New(t)
	// map objects to output
	cases := map[Composer]string{
		NewString(testMsg):                                                      testMsg,
		NewDefaultMessage(level.Error, testMsg):                                 testMsg,
		NewBytes([]byte(testMsg)):                                               testMsg,
		NewBytesMessage(level.Error, []byte(testMsg)):                           testMsg,
		NewError(errors.New(testMsg)):                                           testMsg,
		NewErrorMessage(level.Error, errors.New(testMsg)):                       testMsg,
		NewErrorWrap(errors.New(testMsg), ""):                                   testMsg,
		NewErrorWrapMessage(level.Error, errors.New(testMsg), ""):               testMsg,
		NewFormatted(string(testMsg[0])+"%s", testMsg[1:]):                      testMsg,
		NewFormattedMessage(level.Error, string(testMsg[0])+"%s", testMsg[1:]):  testMsg,
		WrapError(errors.New(testMsg), ""):                                      testMsg,
		WrapErrorf(errors.New(testMsg), ""):                                     testMsg,
		NewLine(testMsg, ""):                                                    testMsg,
		NewLineMessage(level.Error, testMsg, ""):                                testMsg,
		NewLine(testMsg):                                                        testMsg,
		NewLineMessage(level.Error, testMsg):                                    testMsg,
		MakeGroupComposer(NewString(testMsg)):                                   testMsg,
		NewGroupComposer([]Composer{NewString(testMsg)}):                        testMsg,
		MakeJiraMessage(JiraIssue{Summary: testMsg, Type: "Something"}):         testMsg,
		NewJiraMessage("", testMsg, JiraField{Key: "type", Value: "Something"}): testMsg,
		NewFieldsMessage(level.Error, testMsg, Fields{}):                        fmt.Sprintf("[message='%s']", testMsg),
		NewFields(level.Error, Fields{"test": testMsg}):                         fmt.Sprintf("[test='%s']", testMsg),
		MakeFieldsMessage(testMsg, Fields{}):                                    fmt.Sprintf("[message='%s']", testMsg),
		MakeFields(Fields{"test": testMsg}):                                     fmt.Sprintf("[test='%s']", testMsg),
		NewErrorWrappedComposer(errors.New("hello"), NewString("world")):        "world: hello",
		When(true, testMsg):                                                     testMsg,
		Whenf(true, testMsg):                                                    testMsg,
		Whenln(true, testMsg):                                                   testMsg,
		NewEmailMessage(level.Error, Email{
			Recipients: []string{"someone@example.com"},
			Subject:    "Test msg",
			Body:       testMsg,
		}): fmt.Sprintf("To: someone@example.com; Body: %s", testMsg),
		NewGithubStatusMessage(level.Error, "tests", GithubStateError, "https://example.com", testMsg): fmt.Sprintf("tests error: %s (https://example.com)", testMsg),
		NewGithubStatusMessageWithRepo(level.Error, GithubStatus{
			Owner: "mongodb",
			Repo:  "grip",
			Ref:   "master",

			Context:     "tests",
			State:       GithubStateError,
			URL:         "https://example.com",
			Description: testMsg,
		}): fmt.Sprintf("mongodb/grip@master tests error: %s (https://example.com)", testMsg),
		NewJIRACommentMessage(level.Error, "ABC-123", testMsg): testMsg,
		NewSlackMessage(level.Error, "@someone", testMsg, nil): fmt.Sprintf("@someone: %s", testMsg),
	}

	for msg, output := range cases {
		assert.NotNil(msg)
		assert.NotEmpty(output)
		assert.Implements((*Composer)(nil), msg)
		assert.True(msg.Loggable())
		assert.NotNil(msg.Raw())

		if strings.HasPrefix(output, "[") {
			output = strings.Trim(output, "[]")
			assert.True(strings.Contains(msg.String(), output))

		} else {
			// run the string test to make sure it doesn't change:
			assert.Equal(msg.String(), output, "%T", msg)
			assert.Equal(msg.String(), output, "%T", msg)
		}

		if msg.Priority() != level.Invalid {
			assert.Equal(msg.Priority(), level.Error)
		}

		// check message annotation functionality
		switch msg.(type) {
		case *GroupComposer:
			continue
		case *slackMessage:
			continue
		default:
			assert.NoError(msg.Annotate("k1", "foo"), "%T", msg)
			assert.Error(msg.Annotate("k1", "foo"), "%T", msg)
			assert.NoError(msg.Annotate("k2", "foo"), "%T", msg)
		}
	}
}

func TestUnpopulatedMessageComposers(t *testing.T) {
	assert := assert.New(t) // nolint
	// map objects to output
	cases := []Composer{
		&stringMessage{},
		NewString(""),
		NewDefaultMessage(level.Error, ""),
		&bytesMessage{},
		NewBytes([]byte{}),
		NewBytesMessage(level.Error, []byte{}),
		&ProcessInfo{},
		&SystemInfo{},
		&lineMessenger{},
		NewLine(),
		NewLineMessage(level.Error),
		&formatMessenger{},
		NewFormatted(""),
		NewFormattedMessage(level.Error, ""),
		NewStack(1, ""),
		NewStackLines(1),
		NewStackFormatted(1, ""),
		MakeGroupComposer(),
		&GroupComposer{},
		When(false, ""),
		Whenf(false, "", ""),
		Whenln(false, "", ""),
		NewEmailMessage(level.Error, Email{}),
		NewGithubStatusMessage(level.Error, "", GithubState(""), "", ""),
		NewGithubStatusMessageWithRepo(level.Error, GithubStatus{}),
		NewJIRACommentMessage(level.Error, "", ""),
		NewSlackMessage(level.Error, "", "", nil),
	}

	for idx, msg := range cases {
		assert.False(msg.Loggable(), "%d:%T", idx, msg)
	}
}

func TestDataCollecterComposerConstructors(t *testing.T) {
	const testMsg = "hello"
	assert := assert.New(t) // nolint
	// map objects to output (prefix)
	cases := map[Composer]string{
		NewProcessInfo(level.Error, int32(os.Getpid()), testMsg): "",
		NewSystemInfo(level.Error, testMsg):                      testMsg,
		MakeSystemInfo(testMsg):                                  testMsg,
		CollectProcessInfo(int32(1)):                             "",
		CollectProcessInfoSelf():                                 "",
		CollectSystemInfo():                                      "",
		CollectGoStats():                                         "",
	}

	for msg, prefix := range cases {
		assert.NotNil(msg)
		assert.NotNil(msg.Raw())
		assert.Implements((*Composer)(nil), msg)
		assert.True(msg.Loggable())
		assert.True(strings.HasPrefix(msg.String(), prefix), "%T: %s", msg, msg)
	}

	multiCases := [][]Composer{
		CollectProcessInfoSelfWithChildren(),
		CollectProcessInfoWithChildren(int32(1)),
		CollectAllProcesses(),
	}

	for _, group := range multiCases {
		assert.True(len(group) >= 1)
		for _, msg := range group {
			assert.NotNil(msg)
			assert.Implements((*Composer)(nil), msg)
			assert.NotEqual("", msg.String())
			assert.True(msg.Loggable())
		}
	}
}

func TestStackMessages(t *testing.T) {
	const testMsg = "hello"
	var stackMsg = "message/composer_test"

	assert := assert.New(t) // nolint
	// map objects to output (prefix)
	cases := map[Composer]string{
		NewStack(1, testMsg):                                       testMsg,
		NewStackLines(1, testMsg):                                  testMsg,
		NewStackLines(1):                                           "",
		NewStackFormatted(1, "%s", testMsg):                        testMsg,
		NewStackFormatted(1, string(testMsg[0])+"%s", testMsg[1:]): testMsg,

		// with 0 frame
		NewStack(0, testMsg):                                       testMsg,
		NewStackLines(0, testMsg):                                  testMsg,
		NewStackLines(0):                                           "",
		NewStackFormatted(0, "%s", testMsg):                        testMsg,
		NewStackFormatted(0, string(testMsg[0])+"%s", testMsg[1:]): testMsg,
	}

	for msg, text := range cases {
		assert.NotNil(msg)
		assert.Implements((*Composer)(nil), msg)
		assert.NotNil(msg.Raw())
		if text != "" {
			assert.True(msg.Loggable())
		}

		diagMsg := fmt.Sprintf("%T: %+v", msg, msg)
		assert.True(strings.Contains(msg.String(), text), diagMsg)
		assert.True(strings.Contains(msg.String(), stackMsg), diagMsg)
	}
}

func TestComposerConverter(t *testing.T) {
	const testMsg = "hello world"
	assert := assert.New(t) // nolint

	cases := []interface{}{
		NewLine(testMsg),
		testMsg,
		errors.New(testMsg),
		[]string{testMsg},
		[]interface{}{testMsg},
		[]byte(testMsg),
		[]Composer{NewString(testMsg)},
	}

	for _, msg := range cases {
		comp := ConvertToComposer(level.Error, msg)
		assert.True(comp.Loggable())
		assert.Equal(testMsg, comp.String(), "%T", msg)
	}

	cases = []interface{}{
		nil,
		"",
		[]interface{}{},
		[]string{},
		[]byte{},
		Fields{},
		map[string]interface{}{},
	}

	for _, msg := range cases {
		comp := ConvertToComposer(level.Error, msg)
		assert.False(comp.Loggable())
		assert.Equal("", comp.String(), "%T", msg)
	}

	outputCases := map[string]interface{}{
		"1":            1,
		"2":            int32(2),
		"[message='3'": Fields{"message": 3},
		"[message='4'": map[string]interface{}{"message": "4"},
	}

	for out, in := range outputCases {
		comp := ConvertToComposer(level.Error, in)
		assert.True(comp.Loggable())
		assert.True(strings.HasPrefix(comp.String(), out))
	}

}

func TestJiraMessageComposerConstructor(t *testing.T) {
	const testMsg = "hello"
	assert := assert.New(t) // nolint
	reporterField := JiraField{Key: "Reporter", Value: "Annie"}
	assigneeField := JiraField{Key: "Assignee", Value: "Sejin"}
	typeField := JiraField{Key: "Type", Value: "Bug"}
	labelsField := JiraField{Key: "Labels", Value: []string{"Soul", "Pop"}}
	unknownField := JiraField{Key: "Artist", Value: "Adele"}
	msg := NewJiraMessage("project", testMsg, reporterField, assigneeField, typeField, labelsField, unknownField)
	issue := msg.Raw().(JiraIssue)

	assert.Equal(issue.Project, "project")
	assert.Equal(issue.Summary, testMsg)
	assert.Equal(issue.Reporter, reporterField.Value)
	assert.Equal(issue.Assignee, assigneeField.Value)
	assert.Equal(issue.Type, typeField.Value)
	assert.Equal(issue.Labels, labelsField.Value)
	assert.Equal(issue.Fields[unknownField.Key], unknownField.Value)
}

func TestProcessTreeDoesNotHaveDuplicates(t *testing.T) {
	assert := assert.New(t) // nolint

	procs := CollectProcessInfoWithChildren(1)
	seen := make(map[int32]struct{})

	for _, p := range procs {
		pinfo, ok := p.(*ProcessInfo)
		assert.True(ok)
		seen[pinfo.Pid] = struct{}{}
	}

	assert.Equal(len(seen), len(procs))
}

func TestJiraIssueAnnotationOnlySupportsStrings(t *testing.T) {
	assert := assert.New(t) // nolint

	m := &jiraMessage{}

	assert.Error(m.Annotate("k", 1))
	assert.Error(m.Annotate("k", true))
	assert.Error(m.Annotate("k", nil))
}

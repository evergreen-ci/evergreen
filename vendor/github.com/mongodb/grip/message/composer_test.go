package message

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"strings"

	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestMessageComposerConstructors(t *testing.T) {
	const testMsg = "hello"
	assert := assert.New(t)
	// map objects to output
	cases := map[Composer]string{
		NewString(testMsg):                                                     testMsg,
		NewDefaultMessage(level.Error, testMsg):                                testMsg,
		NewBytes([]byte(testMsg)):                                              testMsg,
		NewBytesMessage(level.Error, []byte(testMsg)):                          testMsg,
		NewError(errors.New(testMsg)):                                          testMsg,
		NewErrorMessage(level.Error, errors.New(testMsg)):                      testMsg,
		NewErrorWrap(errors.New(testMsg), ""):                                  testMsg,
		NewErrorWrapMessage(level.Error, errors.New(testMsg), ""):              testMsg,
		NewFieldsMessage(level.Error, testMsg, Fields{}):                       fmt.Sprintf("[msg='%s']", testMsg),
		NewFields(level.Error, Fields{"test": testMsg}):                        fmt.Sprintf("[test='%s']", testMsg),
		MakeFieldsMessage(testMsg, Fields{}):                                   fmt.Sprintf("[msg='%s']", testMsg),
		MakeFields(Fields{"test": testMsg}):                                    fmt.Sprintf("[test='%s']", testMsg),
		NewFormatted(string(testMsg[0])+"%s", testMsg[1:]):                     testMsg,
		NewFormattedMessage(level.Error, string(testMsg[0])+"%s", testMsg[1:]): testMsg,
		NewLine(testMsg, ""):                                                   testMsg,
		NewLineMessage(level.Error, testMsg, ""):                               testMsg,
		NewLine(testMsg):                                                       testMsg,
		NewLineMessage(level.Error, testMsg):                                   testMsg,
	}

	for msg, output := range cases {
		assert.NotNil(msg)
		assert.NotEmpty(output)
		assert.Implements((*Composer)(nil), msg)
		assert.Equal(msg.String(), output)

		if msg.Priority() != level.Invalid {
			assert.Equal(msg.Priority(), level.Error)
		}
	}
}

func TestDataCollecterComposerConstructors(t *testing.T) {
	const testMsg = "hello"
	assert := assert.New(t)
	// map objects to output (prefix)
	cases := map[Composer]string{
		NewProcessInfo(level.Error, int32(os.Getpid()), testMsg): "",
		NewSystemInfo(level.Error, testMsg):                      testMsg,
		MakeSystemInfo(testMsg):                                  testMsg,
	}

	for msg, prefix := range cases {
		assert.NotNil(msg)
		assert.Implements((*Composer)(nil), msg)

		assert.True(strings.HasPrefix(msg.String(), prefix), fmt.Sprintf("%T: %s", msg, msg))
	}
}

func TestStackMessages(t *testing.T) {
	const testMsg = "hello"
	const stackMsg = "message/composer_test"
	assert := assert.New(t)
	// map objects to output (prefix)
	cases := map[Composer]string{
		NewStack(1, testMsg):                                       testMsg,
		NewStackLines(1, testMsg):                                  testMsg,
		NewStackLines(1):                                           "",
		NewStackFormatted(1, "%s", testMsg):                        testMsg,
		NewStackFormatted(1, string(testMsg[0])+"%s", testMsg[1:]): testMsg,
	}

	for msg, text := range cases {
		assert.NotNil(msg)
		assert.Implements((*Composer)(nil), msg)

		diagMsg := fmt.Sprintf("%T: %+v", msg, msg)
		assert.True(strings.Contains(msg.String(), text), diagMsg)
		assert.True(strings.Contains(msg.String(), stackMsg), diagMsg)
	}
}

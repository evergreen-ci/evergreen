package message

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tychoish/grip/level"
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

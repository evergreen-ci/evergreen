package taskrunner

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorHandlerIntegration(t *testing.T) {
	assert := assert.New(t) // nolint

	// confirm the global error collector is initialized.
	assert.NotNil(errorCollector)
	assert.Len(errorCollector.cache, 0)

	ec := newErrorCollector()
	assert.NotNil(ec)
	assert.Len(ec.cache, 0)

	// nil error should get ignored
	ec.add("", "", "", nil)
	assert.Len(ec.cache, 0)

	// non-nil error should be captured but not lead to an error.
	ec.add("", "", "", errors.New("foo"))
	assert.Len(ec.cache, 1)
	assert.NoError(ec.report())

	// push the error count on this "host" and make sure it produces an error
	for i := 0; i < 4; i++ {
		ec.add("", "", "", errors.New("foo"))
		assert.Len(ec.cache, 1)
	}
	err := ec.report()

	assert.Error(err)

	assert.Contains(err.Error(), "foo")
	errParts := strings.Split(err.Error(), "\n")
	count := 0
	for _, str := range errParts {
		str = strings.Trim(str, "\n\t\r ")
		if str == "foo" {
			count++
		}
	}
	assert.Len(ec.cache, 0)

	assert.Equal(5, count, "%s", errParts)

	// rerun the test but pass a nil error at the end, which is
	// like a host recovering, and therefore clearing the error
	for i := 0; i < 4; i++ {
		ec.add("", "", "", errors.New("foo"))
		assert.Len(ec.cache, 1)
	}
	assert.Len(ec.cache, 1)

	ec.add("", "", "", nil)
	assert.Len(ec.cache, 0)

}

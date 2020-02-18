package amboy

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestDuplicateError(t *testing.T) {
	assert.False(t, IsDuplicateJobError(errors.New("err")))
	assert.False(t, IsDuplicateJobError(nil))
	assert.True(t, IsDuplicateJobError(NewDuplicateJobError("err")))
	assert.True(t, IsDuplicateJobError(NewDuplicateJobErrorf("err")))
	assert.True(t, IsDuplicateJobError(NewDuplicateJobErrorf("err %s", "err")))
	assert.True(t, IsDuplicateJobError(MakeDuplicateJobError(errors.New("err"))))
}

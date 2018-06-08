package trigger

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistryItemsAreSane(t *testing.T) {
	registry.lock.RLock()
	defer registry.lock.RUnlock()

	assert := assert.New(t)

	for k, v := range registry.handlers {
		handler := v()
		assert.NotNil(handler, "handler factory for '%s' returned nil (%s)", k, runtime.FuncForPC(reflect.ValueOf(v).Pointer()).Name())
	}
}

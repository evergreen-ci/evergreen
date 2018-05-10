package trigger

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistryItemsAreSane(t *testing.T) {
	registry.lock.RLock()
	defer registry.lock.RUnlock()

	assert := assert.New(t)

	for k, v := range registry.triggers {
		set := map[reflect.Value]bool{}

		assert.NotEmpty(v)

		for i := range v {
			f := reflect.ValueOf(v[i])
			_, ok := set[f]
			assert.False(ok, fmt.Sprintf("triggers for '%s' has a duplicate function: '%s' index: %d", k, runtime.FuncForPC(f.Pointer()).Name(), i))
			set[f] = true

			_, ok = registry.prefetch[k]
			assert.True(ok, "missing prefetch function for resource type: '%s'", k)
		}
	}
}

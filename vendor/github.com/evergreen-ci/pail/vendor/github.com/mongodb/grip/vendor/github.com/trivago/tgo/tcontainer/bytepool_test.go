// Copyright 2015-2016 trivago GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tcontainer

import (
	"github.com/trivago/tgo/ttesting"
	//"runtime"
	"testing"
	//"time"
)

func TestBytePool(t *testing.T) {
	expect := ttesting.NewExpect(t)
	pool := NewBytePool()

	tinyMin := pool.Get(1)
	expect.Equal(tiny, cap(tinyMin))
	expect.Equal(1, len(tinyMin))

	tinyMax := pool.Get(small - tiny)
	expect.Equal(small-tiny, cap(tinyMax))
	expect.Equal(small-tiny, len(tinyMax))

	smallMin := pool.Get(small - tiny + 1)
	expect.Equal(small, cap(smallMin))
	expect.Equal(small-tiny+1, len(smallMin))

	smallMax := pool.Get(medium - small)
	expect.Equal(medium-small, cap(smallMax))
	expect.Equal(medium-small, len(smallMax))

	mediumMin := pool.Get(medium - small + 1)
	expect.Equal(medium, cap(mediumMin))
	expect.Equal(medium-small+1, len(mediumMin))

	mediumMax := pool.Get(large - medium)
	expect.Equal(large-medium, cap(mediumMax))
	expect.Equal(large-medium, len(mediumMax))

	largeMin := pool.Get(large - medium + 1)
	expect.Equal(large, cap(largeMin))
	expect.Equal(large-medium+1, len(largeMin))

	largeMax := pool.Get(huge - large)
	expect.Equal(huge-large, cap(largeMax))
	expect.Equal(huge-large, len(largeMax))

	hugeMin := pool.Get(huge - large + 1)
	expect.Equal(huge-large+1, cap(hugeMin))
	expect.Equal(huge-large+1, len(hugeMin))
}

/*func TestBytePool(t *testing.T) {
	expect := ttesting.NewExpect(t)
	pool := NewBytePool()

	tinyMin := pool.Get(1)
	expect.Equal(pool.tiny.unit, cap(tinyMin))
	expect.Equal(1, len(tinyMin))

	tinyMax := pool.Get(pool.tiny.max)
	expect.Equal(pool.tiny.max, cap(tinyMax))
	expect.Equal(pool.tiny.max, len(tinyMax))

	smallMin := pool.Get(pool.tiny.max + 1)
	expect.Equal(pool.small.unit, cap(smallMin))
	expect.Equal(pool.tiny.max+1, len(smallMin))

	smallMax := pool.Get(pool.small.max)
	expect.Equal(pool.small.max, cap(smallMax))
	expect.Equal(pool.small.max, len(smallMax))

	mediumMin := pool.Get(pool.small.max + 1)
	expect.Equal(pool.medium.unit, cap(mediumMin))
	expect.Equal(pool.small.max+1, len(mediumMin))

	mediumMax := pool.Get(pool.medium.max)
	expect.Equal(pool.medium.max, cap(mediumMax))
	expect.Equal(pool.medium.max, len(mediumMax))

	largeMin := pool.Get(pool.medium.max + 1)
	expect.Equal(pool.large.unit, cap(largeMin))
	expect.Equal(pool.medium.max+1, len(largeMin))

	largeMax := pool.Get(pool.large.max)
	expect.Equal(pool.large.max, cap(largeMax))
	expect.Equal(pool.large.max, len(largeMax))

	huge := pool.Get(pool.large.max + 1)
	expect.Equal(pool.large.max+1, cap(huge))
	expect.Equal(pool.large.max+1, len(huge))
}

func allocateWaste(pool *BytePool, expect ttesting.Expect) {
	data := pool.Get(32)
	for i := 0; i < 32; i++ {
		data[i] = byte(i)
	}

	expect.Equal(int32(-1), *pool.tiny.slabs[0].top)
}

func TestBytePoolRecycle(t *testing.T) {
	expect := ttesting.NewExpect(t)
	pool := NewBytePool()

	expect.Nil(pool.tiny.slabs[0])
	allocateWaste(&pool, expect)

	expect.NonBlocking(time.Second, func() {
		for *pool.tiny.slabs[0].top < 0 {
			runtime.Gosched()
			runtime.GC()
		}
	})

	expect.Equal(int32(0), *pool.tiny.slabs[0].top)

	data := pool.Get(32)
	for i := 0; i < 32; i++ {
		expect.Equal(byte(i), data[i])
	}
}*/

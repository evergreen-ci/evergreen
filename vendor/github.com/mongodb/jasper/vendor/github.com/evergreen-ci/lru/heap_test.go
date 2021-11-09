package lru

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// the fileObjectHeap is just a sliced back FILO object, that
// implements go's "container/heap" interface, which actually does
// provides the heap heap behavior/operations. These are just some
// simple tests of the underlying structure.

func TestFileObjectHeap(t *testing.T) {
	assert := assert.New(t)

	var fileHeap fileObjectHeap

	assert.Equal(len(fileHeap), fileHeap.Len())
	assert.Equal(0, fileHeap.Len())

	smaller := &FileObject{Time: time.Now().Add(-300 * time.Second), Path: "smaller", index: 1}
	larger := &FileObject{Time: time.Now(), Path: "larger", index: 0}

	fileHeap.Push(smaller)

	assert.Equal(len(fileHeap), fileHeap.Len())
	assert.Equal(1, fileHeap.Len())

	fileHeap.Push(larger)

	assert.Equal(len(fileHeap), fileHeap.Len())
	assert.Equal(2, fileHeap.Len())

	first := fileHeap.Pop().(*FileObject)

	assert.Equal(len(fileHeap), fileHeap.Len())
	assert.Equal(1, fileHeap.Len())

	second := fileHeap.Pop().(*FileObject)

	assert.Equal(len(fileHeap), fileHeap.Len())
	assert.Equal(0, fileHeap.Len())

	assert.Equal(first.Path, "larger", fmt.Sprintf("%+v", first))
	assert.Equal(second.Path, "smaller", fmt.Sprintf("%+v", second))
}

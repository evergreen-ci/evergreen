package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPassingButInAnotherFile(t *testing.T) {
	t.Parallel()
	t.Log("start")
	assert := assert.New(t)
	assert.True(true)
	time.Sleep(10 * time.Second)
	t.Log("end")
}
func TestFailingButInAnotherFile(t *testing.T) {
	t.Parallel()
	t.Log("start")
	assert := assert.New(t)
	assert.True(false)
	time.Sleep(10 * time.Second)
	t.Log("end")
}

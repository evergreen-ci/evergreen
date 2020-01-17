package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFoo(t *testing.T) {
	assert := assert.New(t)
	utcLoc, err := time.LoadLocation("")
	assert.NoError(err)
	TestTimeAsString := "\"2017-04-06T19:53:46.404Z\""
	TestTimeAsTime := time.Date(2017, time.April, 6, 19, 53, 46, 404000000, utcLoc)
	res, err := json.Marshal(TestTimeAsTime)
	assert.NoError(err)
	assert.EqualValues(TestTimeAsString, string(res))

	var mTime time.Time
	err = json.Unmarshal(res, &mTime)
	assert.NoError(err)
	assert.EqualValues(mTime, TestTimeAsTime)
}

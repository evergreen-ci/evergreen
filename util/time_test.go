package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeJitter(t *testing.T) {
	assert := assert.New(t)

	for i := 0; i < 100; i++ {
		t := JitterInterval(time.Second * 15)
		assert.True(t >= 15*time.Second)
		assert.True(t <= 30*time.Second)
	}
}

func TestTimeRoundPartMinute(t *testing.T) {
	assert := assert.New(t)

	// make sure the fixtures work:
	for i := 0; i < 60; i++ {
		assert.Equal(i, getTimeWithMin(i).Minute())
	}

	type timeParts struct {
		expectedValue int
		actualMins    int
		interval      int
	}

	cases := []timeParts{
		{0, 12, 30},
		{0, 13, 40},
		{0, 48, 40},
		{0, 27, 31},
		{2, 3, 2},
		{4, 4, 2},
		{4, 5, 2},
		{5, 8, 5},
		{5, 9, 5},
		{10, 10, 5},
		{10, 12, 10},
		{10, 15, 10},
		{10, 18, 10},
		{15, 16, 5},
		{20, 20, 20},
		{20, 27, 20},
		{20, 28, 10},
		{24, 25, 2},
		{30, 48, 30},
	}

	for _, c := range cases {
		assert.Equal(c.expectedValue, findPartMin(getTimeWithMin(c.actualMins), c.interval).Minute(),
			fmt.Sprintf("%+v", c))
	}
}

func TestTimeRoundPartSecond(t *testing.T) {
	assert := assert.New(t)

	// make sure the fixtures work:
	for i := 0; i < 60; i++ {
		assert.Equal(i, getTimeWithSec(i).Second())
	}

	type timeParts struct {
		expectedValue int
		actualSecs    int
		interval      int
	}

	cases := []timeParts{
		{0, 12, 30},
		{0, 13, 40},
		{0, 48, 40},
		{0, 27, 31},
		{2, 3, 2},
		{4, 4, 2},
		{4, 5, 2},
		{5, 8, 5},
		{5, 9, 5},
		{10, 10, 5},
		{10, 12, 10},
		{10, 15, 10},
		{10, 18, 10},
		{15, 16, 5},
		{20, 20, 20},
		{20, 27, 20},
		{20, 28, 10},
		{24, 25, 2},
		{30, 48, 30},
	}

	for _, c := range cases {
		assert.Equal(c.expectedValue, findPartSec(getTimeWithSec(c.actualSecs), c.interval).Second(),
			fmt.Sprintf("%+v", c))
	}
}

func getTimeWithMin(min int) time.Time {
	now := time.Now()

	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), min, 0, 0, time.UTC)
}

func getTimeWithSec(sec int) time.Time {
	now := time.Now()

	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), sec, 0, time.UTC)
}

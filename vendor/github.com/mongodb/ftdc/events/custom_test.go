package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestRollupRoundTrip(t *testing.T) {
	data := MakeCustom(4)
	assert.NoError(t, data.Add("a", 1.2))
	assert.NoError(t, data.Add("f", 100))
	assert.NoError(t, data.Add("b", 45.0))
	assert.NoError(t, data.Add("d", []int64{45, 32}))
	assert.Error(t, data.Add("foo", Custom{}))
	assert.Len(t, data, 4)

	t.Run("NewBSON", func(t *testing.T) {
		payload, err := bson.Marshal(data)
		require.NoError(t, err)

		rt := Custom{}
		err = bson.Unmarshal(payload, &rt)
		require.NoError(t, err)

		require.Len(t, rt, 4)
		assert.Equal(t, "a", rt[0].Name)
		assert.Equal(t, "b", rt[1].Name)
		assert.Equal(t, "d", rt[2].Name)
		assert.Equal(t, "f", rt[3].Name)
		assert.Equal(t, 1.2, rt[0].Value)
		assert.Equal(t, 45.0, rt[1].Value)
		assert.EqualValues(t, 100, rt[3].Value)
	})
}

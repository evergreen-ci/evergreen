package bsonx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireErrEqual(t *testing.T, err1 error, err2 error) {
	if err1 != nil && err2 != nil {
		require.Equal(t, err1.Error(), err2.Error())
		return
	}

	require.Equal(t, err1, err2)
}

func elementSliceEqual(t *testing.T, e1 []*Element, e2 []*Element) {
	require.Equal(t, len(e1), len(e2))

	for i := range e1 {
		require.True(t, readerElementComparer(e1[i], e2[i]))
	}
}

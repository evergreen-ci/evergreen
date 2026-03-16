package task

import (
	"context"
	"strings"
	"testing"

	"github.com/evergreen-ci/pail"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsContextError(t *testing.T) {
	for name, tc := range map[string]struct {
		err      error
		expected bool
	}{
		"DeadlineExceeded":           {context.DeadlineExceeded, true},
		"Canceled":                   {context.Canceled, true},
		"WrappedDeadlineExceeded":    {errors.Wrap(context.DeadlineExceeded, "wrap"), true},
		"WrappedCanceled":            {errors.Wrap(context.Canceled, "wrap"), true},
		"MessageContainsDeadline":    {errors.New("operation error S3: CopyObject, context deadline exceeded"), true},
		"MessageContainsCanceled":    {errors.New("request canceled"), true},
		"Nil":                        {nil, false},
		"OtherError":                 {errors.New("some other error"), false},
		"MessageContainsDeadlineSub": {errors.New("context deadline exceeded"), true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isContextError(tc.err))
		})
	}
}

func TestAllObjectsExistInBucket(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: tmpDir, UseSlash: true})
	require.NoError(t, err)

	require.NoError(t, bucket.Put(ctx, "key1", strings.NewReader("data1")))
	require.NoError(t, bucket.Put(ctx, "key2", strings.NewReader("data2")))

	for name, tc := range map[string]struct {
		keys     []string
		expected bool
	}{
		"AllExist":     {[]string{"key1", "key2"}, true},
		"OneExists":    {[]string{"key1"}, true},
		"EmptyKeys":    {[]string{}, true},
		"MissingKey":   {[]string{"key1", "key2", "key3"}, false},
		"AllMissing":   {[]string{"key3", "key4"}, false},
		"FirstMissing": {[]string{"key3", "key1"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, allObjectsExistInBucket(ctx, bucket, tc.keys))
		})
	}
}

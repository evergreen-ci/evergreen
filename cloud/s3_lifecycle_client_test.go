package cloud

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestS3LifecycleClient_ValidationErrors(t *testing.T) {
	client := NewS3LifecycleClient()
	ctx := context.Background()

	// Test empty bucket name
	rules, err := client.GetBucketLifecycleConfiguration(ctx, "", "us-east-1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bucket name cannot be empty")
	assert.Nil(t, rules)

	// Test empty region
	rules, err = client.GetBucketLifecycleConfiguration(ctx, "test-bucket", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "region cannot be empty")
	assert.Nil(t, rules)
}

func TestS3LifecycleClient_Interface(t *testing.T) {
	// Verify the client implements the interface
	client := NewS3LifecycleClient()
	assert.NotNil(t, client)

	// Verify the client has the expected type
	impl, ok := client.(*s3LifecycleClientImpl)
	assert.True(t, ok)
	assert.NotNil(t, impl)
}

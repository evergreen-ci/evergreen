package cloud

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeAWSLogMessageRedactsUserData(t *testing.T) {
	const secretUserData = "c29tZS1ob3N0LXNlY3JldA==" //nolint:gosec // Not a real secret.

	t.Run("NestedUserDataInLaunchTemplateIsRedacted", func(t *testing.T) {
		input := &ec2.CreateLaunchTemplateInput{
			LaunchTemplateName: aws.String("template"),
			LaunchTemplateData: &types.RequestLaunchTemplateData{
				UserData: aws.String(secretUserData),
			},
		}
		msg := makeAWSLogMessage("CreateLaunchTemplate", "client", input)

		args, ok := msg["args"].(map[string]any)
		require.True(t, ok)
		launchTemplateData, ok := args["LaunchTemplateData"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "{REDACTED}", launchTemplateData["UserData"])
		require.Contains(t, args, "LaunchTemplateName")
	})

	t.Run("TopLevelUserDataInRunInstancesIsRedacted", func(t *testing.T) {
		input := &ec2.RunInstancesInput{
			UserData:     aws.String(secretUserData),
			InstanceType: types.InstanceTypeT2Micro,
		}
		msg := makeAWSLogMessage("RunInstances", "client", input)

		args, ok := msg["args"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "{REDACTED}", args["UserData"])
		require.Contains(t, args, "InstanceType")
	})
}

func TestMakeCreateKeyPairInputAppliesMongoDBResourceTags(t *testing.T) {
	resourceTags := evergreen.ResourceTagsConfig{
		MongoDBOwner: "evergreen@mongodb.com",
		MongoDBEnv:   "prod",
	}
	input := makeCreateKeyPairInput("key_name", resourceTags)

	assert.Equal(t, "key_name", aws.ToString(input.KeyName))
	require.Len(t, input.TagSpecifications, 1)
	assert.Equal(t, types.ResourceTypeKeyPair, input.TagSpecifications[0].ResourceType)
	tagsByKey := map[string]string{}
	for _, tag := range input.TagSpecifications[0].Tags {
		tagsByKey[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	assert.Equal(t, resourceTags.MongoDBOwner, tagsByKey[evergreen.TagMongoDBOwner])
	assert.Equal(t, resourceTags.MongoDBEnv, tagsByKey[evergreen.TagMongoDBEnv])
}

func TestMakeCreateKeyPairInputOmitsEmptyMongoDBResourceTags(t *testing.T) {
	input := makeCreateKeyPairInput("key_name", evergreen.ResourceTagsConfig{})
	assert.Empty(t, input.TagSpecifications)
}

// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package mediatailoriface provides an interface to enable mocking the AWS MediaTailor service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package mediatailoriface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/mediatailor"
)

// MediaTailorAPI provides an interface to enable mocking the
// mediatailor.MediaTailor service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS MediaTailor.
//    func myFunc(svc mediatailoriface.MediaTailorAPI) bool {
//        // Make svc.CreateChannel request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := mediatailor.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockMediaTailorClient struct {
//        mediatailoriface.MediaTailorAPI
//    }
//    func (m *mockMediaTailorClient) CreateChannel(input *mediatailor.CreateChannelInput) (*mediatailor.CreateChannelOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockMediaTailorClient{}
//
//        myfunc(mockSvc)
//
//        // Verify myFunc's functionality
//    }
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters. Its suggested to use the pattern above for testing, or using
// tooling to generate mocks to satisfy the interfaces.
type MediaTailorAPI interface {
	CreateChannel(*mediatailor.CreateChannelInput) (*mediatailor.CreateChannelOutput, error)
	CreateChannelWithContext(aws.Context, *mediatailor.CreateChannelInput, ...request.Option) (*mediatailor.CreateChannelOutput, error)
	CreateChannelRequest(*mediatailor.CreateChannelInput) (*request.Request, *mediatailor.CreateChannelOutput)

	CreateProgram(*mediatailor.CreateProgramInput) (*mediatailor.CreateProgramOutput, error)
	CreateProgramWithContext(aws.Context, *mediatailor.CreateProgramInput, ...request.Option) (*mediatailor.CreateProgramOutput, error)
	CreateProgramRequest(*mediatailor.CreateProgramInput) (*request.Request, *mediatailor.CreateProgramOutput)

	CreateSourceLocation(*mediatailor.CreateSourceLocationInput) (*mediatailor.CreateSourceLocationOutput, error)
	CreateSourceLocationWithContext(aws.Context, *mediatailor.CreateSourceLocationInput, ...request.Option) (*mediatailor.CreateSourceLocationOutput, error)
	CreateSourceLocationRequest(*mediatailor.CreateSourceLocationInput) (*request.Request, *mediatailor.CreateSourceLocationOutput)

	CreateVodSource(*mediatailor.CreateVodSourceInput) (*mediatailor.CreateVodSourceOutput, error)
	CreateVodSourceWithContext(aws.Context, *mediatailor.CreateVodSourceInput, ...request.Option) (*mediatailor.CreateVodSourceOutput, error)
	CreateVodSourceRequest(*mediatailor.CreateVodSourceInput) (*request.Request, *mediatailor.CreateVodSourceOutput)

	DeleteChannel(*mediatailor.DeleteChannelInput) (*mediatailor.DeleteChannelOutput, error)
	DeleteChannelWithContext(aws.Context, *mediatailor.DeleteChannelInput, ...request.Option) (*mediatailor.DeleteChannelOutput, error)
	DeleteChannelRequest(*mediatailor.DeleteChannelInput) (*request.Request, *mediatailor.DeleteChannelOutput)

	DeleteChannelPolicy(*mediatailor.DeleteChannelPolicyInput) (*mediatailor.DeleteChannelPolicyOutput, error)
	DeleteChannelPolicyWithContext(aws.Context, *mediatailor.DeleteChannelPolicyInput, ...request.Option) (*mediatailor.DeleteChannelPolicyOutput, error)
	DeleteChannelPolicyRequest(*mediatailor.DeleteChannelPolicyInput) (*request.Request, *mediatailor.DeleteChannelPolicyOutput)

	DeletePlaybackConfiguration(*mediatailor.DeletePlaybackConfigurationInput) (*mediatailor.DeletePlaybackConfigurationOutput, error)
	DeletePlaybackConfigurationWithContext(aws.Context, *mediatailor.DeletePlaybackConfigurationInput, ...request.Option) (*mediatailor.DeletePlaybackConfigurationOutput, error)
	DeletePlaybackConfigurationRequest(*mediatailor.DeletePlaybackConfigurationInput) (*request.Request, *mediatailor.DeletePlaybackConfigurationOutput)

	DeleteProgram(*mediatailor.DeleteProgramInput) (*mediatailor.DeleteProgramOutput, error)
	DeleteProgramWithContext(aws.Context, *mediatailor.DeleteProgramInput, ...request.Option) (*mediatailor.DeleteProgramOutput, error)
	DeleteProgramRequest(*mediatailor.DeleteProgramInput) (*request.Request, *mediatailor.DeleteProgramOutput)

	DeleteSourceLocation(*mediatailor.DeleteSourceLocationInput) (*mediatailor.DeleteSourceLocationOutput, error)
	DeleteSourceLocationWithContext(aws.Context, *mediatailor.DeleteSourceLocationInput, ...request.Option) (*mediatailor.DeleteSourceLocationOutput, error)
	DeleteSourceLocationRequest(*mediatailor.DeleteSourceLocationInput) (*request.Request, *mediatailor.DeleteSourceLocationOutput)

	DeleteVodSource(*mediatailor.DeleteVodSourceInput) (*mediatailor.DeleteVodSourceOutput, error)
	DeleteVodSourceWithContext(aws.Context, *mediatailor.DeleteVodSourceInput, ...request.Option) (*mediatailor.DeleteVodSourceOutput, error)
	DeleteVodSourceRequest(*mediatailor.DeleteVodSourceInput) (*request.Request, *mediatailor.DeleteVodSourceOutput)

	DescribeChannel(*mediatailor.DescribeChannelInput) (*mediatailor.DescribeChannelOutput, error)
	DescribeChannelWithContext(aws.Context, *mediatailor.DescribeChannelInput, ...request.Option) (*mediatailor.DescribeChannelOutput, error)
	DescribeChannelRequest(*mediatailor.DescribeChannelInput) (*request.Request, *mediatailor.DescribeChannelOutput)

	DescribeProgram(*mediatailor.DescribeProgramInput) (*mediatailor.DescribeProgramOutput, error)
	DescribeProgramWithContext(aws.Context, *mediatailor.DescribeProgramInput, ...request.Option) (*mediatailor.DescribeProgramOutput, error)
	DescribeProgramRequest(*mediatailor.DescribeProgramInput) (*request.Request, *mediatailor.DescribeProgramOutput)

	DescribeSourceLocation(*mediatailor.DescribeSourceLocationInput) (*mediatailor.DescribeSourceLocationOutput, error)
	DescribeSourceLocationWithContext(aws.Context, *mediatailor.DescribeSourceLocationInput, ...request.Option) (*mediatailor.DescribeSourceLocationOutput, error)
	DescribeSourceLocationRequest(*mediatailor.DescribeSourceLocationInput) (*request.Request, *mediatailor.DescribeSourceLocationOutput)

	DescribeVodSource(*mediatailor.DescribeVodSourceInput) (*mediatailor.DescribeVodSourceOutput, error)
	DescribeVodSourceWithContext(aws.Context, *mediatailor.DescribeVodSourceInput, ...request.Option) (*mediatailor.DescribeVodSourceOutput, error)
	DescribeVodSourceRequest(*mediatailor.DescribeVodSourceInput) (*request.Request, *mediatailor.DescribeVodSourceOutput)

	GetChannelPolicy(*mediatailor.GetChannelPolicyInput) (*mediatailor.GetChannelPolicyOutput, error)
	GetChannelPolicyWithContext(aws.Context, *mediatailor.GetChannelPolicyInput, ...request.Option) (*mediatailor.GetChannelPolicyOutput, error)
	GetChannelPolicyRequest(*mediatailor.GetChannelPolicyInput) (*request.Request, *mediatailor.GetChannelPolicyOutput)

	GetChannelSchedule(*mediatailor.GetChannelScheduleInput) (*mediatailor.GetChannelScheduleOutput, error)
	GetChannelScheduleWithContext(aws.Context, *mediatailor.GetChannelScheduleInput, ...request.Option) (*mediatailor.GetChannelScheduleOutput, error)
	GetChannelScheduleRequest(*mediatailor.GetChannelScheduleInput) (*request.Request, *mediatailor.GetChannelScheduleOutput)

	GetChannelSchedulePages(*mediatailor.GetChannelScheduleInput, func(*mediatailor.GetChannelScheduleOutput, bool) bool) error
	GetChannelSchedulePagesWithContext(aws.Context, *mediatailor.GetChannelScheduleInput, func(*mediatailor.GetChannelScheduleOutput, bool) bool, ...request.Option) error

	GetPlaybackConfiguration(*mediatailor.GetPlaybackConfigurationInput) (*mediatailor.GetPlaybackConfigurationOutput, error)
	GetPlaybackConfigurationWithContext(aws.Context, *mediatailor.GetPlaybackConfigurationInput, ...request.Option) (*mediatailor.GetPlaybackConfigurationOutput, error)
	GetPlaybackConfigurationRequest(*mediatailor.GetPlaybackConfigurationInput) (*request.Request, *mediatailor.GetPlaybackConfigurationOutput)

	ListChannels(*mediatailor.ListChannelsInput) (*mediatailor.ListChannelsOutput, error)
	ListChannelsWithContext(aws.Context, *mediatailor.ListChannelsInput, ...request.Option) (*mediatailor.ListChannelsOutput, error)
	ListChannelsRequest(*mediatailor.ListChannelsInput) (*request.Request, *mediatailor.ListChannelsOutput)

	ListChannelsPages(*mediatailor.ListChannelsInput, func(*mediatailor.ListChannelsOutput, bool) bool) error
	ListChannelsPagesWithContext(aws.Context, *mediatailor.ListChannelsInput, func(*mediatailor.ListChannelsOutput, bool) bool, ...request.Option) error

	ListPlaybackConfigurations(*mediatailor.ListPlaybackConfigurationsInput) (*mediatailor.ListPlaybackConfigurationsOutput, error)
	ListPlaybackConfigurationsWithContext(aws.Context, *mediatailor.ListPlaybackConfigurationsInput, ...request.Option) (*mediatailor.ListPlaybackConfigurationsOutput, error)
	ListPlaybackConfigurationsRequest(*mediatailor.ListPlaybackConfigurationsInput) (*request.Request, *mediatailor.ListPlaybackConfigurationsOutput)

	ListPlaybackConfigurationsPages(*mediatailor.ListPlaybackConfigurationsInput, func(*mediatailor.ListPlaybackConfigurationsOutput, bool) bool) error
	ListPlaybackConfigurationsPagesWithContext(aws.Context, *mediatailor.ListPlaybackConfigurationsInput, func(*mediatailor.ListPlaybackConfigurationsOutput, bool) bool, ...request.Option) error

	ListSourceLocations(*mediatailor.ListSourceLocationsInput) (*mediatailor.ListSourceLocationsOutput, error)
	ListSourceLocationsWithContext(aws.Context, *mediatailor.ListSourceLocationsInput, ...request.Option) (*mediatailor.ListSourceLocationsOutput, error)
	ListSourceLocationsRequest(*mediatailor.ListSourceLocationsInput) (*request.Request, *mediatailor.ListSourceLocationsOutput)

	ListSourceLocationsPages(*mediatailor.ListSourceLocationsInput, func(*mediatailor.ListSourceLocationsOutput, bool) bool) error
	ListSourceLocationsPagesWithContext(aws.Context, *mediatailor.ListSourceLocationsInput, func(*mediatailor.ListSourceLocationsOutput, bool) bool, ...request.Option) error

	ListTagsForResource(*mediatailor.ListTagsForResourceInput) (*mediatailor.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *mediatailor.ListTagsForResourceInput, ...request.Option) (*mediatailor.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*mediatailor.ListTagsForResourceInput) (*request.Request, *mediatailor.ListTagsForResourceOutput)

	ListVodSources(*mediatailor.ListVodSourcesInput) (*mediatailor.ListVodSourcesOutput, error)
	ListVodSourcesWithContext(aws.Context, *mediatailor.ListVodSourcesInput, ...request.Option) (*mediatailor.ListVodSourcesOutput, error)
	ListVodSourcesRequest(*mediatailor.ListVodSourcesInput) (*request.Request, *mediatailor.ListVodSourcesOutput)

	ListVodSourcesPages(*mediatailor.ListVodSourcesInput, func(*mediatailor.ListVodSourcesOutput, bool) bool) error
	ListVodSourcesPagesWithContext(aws.Context, *mediatailor.ListVodSourcesInput, func(*mediatailor.ListVodSourcesOutput, bool) bool, ...request.Option) error

	PutChannelPolicy(*mediatailor.PutChannelPolicyInput) (*mediatailor.PutChannelPolicyOutput, error)
	PutChannelPolicyWithContext(aws.Context, *mediatailor.PutChannelPolicyInput, ...request.Option) (*mediatailor.PutChannelPolicyOutput, error)
	PutChannelPolicyRequest(*mediatailor.PutChannelPolicyInput) (*request.Request, *mediatailor.PutChannelPolicyOutput)

	PutPlaybackConfiguration(*mediatailor.PutPlaybackConfigurationInput) (*mediatailor.PutPlaybackConfigurationOutput, error)
	PutPlaybackConfigurationWithContext(aws.Context, *mediatailor.PutPlaybackConfigurationInput, ...request.Option) (*mediatailor.PutPlaybackConfigurationOutput, error)
	PutPlaybackConfigurationRequest(*mediatailor.PutPlaybackConfigurationInput) (*request.Request, *mediatailor.PutPlaybackConfigurationOutput)

	StartChannel(*mediatailor.StartChannelInput) (*mediatailor.StartChannelOutput, error)
	StartChannelWithContext(aws.Context, *mediatailor.StartChannelInput, ...request.Option) (*mediatailor.StartChannelOutput, error)
	StartChannelRequest(*mediatailor.StartChannelInput) (*request.Request, *mediatailor.StartChannelOutput)

	StopChannel(*mediatailor.StopChannelInput) (*mediatailor.StopChannelOutput, error)
	StopChannelWithContext(aws.Context, *mediatailor.StopChannelInput, ...request.Option) (*mediatailor.StopChannelOutput, error)
	StopChannelRequest(*mediatailor.StopChannelInput) (*request.Request, *mediatailor.StopChannelOutput)

	TagResource(*mediatailor.TagResourceInput) (*mediatailor.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *mediatailor.TagResourceInput, ...request.Option) (*mediatailor.TagResourceOutput, error)
	TagResourceRequest(*mediatailor.TagResourceInput) (*request.Request, *mediatailor.TagResourceOutput)

	UntagResource(*mediatailor.UntagResourceInput) (*mediatailor.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *mediatailor.UntagResourceInput, ...request.Option) (*mediatailor.UntagResourceOutput, error)
	UntagResourceRequest(*mediatailor.UntagResourceInput) (*request.Request, *mediatailor.UntagResourceOutput)

	UpdateChannel(*mediatailor.UpdateChannelInput) (*mediatailor.UpdateChannelOutput, error)
	UpdateChannelWithContext(aws.Context, *mediatailor.UpdateChannelInput, ...request.Option) (*mediatailor.UpdateChannelOutput, error)
	UpdateChannelRequest(*mediatailor.UpdateChannelInput) (*request.Request, *mediatailor.UpdateChannelOutput)

	UpdateSourceLocation(*mediatailor.UpdateSourceLocationInput) (*mediatailor.UpdateSourceLocationOutput, error)
	UpdateSourceLocationWithContext(aws.Context, *mediatailor.UpdateSourceLocationInput, ...request.Option) (*mediatailor.UpdateSourceLocationOutput, error)
	UpdateSourceLocationRequest(*mediatailor.UpdateSourceLocationInput) (*request.Request, *mediatailor.UpdateSourceLocationOutput)

	UpdateVodSource(*mediatailor.UpdateVodSourceInput) (*mediatailor.UpdateVodSourceOutput, error)
	UpdateVodSourceWithContext(aws.Context, *mediatailor.UpdateVodSourceInput, ...request.Option) (*mediatailor.UpdateVodSourceOutput, error)
	UpdateVodSourceRequest(*mediatailor.UpdateVodSourceInput) (*request.Request, *mediatailor.UpdateVodSourceOutput)
}

var _ MediaTailorAPI = (*mediatailor.MediaTailor)(nil)

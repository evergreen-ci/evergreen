// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package resourcegroups

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/aws/aws-sdk-go/private/protocol/restjson"
)

// ResourceGroups provides the API operation methods for making requests to
// AWS Resource Groups. See this package's package overview docs
// for details on the service.
//
// ResourceGroups methods are safe to use concurrently. It is not safe to
// modify mutate any of the struct's properties though.
type ResourceGroups struct {
	*client.Client
}

// Used for custom client initialization logic
var initClient func(*client.Client)

// Used for custom request initialization logic
var initRequest func(*request.Request)

// Service information constants
const (
	ServiceName = "resource-groups" // Service endpoint prefix API calls made to.
	EndpointsID = ServiceName       // Service ID for Regions and Endpoints metadata.
)

// New creates a new instance of the ResourceGroups client with a session.
// If additional configuration is needed for the client instance use the optional
// aws.Config parameter to add your extra config.
//
// Example:
//     // Create a ResourceGroups client from just a session.
//     svc := resourcegroups.New(mySession)
//
//     // Create a ResourceGroups client with additional configuration
//     svc := resourcegroups.New(mySession, aws.NewConfig().WithRegion("us-west-2"))
func New(p client.ConfigProvider, cfgs ...*aws.Config) *ResourceGroups {
	c := p.ClientConfig(EndpointsID, cfgs...)
	return newClient(*c.Config, c.Handlers, c.Endpoint, c.SigningRegion, c.SigningName)
}

// newClient creates, initializes and returns a new service client instance.
func newClient(cfg aws.Config, handlers request.Handlers, endpoint, signingRegion, signingName string) *ResourceGroups {
	if len(signingName) == 0 {
		signingName = "resource-groups"
	}
	svc := &ResourceGroups{
		Client: client.New(
			cfg,
			metadata.ClientInfo{
				ServiceName:   ServiceName,
				SigningName:   signingName,
				SigningRegion: signingRegion,
				Endpoint:      endpoint,
				APIVersion:    "2017-11-27",
			},
			handlers,
		),
	}

	// Handlers
	svc.Handlers.Sign.PushBackNamed(v4.SignRequestHandler)
	svc.Handlers.Build.PushBackNamed(restjson.BuildHandler)
	svc.Handlers.Unmarshal.PushBackNamed(restjson.UnmarshalHandler)
	svc.Handlers.UnmarshalMeta.PushBackNamed(restjson.UnmarshalMetaHandler)
	svc.Handlers.UnmarshalError.PushBackNamed(restjson.UnmarshalErrorHandler)

	// Run custom client initialization if present
	if initClient != nil {
		initClient(svc.Client)
	}

	return svc
}

// newRequest creates a new request for a ResourceGroups operation and runs any
// custom request initialization.
func (c *ResourceGroups) newRequest(op *request.Operation, params, data interface{}) *request.Request {
	req := c.NewRequest(op, params, data)

	// Run custom request initialization if present
	if initRequest != nil {
		initRequest(req)
	}

	return req
}

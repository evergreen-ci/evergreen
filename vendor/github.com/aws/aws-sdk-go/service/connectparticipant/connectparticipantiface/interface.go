// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package connectparticipantiface provides an interface to enable mocking the Amazon Connect Participant Service service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package connectparticipantiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/connectparticipant"
)

// ConnectParticipantAPI provides an interface to enable mocking the
// connectparticipant.ConnectParticipant service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon Connect Participant Service.
//    func myFunc(svc connectparticipantiface.ConnectParticipantAPI) bool {
//        // Make svc.CreateParticipantConnection request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := connectparticipant.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockConnectParticipantClient struct {
//        connectparticipantiface.ConnectParticipantAPI
//    }
//    func (m *mockConnectParticipantClient) CreateParticipantConnection(input *connectparticipant.CreateParticipantConnectionInput) (*connectparticipant.CreateParticipantConnectionOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockConnectParticipantClient{}
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
type ConnectParticipantAPI interface {
	CreateParticipantConnection(*connectparticipant.CreateParticipantConnectionInput) (*connectparticipant.CreateParticipantConnectionOutput, error)
	CreateParticipantConnectionWithContext(aws.Context, *connectparticipant.CreateParticipantConnectionInput, ...request.Option) (*connectparticipant.CreateParticipantConnectionOutput, error)
	CreateParticipantConnectionRequest(*connectparticipant.CreateParticipantConnectionInput) (*request.Request, *connectparticipant.CreateParticipantConnectionOutput)

	DisconnectParticipant(*connectparticipant.DisconnectParticipantInput) (*connectparticipant.DisconnectParticipantOutput, error)
	DisconnectParticipantWithContext(aws.Context, *connectparticipant.DisconnectParticipantInput, ...request.Option) (*connectparticipant.DisconnectParticipantOutput, error)
	DisconnectParticipantRequest(*connectparticipant.DisconnectParticipantInput) (*request.Request, *connectparticipant.DisconnectParticipantOutput)

	GetTranscript(*connectparticipant.GetTranscriptInput) (*connectparticipant.GetTranscriptOutput, error)
	GetTranscriptWithContext(aws.Context, *connectparticipant.GetTranscriptInput, ...request.Option) (*connectparticipant.GetTranscriptOutput, error)
	GetTranscriptRequest(*connectparticipant.GetTranscriptInput) (*request.Request, *connectparticipant.GetTranscriptOutput)

	GetTranscriptPages(*connectparticipant.GetTranscriptInput, func(*connectparticipant.GetTranscriptOutput, bool) bool) error
	GetTranscriptPagesWithContext(aws.Context, *connectparticipant.GetTranscriptInput, func(*connectparticipant.GetTranscriptOutput, bool) bool, ...request.Option) error

	SendEvent(*connectparticipant.SendEventInput) (*connectparticipant.SendEventOutput, error)
	SendEventWithContext(aws.Context, *connectparticipant.SendEventInput, ...request.Option) (*connectparticipant.SendEventOutput, error)
	SendEventRequest(*connectparticipant.SendEventInput) (*request.Request, *connectparticipant.SendEventOutput)

	SendMessage(*connectparticipant.SendMessageInput) (*connectparticipant.SendMessageOutput, error)
	SendMessageWithContext(aws.Context, *connectparticipant.SendMessageInput, ...request.Option) (*connectparticipant.SendMessageOutput, error)
	SendMessageRequest(*connectparticipant.SendMessageInput) (*request.Request, *connectparticipant.SendMessageOutput)
}

var _ ConnectParticipantAPI = (*connectparticipant.ConnectParticipant)(nil)

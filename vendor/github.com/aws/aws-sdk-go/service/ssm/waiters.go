// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package ssm

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
)

// WaitUntilCommandExecuted uses the Amazon SSM API operation
// GetCommandInvocation to wait for a condition to be met before returning.
// If the condition is not met within the max attempt window, an error will
// be returned.
func (c *SSM) WaitUntilCommandExecuted(input *GetCommandInvocationInput) error {
	return c.WaitUntilCommandExecutedWithContext(aws.BackgroundContext(), input)
}

// WaitUntilCommandExecutedWithContext is an extended version of WaitUntilCommandExecuted.
// With the support for passing in a context and options to configure the
// Waiter and the underlying request options.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *SSM) WaitUntilCommandExecutedWithContext(ctx aws.Context, input *GetCommandInvocationInput, opts ...request.WaiterOption) error {
	w := request.Waiter{
		Name:        "WaitUntilCommandExecuted",
		MaxAttempts: 20,
		Delay:       request.ConstantWaiterDelay(5 * time.Second),
		Acceptors: []request.WaiterAcceptor{
			{
				State:   request.RetryWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "Pending",
			},
			{
				State:   request.RetryWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "InProgress",
			},
			{
				State:   request.RetryWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "Delayed",
			},
			{
				State:   request.SuccessWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "Success",
			},
			{
				State:   request.FailureWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "Cancelled",
			},
			{
				State:   request.FailureWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "TimedOut",
			},
			{
				State:   request.FailureWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "Failed",
			},
			{
				State:   request.FailureWaiterState,
				Matcher: request.PathWaiterMatch, Argument: "Status",
				Expected: "Cancelling",
			},
		},
		Logger: c.Config.Logger,
		NewRequest: func(opts []request.Option) (*request.Request, error) {
			var inCpy *GetCommandInvocationInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.GetCommandInvocationRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}
	w.ApplyOptions(opts...)

	return w.WaitWithContext(ctx)
}

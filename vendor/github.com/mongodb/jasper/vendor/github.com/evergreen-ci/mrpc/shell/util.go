package shell

import (
	"context"
	"io"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// WriteResponse sends a response the the writer output.
func WriteResponse(ctx context.Context, w io.Writer, resp mongowire.Message, op string) {
	grip.Error(message.WrapError(mongowire.SendMessage(ctx, resp, w), message.Fields{
		"message": "could not write response",
		"op":      op,
	}))
}

// WriteErrorResponse writes a response indicating an error occurred to the
// writer output.
func WriteErrorResponse(ctx context.Context, w io.Writer, err error, op string) {
	resp, err := ResponseToMessage(MakeErrorResponse(false, err))
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not write response",
			"op":      op,
		}))
		return
	}
	WriteResponse(ctx, w, resp, op)
}

// WriteOKResponse writes a response indicating that the request was ok.
func WriteOKResponse(ctx context.Context, w io.Writer, op string) {
	resp, err := ResponseToMessage(MakeErrorResponse(true, nil))
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not write response",
			"op":      op,
		}))
		return
	}
	WriteResponse(ctx, w, resp, op)
}

// WriteOKResponse writes a response indicating that the request was not ok.
func WriteNotOKResponse(ctx context.Context, w io.Writer, op string) {
	resp, err := ResponseToMessage(MakeErrorResponse(false, nil))
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not write response",
			"op":      op,
		}))
		return
	}
	WriteResponse(ctx, w, resp, op)

}

// MessageToRequest converts a mongowire.Message into a request
func MessageToRequest(msg mongowire.Message, out interface{}) error {
	doc, err := requestMessageToDocument(msg)
	if err != nil {
		return errors.Wrap(err, "could not read response")
	}
	b, err := doc.MarshalBSON()
	if err != nil {
		return errors.Wrap(err, "could not convert document to BSON")
	}
	if err := bson.Unmarshal(b, out); err != nil {
		return errors.Wrap(err, "could not convert BSON to response")
	}
	return nil
}

// MessageToResponse converts a mongowire.Message into a response.
func MessageToResponse(msg mongowire.Message, out interface{}) error {
	doc, err := responseMessageToDocument(msg)
	if err != nil {
		return errors.Wrap(err, "could not read response")
	}
	b, err := doc.MarshalBSON()
	if err != nil {
		return errors.Wrap(err, "could not convert document to BSON")
	}
	if err := bson.Unmarshal(b, out); err != nil {
		return errors.Wrap(err, "could not convert BSON to response")
	}
	return nil
}

// ResponseToMessage converts a response into a wire protocol reply.
// TODO: support OP_MSG
func ResponseToMessage(resp interface{}) (mongowire.Message, error) {
	b, err := bson.Marshal(resp)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert response to BSON")
	}
	doc, err := birch.ReadDocument(b)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert BSON response to document")
	}
	return mongowire.NewReply(0, 0, 0, 1, []birch.Document{*doc}), nil
}

// RequestToMessage converts a request into a wire protocol query.
// TODO: support OP_MSG
func RequestToMessage(req interface{}) (mongowire.Message, error) {
	// <namespace.$cmd  format is required to indicate that the OP_QUERY should
	// be interpreted as an OP_COMMAND.
	const namespace = "mrpc.$cmd"

	b, err := bson.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert response to BSON")
	}
	doc, err := birch.ReadDocument(b)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert BSON response to document")
	}
	return mongowire.NewQuery(namespace, 0, 0, 1, doc, birch.NewDocument()), nil
}

// requestMessageToDocument converts a wire protocol request message into a
// document.
// TODO: support OP_MSG
func requestMessageToDocument(msg mongowire.Message) (*birch.Document, error) {
	cmdMsg, ok := msg.(*mongowire.CommandMessage)
	if !ok {
		return nil, errors.Errorf("message is not of type %s", mongowire.OP_COMMAND.String())
	}
	return cmdMsg.CommandArgs, nil
}

// responseMessageToDocument converts a wire protocol response message into a
// document.
// TODO: support OP_MSG
func responseMessageToDocument(msg mongowire.Message) (*birch.Document, error) {
	if replyMsg, ok := msg.(*mongowire.ReplyMessage); ok {
		return &replyMsg.Docs[0], nil
	}
	if cmdReplyMsg, ok := msg.(*mongowire.CommandReplyMessage); ok {
		return cmdReplyMsg.CommandReply, nil
	}
	return nil, errors.Errorf("message is not of type %s nor %s", mongowire.OP_COMMAND_REPLY.String(), mongowire.OP_REPLY.String())
}

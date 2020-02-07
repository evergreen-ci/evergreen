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
func WriteErrorResponse(ctx context.Context, w io.Writer, t mongowire.OpType, err error, op string) {
	resp, err := ResponseToMessage(t, MakeErrorResponse(false, err))
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
func WriteOKResponse(ctx context.Context, w io.Writer, t mongowire.OpType, op string) {
	resp, err := ResponseToMessage(t, MakeErrorResponse(true, nil))
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
func WriteNotOKResponse(ctx context.Context, w io.Writer, t mongowire.OpType, op string) {
	resp, err := ResponseToMessage(t, MakeErrorResponse(false, nil))
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
func ResponseToMessage(t mongowire.OpType, resp interface{}) (mongowire.Message, error) {
	b, err := bson.Marshal(resp)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert response to BSON")
	}
	doc, err := birch.ReadDocument(b)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert BSON response to document")
	}
	if t == mongowire.OP_MSG {
		return mongowire.NewOpMessage(false, []birch.Document{*doc}), nil
	}
	return mongowire.NewReply(0, 0, 0, 1, []birch.Document{*doc}), nil
}

// RequestToMessage converts a request into a wire protocol query.
func RequestToMessage(t mongowire.OpType, req interface{}) (mongowire.Message, error) {
	b, err := bson.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert response to BSON")
	}
	doc, err := birch.ReadDocument(b)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert BSON response to document")
	}
	if t == mongowire.OP_MSG {
		return mongowire.NewOpMessage(false, []birch.Document{*doc}), nil
	}

	// <namespace.$cmd  format is required to indicate that the OP_QUERY should
	// be interpreted as an OP_COMMAND.
	const namespace = "mrpc.$cmd"
	return mongowire.NewQuery(namespace, 0, 0, 1, doc, birch.NewDocument()), nil
}

// requestMessageToDocument converts a wire protocol request message into a
// document.
func requestMessageToDocument(msg mongowire.Message) (*birch.Document, error) {
	opMsg, ok := msg.(*mongowire.OpMessage)
	if ok {
		for _, section := range opMsg.Items {
			if section.Type() == mongowire.OpMessageSectionBody && len(section.Documents()) != 0 {
				return section.Documents()[0].Copy(), nil
			}
		}
		return nil, errors.Errorf("%s message did not contain body", msg.Header().OpCode)
	}
	opCmdMsg, ok := msg.(*mongowire.CommandMessage)
	if !ok {
		return nil, errors.Errorf("message is not of type %s", mongowire.OP_COMMAND.String())
	}
	return opCmdMsg.CommandArgs, nil
}

// responseMessageToDocument converts a wire protocol response message into a
// document.
func responseMessageToDocument(msg mongowire.Message) (*birch.Document, error) {
	if opReplyMsg, ok := msg.(*mongowire.ReplyMessage); ok {
		return &opReplyMsg.Docs[0], nil
	}
	if opCmdReplyMsg, ok := msg.(*mongowire.CommandReplyMessage); ok {
		return opCmdReplyMsg.CommandReply, nil
	}
	if opMsg, ok := msg.(*mongowire.OpMessage); ok {
		for _, section := range opMsg.Items {
			if section.Type() == mongowire.OpMessageSectionBody && len(section.Documents()) != 0 {
				return section.Documents()[0].Copy(), nil
			}
		}
		return nil, errors.Errorf("%s response did not contain body", mongowire.OP_MSG.String())
	}
	return nil, errors.Errorf("message is not of type %s, %s, nor %s", mongowire.OP_COMMAND_REPLY.String(), mongowire.OP_REPLY.String(), mongowire.OP_MSG.String())
}

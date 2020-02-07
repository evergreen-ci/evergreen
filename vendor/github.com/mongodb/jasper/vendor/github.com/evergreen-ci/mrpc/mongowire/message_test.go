package mongowire

import (
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/mrpc/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func TestMessage(t *testing.T) {
	bytes, err := bson.Marshal(bson.M{"foo": "bar"})
	require.NoError(t, err)
	query, err := birch.ReadDocument(bytes)
	require.NoError(t, err)
	bytes, err = bson.Marshal(bson.M{"bar": "foo"})
	require.NoError(t, err)
	project, err := birch.ReadDocument(bytes)
	require.NoError(t, err)

	headerSize := 16
	for _, test := range []struct {
		name        string
		message     Message
		header      MessageHeader
		hasResponse bool
		scope       *OpScope
		bodySize    int
	}{
		{
			name:     OP_REPLY.String(),
			message:  NewReply(1, 0, 0, 1, []birch.Document{*query, *project}),
			header:   MessageHeader{RequestID: 19, OpCode: OP_REPLY},
			scope:    nil,
			bodySize: 20 + getDocSize(query) + getDocSize(project),
		},
		{
			name: OP_MSG.String(),
			message: NewOpMessage(
				false,
				[]birch.Document{*query},
				model.SequenceItem{Identifier: "foo", Documents: []birch.Document{*project, *query}},
				model.SequenceItem{Identifier: "bar", Documents: []birch.Document{*query}},
			),
			header:   MessageHeader{RequestID: 19, OpCode: OP_MSG},
			scope:    &OpScope{Type: OP_MSG, Command: "foo"},
			bodySize: 4 + (1 + getDocSize(query)) + (1 + 4 + 3 + 1 + getDocSize(project) + getDocSize(query)) + (1 + 4 + 3 + 1 + getDocSize(query)),
		},
		{
			name:     OP_UPDATE.String(),
			message:  NewUpdate("ns", 0, query, project),
			header:   MessageHeader{RequestID: 19, OpCode: OP_UPDATE},
			scope:    &OpScope{Type: OP_UPDATE, Context: "ns"},
			bodySize: 8 + 3 + getDocSize(query) + getDocSize(project),
		},
		{
			name:     OP_INSERT.String(),
			message:  NewInsert("ns", query, project),
			header:   MessageHeader{RequestID: 19, OpCode: OP_INSERT},
			scope:    &OpScope{Type: OP_INSERT, Context: "ns"},
			bodySize: 4 + 3 + getDocSize(query) + getDocSize(project),
		},
		{
			name:        OP_GET_MORE.String(),
			message:     NewGetMore("ns", 5, 98),
			header:      MessageHeader{RequestID: 19, OpCode: OP_GET_MORE},
			hasResponse: true,
			scope:       &OpScope{Type: OP_GET_MORE, Context: "ns"},
			bodySize:    16 + 3,
		},
		{
			name:     OP_DELETE.String(),
			message:  NewDelete("ns", 0, query),
			header:   MessageHeader{RequestID: 19, OpCode: OP_DELETE},
			scope:    &OpScope{Type: OP_DELETE, Context: "ns"},
			bodySize: 8 + 3 + getDocSize(query),
		},
		{
			name:     OP_KILL_CURSORS.String(),
			message:  NewKillCursors(1, 2, 3),
			header:   MessageHeader{RequestID: 19, OpCode: OP_KILL_CURSORS},
			scope:    &OpScope{Type: OP_KILL_CURSORS},
			bodySize: 8 + 8*3,
		},
		{
			name:        OP_COMMAND.String(),
			message:     NewCommand("db", "cmd", query, project, []birch.Document{*query, *project}),
			header:      MessageHeader{RequestID: 19, OpCode: OP_COMMAND},
			hasResponse: true,
			scope:       &OpScope{Type: OP_COMMAND, Context: "db", Command: "cmd"},
			bodySize:    3 + 4 + 2*getDocSize(query) + 2*getDocSize(project),
		},
		{
			name:     OP_COMMAND_REPLY.String(),
			message:  NewCommandReply(query, project, []birch.Document{*query, *project}),
			header:   MessageHeader{RequestID: 19, OpCode: OP_COMMAND_REPLY},
			bodySize: 2*getDocSize(query) + 2*getDocSize(project),
		},
		{
			name:        OP_QUERY.String(),
			message:     NewQuery("ns", 0, 0, 1, query, project),
			header:      MessageHeader{RequestID: 19, OpCode: OP_QUERY},
			hasResponse: true,
			scope:       &OpScope{Type: OP_QUERY, Context: "ns"},
			bodySize:    12 + 3 + getDocSize(query) + getDocSize(project),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.header, test.message.Header())
			assert.Equal(t, test.hasResponse, test.message.HasResponse())
			assert.Equal(t, test.scope, test.message.Scope())
			assert.Equal(t, headerSize+test.bodySize, len(test.message.Serialize()))
			assert.Equal(t, int32(headerSize+test.bodySize), test.message.Header().Size)
			m, err := test.header.Parse(test.message.Serialize()[headerSize:])
			require.NoError(t, err)
			assert.Equal(t, test.message.Serialize(), m.Serialize())
			assert.Equal(t, test.message.Header(), m.Header())
		})
	}
}

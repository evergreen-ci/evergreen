package buildlogger

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/timber/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockClient struct {
	createErr  bool
	appendErr  bool
	closeErr   bool
	logData    *gopb.LogData
	logLines   *gopb.LogLines
	logEndInfo *gopb.LogEndInfo
}

func (mc *mockClient) CreateLog(_ context.Context, in *gopb.LogData, _ ...grpc.CallOption) (*gopb.BuildloggerResponse, error) {
	if mc.createErr {
		return nil, errors.New("create error")
	}

	mc.logData = in

	return &gopb.BuildloggerResponse{LogId: in.Info.TestName}, nil
}

func (mc *mockClient) AppendLogLines(_ context.Context, in *gopb.LogLines, _ ...grpc.CallOption) (*gopb.BuildloggerResponse, error) {
	if mc.appendErr {
		return nil, errors.New("append error")
	}

	mc.logLines = in

	return &gopb.BuildloggerResponse{LogId: in.LogId}, nil
}

func (*mockClient) StreamLogLines(_ context.Context, _ ...grpc.CallOption) (gopb.Buildlogger_StreamLogLinesClient, error) {
	return nil, nil
}

func (mc *mockClient) CloseLog(_ context.Context, in *gopb.LogEndInfo, _ ...grpc.CallOption) (*gopb.BuildloggerResponse, error) {
	if mc.closeErr {
		return nil, errors.New("close error")
	}

	mc.logEndInfo = in

	return &gopb.BuildloggerResponse{LogId: in.LogId}, nil
}

type mockSender struct {
	*send.Base
	lastMessage string
}

func (ms *mockSender) Send(m message.Composer) {
	if ms.Level().ShouldLog(m) {
		ms.lastMessage = m.String()
	}
}

func (ms *mockSender) Flush(_ context.Context) error { return nil }

func TestLoggerOptionsValidate(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		opts := &LoggerOptions{ClientConn: &grpc.ClientConn{}}
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogFormat(opts.Format), gopb.LogFormat_LOG_FORMAT_UNKNOWN)
		assert.Equal(t, gopb.LogStorage(opts.Storage), gopb.LogStorage_LOG_STORAGE_S3)
		assert.NotNil(t, opts.Local)
		assert.Equal(t, defaultMaxBufferSize, opts.MaxBufferSize)
		assert.Equal(t, defaultFlushInterval, opts.FlushInterval)

		size := 100
		interval := time.Second
		local := &mockSender{Base: send.NewBase("test")}
		opts = &LoggerOptions{
			ClientConn:    &grpc.ClientConn{},
			MaxBufferSize: size,
			FlushInterval: interval,
			Local:         local,
		}
		require.NoError(t, opts.validate())
		assert.Equal(t, size, opts.MaxBufferSize)
		assert.Equal(t, interval, opts.FlushInterval)
		assert.Equal(t, local, opts.Local)

		opts.Format = LogFormatText
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogFormat(opts.Format), gopb.LogFormat_LOG_FORMAT_TEXT)
		opts.Format = LogFormatJSON
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogFormat(opts.Format), gopb.LogFormat_LOG_FORMAT_JSON)
		opts.Format = LogFormatBSON
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogFormat(opts.Format), gopb.LogFormat_LOG_FORMAT_BSON)

		opts.Storage = LogStorageS3
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogStorage(opts.Storage), gopb.LogStorage_LOG_STORAGE_S3)
		opts.Storage = LogStorageLocal
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogStorage(opts.Storage), gopb.LogStorage_LOG_STORAGE_LOCAL)
		opts.Storage = LogStorageGridFS
		require.NoError(t, opts.validate())
		assert.Equal(t, gopb.LogStorage(opts.Storage), gopb.LogStorage_LOG_STORAGE_GRIDFS)
	})
	t.Run("InvalidLogFormat", func(t *testing.T) {
		opts := &LoggerOptions{
			ClientConn: &grpc.ClientConn{},
			Format:     4,
		}
		assert.Error(t, opts.validate())
	})
	t.Run("InvalidLogStorage", func(t *testing.T) {
		opts := &LoggerOptions{
			ClientConn: &grpc.ClientConn{},
			Storage:    3,
		}
		assert.Error(t, opts.validate())
	})
	t.Run("ClientConnNilAndNoAddressOrPort", func(t *testing.T) {
		opts := &LoggerOptions{
			Insecure: true,
		}
		assert.Error(t, opts.validate())
	})
	t.Run("InsecureFalseAndNoCreds", func(t *testing.T) {
		opts := &LoggerOptions{
			BaseAddress: "cedar.mongodb.com",
		}
		assert.Error(t, opts.validate())
	})
}

func TestNewLogger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := testutil.NewMockBuildloggerServer(ctx, 4000)
	require.NoError(t, err)
	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)

	t.Run("WithExistingClient", func(t *testing.T) {
		subCtx, subCancel := context.WithCancel(ctx)

		name := "test"
		l := send.LevelInfo{Default: level.Debug, Threshold: level.Debug}
		opts := &LoggerOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "task_name",
			TaskID:      "task_id",
			Execution:   1,
			TestName:    "test_name",
			Trial:       1,
			ProcessName: "process",
			Format:      LogFormatJSON,
			Tags:        []string{"tag1", "tag2"},
			Arguments:   map[string]string{"arg1": "one", "arg2": "two"},
			Mainline:    true,
			Storage:     LogStorageS3,
			Local:       &mockSender{Base: send.NewBase("test")},
			ClientConn:  conn,
		}

		s, err := NewLoggerWithContext(ctx, name, l, opts)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, name, s.Name())
		assert.Equal(t, l, s.Level())
		srv.Mu.Lock()
		require.NotNil(t, srv.CreateLog)
		assert.Equal(t, opts.Project, srv.Create.Info.Project)
		assert.Equal(t, opts.Version, srv.Create.Info.Version)
		assert.Equal(t, opts.Version, srv.Create.Info.Version)
		assert.Equal(t, opts.Variant, srv.Create.Info.Variant)
		assert.Equal(t, opts.TaskName, srv.Create.Info.TaskName)
		assert.Equal(t, opts.TaskID, srv.Create.Info.TaskId)
		assert.Equal(t, opts.Execution, srv.Create.Info.Execution)
		assert.Equal(t, opts.TestName, srv.Create.Info.TestName)
		assert.Equal(t, opts.Trial, srv.Create.Info.Trial)
		assert.Equal(t, opts.ProcessName, srv.Create.Info.ProcName)
		assert.Equal(t, gopb.LogFormat(opts.Format), srv.Create.Info.Format)
		assert.Equal(t, opts.Tags, srv.Create.Info.Tags)
		assert.Equal(t, gopb.LogStorage(opts.Storage), srv.Create.Storage)
		srv.Mu.Unlock()
		b, ok := s.(*buildlogger)
		require.True(t, ok)
		require.NotNil(t, b.ctx)
		assert.NotNil(t, b.cancel)
		subCancel()
		assert.Equal(t, context.Canceled, subCtx.Err())
		assert.NotNil(t, b.buffer)
		assert.Equal(t, opts, b.opts)
		assert.Equal(t, defaultMaxBufferSize, b.opts.MaxBufferSize)
		assert.Equal(t, defaultFlushInterval, b.opts.FlushInterval)
		time.Sleep(time.Second)
		b.mu.Lock()
		assert.NotNil(t, b.timer)
		b.mu.Unlock()
		srv.Mu.Lock()
		srv.Create = nil
		srv.Mu.Unlock()
	})
	t.Run("WithoutExistingClient", func(t *testing.T) {
		subCtx, subCancel := context.WithCancel(ctx)
		name := "test2"
		l := send.LevelInfo{Default: level.Trace, Threshold: level.Alert}
		opts := &LoggerOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "task_name",
			TaskID:      "task_id",
			Execution:   1,
			TestName:    "test_name",
			Trial:       1,
			ProcessName: "process",
			Format:      LogFormatJSON,
			Tags:        []string{"tag1", "tag2"},
			Arguments:   map[string]string{"arg1": "one", "arg2": "two"},
			Mainline:    true,
			Storage:     LogStorageS3,
			Local:       &mockSender{Base: send.NewBase("test")},
			Insecure:    true,
			BaseAddress: srv.DialOpts.BaseAddress,
			RPCPort:     srv.DialOpts.RPCPort,
		}

		s, err := NewLoggerWithContext(ctx, name, l, opts)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, name, s.Name())
		assert.Equal(t, l, s.Level())
		srv.Mu.Lock()
		require.NotNil(t, srv.CreateLog)
		assert.Equal(t, opts.Project, srv.Create.Info.Project)
		assert.Equal(t, opts.Version, srv.Create.Info.Version)
		assert.Equal(t, opts.Version, srv.Create.Info.Version)
		assert.Equal(t, opts.Variant, srv.Create.Info.Variant)
		assert.Equal(t, opts.TaskName, srv.Create.Info.TaskName)
		assert.Equal(t, opts.TaskID, srv.Create.Info.TaskId)
		assert.Equal(t, opts.Execution, srv.Create.Info.Execution)
		assert.Equal(t, opts.TestName, srv.Create.Info.TestName)
		assert.Equal(t, opts.Trial, srv.Create.Info.Trial)
		assert.Equal(t, opts.ProcessName, srv.Create.Info.ProcName)
		assert.Equal(t, gopb.LogFormat(opts.Format), srv.Create.Info.Format)
		assert.Equal(t, opts.Tags, srv.Create.Info.Tags)
		assert.Equal(t, gopb.LogStorage(opts.Storage), srv.Create.Storage)
		srv.Mu.Unlock()
		b, ok := s.(*buildlogger)
		require.True(t, ok)
		require.NotNil(t, b.ctx)
		assert.NotNil(t, b.cancel)
		subCancel()
		assert.Equal(t, context.Canceled, subCtx.Err())
		assert.NotNil(t, b.cancel)
		assert.NotNil(t, b.buffer)
		assert.Equal(t, opts, b.opts)
		assert.Equal(t, defaultMaxBufferSize, b.opts.MaxBufferSize)
		assert.Equal(t, defaultFlushInterval, b.opts.FlushInterval)
		time.Sleep(time.Second)
		b.mu.Lock()
		assert.NotNil(t, b.timer)
		b.mu.Unlock()
		srv.Mu.Lock()
		srv.Create = nil
		srv.Mu.Unlock()
	})
	t.Run("WithoutContext", func(t *testing.T) {
		name := "test"
		l := send.LevelInfo{Default: level.Debug, Threshold: level.Debug}
		opts := &LoggerOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "task_name",
			TaskID:      "task_id",
			Execution:   1,
			TestName:    "test_name",
			Trial:       1,
			ProcessName: "process",
			Format:      LogFormatJSON,
			Tags:        []string{"tag1", "tag2"},
			Arguments:   map[string]string{"arg1": "one", "arg2": "two"},
			Mainline:    true,
			Storage:     LogStorageS3,
			Local:       &mockSender{Base: send.NewBase("test")},
			ClientConn:  conn,
		}

		s, err := NewLogger(name, l, opts)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, name, s.Name())
		assert.Equal(t, l, s.Level())
		srv.Mu.Lock()
		require.NotNil(t, srv.CreateLog)
		assert.Equal(t, opts.Project, srv.Create.Info.Project)
		assert.Equal(t, opts.Version, srv.Create.Info.Version)
		assert.Equal(t, opts.Version, srv.Create.Info.Version)
		assert.Equal(t, opts.Variant, srv.Create.Info.Variant)
		assert.Equal(t, opts.TaskName, srv.Create.Info.TaskName)
		assert.Equal(t, opts.TaskID, srv.Create.Info.TaskId)
		assert.Equal(t, opts.Execution, srv.Create.Info.Execution)
		assert.Equal(t, opts.TestName, srv.Create.Info.TestName)
		assert.Equal(t, opts.Trial, srv.Create.Info.Trial)
		assert.Equal(t, opts.ProcessName, srv.Create.Info.ProcName)
		assert.Equal(t, gopb.LogFormat(opts.Format), srv.Create.Info.Format)
		assert.Equal(t, opts.Tags, srv.Create.Info.Tags)
		assert.Equal(t, gopb.LogStorage(opts.Storage), srv.Create.Storage)
		srv.Mu.Unlock()
		b, ok := s.(*buildlogger)
		require.True(t, ok)
		assert.NotNil(t, b.ctx)
		assert.NotNil(t, b.cancel)
		assert.NotNil(t, b.buffer)
		assert.Equal(t, opts, b.opts)
		assert.Equal(t, defaultMaxBufferSize, b.opts.MaxBufferSize)
		assert.Equal(t, defaultFlushInterval, b.opts.FlushInterval)
		time.Sleep(time.Second)
		b.mu.Lock()
		assert.NotNil(t, b.timer)
		b.mu.Unlock()
		srv.Mu.Lock()
		srv.Create = nil
		srv.Mu.Unlock()
	})
	t.Run("NegativeFlushInterval", func(t *testing.T) {
		name := "test"
		l := send.LevelInfo{Default: level.Debug, Threshold: level.Debug}
		opts := &LoggerOptions{
			ClientConn:    conn,
			Local:         &mockSender{Base: send.NewBase("test")},
			FlushInterval: -1,
		}

		s, err := NewLoggerWithContext(ctx, name, l, opts)
		require.NoError(t, err)
		require.NotNil(t, s)
		b, ok := s.(*buildlogger)
		require.True(t, ok)
		time.Sleep(time.Second)
		assert.Nil(t, b.timer)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		s, err := NewLoggerWithContext(ctx, "test3", send.LevelInfo{}, &LoggerOptions{})
		assert.Error(t, err)
		assert.Nil(t, s)
	})
	t.Run("CreateLogError", func(t *testing.T) {
		name := "test4"
		l := send.LevelInfo{Default: level.Debug, Threshold: level.Debug}
		ms := &mockSender{Base: send.NewBase("test")}
		opts := &LoggerOptions{
			ClientConn: conn,
			Local:      ms,
		}
		srv.Mu.Lock()
		srv.CreateErr = true
		srv.Mu.Unlock()

		s, err := NewLoggerWithContext(ctx, name, l, opts)
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.True(t, strings.Contains(ms.lastMessage, "create error"))
	})
}

func TestSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("RespectsPriority", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.MaxBufferSize = 4096

		require.NoError(t, b.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Emergency}))
		m := message.ConvertToComposer(level.Alert, "alert")
		b.Send(m)
		assert.Empty(t, b.buffer)
		m = message.ConvertToComposer(level.Emergency, "emergency")
		b.Send(m)
		require.NotEmpty(t, b.buffer)
		assert.EqualValues(t, m.String(), b.buffer[len(b.buffer)-1].Data)

		require.NoError(t, b.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Debug}))
		m = message.ConvertToComposer(level.Debug, "debug")
		b.Send(m)
		require.NotEmpty(t, b.buffer)
		assert.EqualValues(t, m.String(), b.buffer[len(b.buffer)-1].Data)
	})
	t.Run("FlushAtCapacityWithNewLineCheck", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.logID = "id"
		b.opts.MaxBufferSize = 4096
		size := 256
		messages := []string{}

		for b.bufferSize < b.opts.MaxBufferSize {
			if b.bufferSize-size <= 0 {
				size = b.opts.MaxBufferSize - b.bufferSize
			}

			m := message.ConvertToComposer(level.Debug, utility.MakeRandomString(size/2))
			b.Send(m)
			require.Empty(t, ms.lastMessage)

			lines := strings.Split(m.String(), "\n")
			for i, line := range lines {
				if len(line) == 0 {
					continue
				}
				assert.Nil(t, mc.logLines)
				require.True(t, len(b.buffer) >= len(lines))
				assert.Equal(t, time.Now().Unix(), b.buffer[len(b.buffer)-1].Timestamp.Seconds)
				assert.EqualValues(t, line, b.buffer[len(b.buffer)-(len(lines)-i)].Data)
				messages = append(messages, line)
			}
		}
		assert.Equal(t, b.opts.MaxBufferSize, b.bufferSize)
		m := message.ConvertToComposer(level.Debug, "overflow")
		messages = append(messages, m.String())
		b.Send(m)
		require.Empty(t, b.buffer)
		assert.Equal(t, 0, b.bufferSize)
		require.NotNil(t, mc.logLines)
		assert.Equal(t, b.opts.logID, mc.logLines.LogId)
		assert.Len(t, mc.logLines.Lines, len(messages))
		for i := range mc.logLines.Lines {
			assert.EqualValues(t, messages[i], mc.logLines.Lines[i].Data)
			assert.EqualValues(t, level.Debug, mc.logLines.Lines[0].Priority)
		}
	})
	t.Run("FlushAtCapacityWithOutNewLineCheck", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.logID = "id"
		b.opts.MaxBufferSize = 4096
		b.opts.DisableNewLineCheck = true
		size := 256
		messages := []message.Composer{}

		for b.bufferSize < b.opts.MaxBufferSize {
			if b.bufferSize-size <= 0 {
				size = b.opts.MaxBufferSize - b.bufferSize
			}

			m := message.ConvertToComposer(level.Info, utility.MakeRandomString(size/2))
			b.Send(m)
			require.Empty(t, ms.lastMessage)

			assert.Nil(t, mc.logLines)
			require.NotEmpty(t, b.buffer)
			assert.Equal(t, time.Now().Unix(), b.buffer[len(b.buffer)-1].Timestamp.Seconds)
			assert.EqualValues(t, m.String(), b.buffer[len(b.buffer)-1].Data)
			messages = append(messages, m)
		}
		assert.Equal(t, b.opts.MaxBufferSize, b.bufferSize)
		m := message.ConvertToComposer(level.Debug, "overflow")
		messages = append(messages, m)
		b.Send(m)
		require.Empty(t, b.buffer)
		assert.Equal(t, 0, b.bufferSize)
		require.NotEmpty(t, mc.logLines)
		assert.Equal(t, b.opts.logID, mc.logLines.LogId)
		assert.Len(t, mc.logLines.Lines, len(messages))
		for i := range mc.logLines.Lines {
			assert.EqualValues(t, messages[i].String(), mc.logLines.Lines[i].Data)
			assert.EqualValues(t, messages[i].Priority(), mc.logLines.Lines[i].Priority)
		}
	})
	t.Run("FlushAtInterval", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.logID = "id"
		b.opts.MaxBufferSize = 4096
		b.opts.FlushInterval = time.Second
		b.opts.DisableNewLineCheck = true
		size := 256

		// flushes after interval
		m := message.ConvertToComposer(level.Debug, utility.MakeRandomString(size/2))
		b.Send(m)
		require.NotEmpty(t, b.buffer)
		go b.timedFlush()
		time.Sleep(2 * time.Second)
		b.mu.Lock()
		require.Empty(t, b.buffer)
		b.mu.Unlock()
		require.Len(t, mc.logLines.Lines, 1)
		assert.EqualValues(t, m.String(), mc.logLines.Lines[0].Data)
		assert.EqualValues(t, m.Priority(), mc.logLines.Lines[0].Priority)

		// flush resets timer
		m = message.ConvertToComposer(level.Emergency, utility.MakeRandomString(size/2))
		b.Send(m)
		b.mu.Lock()
		require.NotEmpty(t, b.buffer)
		b.mu.Unlock()
		time.Sleep(2 * time.Second)
		b.mu.Lock()
		require.Empty(t, b.buffer)
		b.mu.Unlock()
		require.Len(t, mc.logLines.Lines, 1)
		assert.EqualValues(t, m.String(), mc.logLines.Lines[0].Data)
		assert.EqualValues(t, m.Priority(), mc.logLines.Lines[0].Priority)

		// recent last flush
		b.mu.Lock()
		b.lastFlush = time.Now().Add(time.Minute)
		b.mu.Unlock()
		b.Send(m)
		require.NotEmpty(t, b.buffer)
		time.Sleep(2 * time.Second)
		require.NotEmpty(t, b.buffer)
		assert.EqualValues(t, m.String(), b.buffer[0].Data)
	})
	t.Run("GroupComposer", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.MaxBufferSize = 4096
		b.opts.DisableNewLineCheck = true

		m1 := message.ConvertToComposer(level.Emergency, "Hello world!\nThis has a new line.")
		m2 := message.ConvertToComposer(level.Debug, "Goodbye world!")
		m := message.NewGroupComposer([]message.Composer{m1, m2})
		b.Send(m)
		assert.Len(t, b.buffer, 3)
		assert.Equal(t, len(m1.String())+len(m2.String())-1, b.bufferSize)
		assert.EqualValues(t, strings.Split(m1.String(), "\n")[0], b.buffer[0].Data)
		assert.EqualValues(t, m.Priority(), b.buffer[0].Priority)
		assert.EqualValues(t, strings.Split(m1.String(), "\n")[1], b.buffer[1].Data)
		assert.EqualValues(t, m.Priority(), b.buffer[1].Priority)
		assert.EqualValues(t, m2.String(), b.buffer[2].Data)
		assert.EqualValues(t, m.Priority(), b.buffer[2].Priority)
	})
	t.Run("WithPrefix", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.MaxBufferSize = 4096
		b.opts.Prefix = "prefix"

		m := message.ConvertToComposer(level.Emergency, "Hello world!")
		b.Send(m)
		require.NoError(t, b.Close())
		require.Len(t, mc.logLines.Lines, 1)
		assert.EqualValues(t, fmt.Sprintf("[%s] %s", b.opts.Prefix, m.String()), mc.logLines.Lines[0].Data)
		assert.EqualValues(t, m.Priority(), mc.logLines.Lines[0].Priority)
	})
	t.Run("RPCError", func(t *testing.T) {
		str := "overflow"
		mc := &mockClient{appendErr: true}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)

		m := message.ConvertToComposer(level.Debug, str)
		b.Send(m)
		assert.Len(t, b.buffer, 1)
		assert.Equal(t, len(m.String()), b.bufferSize)
		assert.Equal(t, "append error", ms.lastMessage)
	})
	t.Run("ClosedSender", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.logID = "id"
		b.opts.MaxBufferSize = 4096
		b.closed = true

		b.Send(message.ConvertToComposer(level.Debug, "should fail"))
		assert.Empty(t, b.buffer)
		assert.Zero(t, b.bufferSize)
		assert.Nil(t, mc.logLines)
		assert.Equal(t, "cannot call Send on a closed Buildlogger Sender", ms.lastMessage)
	})
}

func TestFlush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("ForceFlush", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.logID = "id"
		b.opts.MaxBufferSize = 4096

		m := message.ConvertToComposer(level.Info, "overflow")
		b.Send(m)
		require.NotEmpty(t, b.buffer)
		require.NoError(t, b.Flush(ctx))
		assert.Empty(t, b.buffer)
		assert.Zero(t, b.bufferSize)
		assert.True(t, time.Since(b.lastFlush) <= time.Second)
		require.Len(t, mc.logLines.Lines, 1)
		assert.EqualValues(t, "overflow", mc.logLines.Lines[0].Data)
	})
	t.Run("ClosedSender", func(t *testing.T) {
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(ctx, mc, ms)
		b.opts.logID = "id"
		b.opts.MaxBufferSize = 4096

		m := message.ConvertToComposer(level.Info, "overflow")
		b.Send(m)
		require.NotEmpty(t, b.buffer)
		b.closed = true
		require.NoError(t, b.Flush(ctx))
		assert.NotEmpty(t, b.buffer)
		assert.NotZero(t, b.bufferSize)
		assert.Nil(t, mc.logLines)
	})
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("CloseNonNilConn", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(subCtx, mc, ms)

		assert.NoError(t, b.Close())
		b.closed = false
		b.conn = &grpc.ClientConn{}
		assert.Panics(t, func() { _ = b.Close() })
	})
	t.Run("EmptyBuffer", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(subCtx, mc, ms)
		b.opts.logID = "id"
		b.opts.SetExitCode(10)

		require.NoError(t, b.Close())
		assert.Equal(t, b.opts.logID, mc.logEndInfo.LogId)
		assert.Equal(t, b.opts.exitCode, mc.logEndInfo.ExitCode)
		assert.True(t, b.closed)
		assert.Equal(t, context.Canceled, b.ctx.Err())
	})
	t.Run("NonEmptyBuffer", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(subCtx, mc, ms)
		b.opts.logID = "id"
		b.opts.SetExitCode(2)
		logLine := &gopb.LogLine{Timestamp: &timestamp.Timestamp{}, Data: []byte("some data")}
		b.buffer = append(b.buffer, logLine)

		require.NoError(t, b.Close())
		assert.NotNil(t, mc.logEndInfo)
		assert.Equal(t, b.opts.logID, mc.logEndInfo.LogId)
		assert.Equal(t, b.opts.exitCode, mc.logEndInfo.ExitCode)
		assert.NotNil(t, mc.logLines)
		assert.Equal(t, b.opts.logID, mc.logLines.LogId)
		assert.Equal(t, logLine, mc.logLines.Lines[0])
		assert.True(t, b.closed)
		assert.Equal(t, context.Canceled, b.ctx.Err())
	})
	t.Run("NoopWhenClosed", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		mc := &mockClient{}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(subCtx, mc, ms)
		b.closed = true

		require.NoError(t, b.Close())
		assert.Nil(t, mc.logEndInfo)
		assert.True(t, b.closed)
		assert.Equal(t, context.Canceled, b.ctx.Err())
	})
	t.Run("RPCErrors", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		mc := &mockClient{appendErr: true}
		ms := &mockSender{Base: send.NewBase("test")}
		b := createSender(subCtx, mc, ms)
		logLine := &gopb.LogLine{Timestamp: &timestamp.Timestamp{}, Data: []byte("some data")}
		b.buffer = append(b.buffer, logLine)

		assert.Error(t, b.Close())
		assert.Equal(t, "append error", ms.lastMessage)

		b.closed = false
		mc.appendErr = false
		mc.closeErr = true
		assert.Error(t, b.Close())
		assert.Equal(t, "close error", ms.lastMessage)
		assert.True(t, b.closed)
		assert.Equal(t, context.Canceled, b.ctx.Err())
	})
}

func createSender(ctx context.Context, mc gopb.BuildloggerClient, ms send.Sender) *buildlogger {
	ctx, cancel := context.WithCancel(ctx)
	return &buildlogger{
		ctx:    ctx,
		cancel: cancel,
		opts: &LoggerOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "task_name",
			TaskID:      "task_id",
			Execution:   1,
			TestName:    "test_name",
			Trial:       2,
			ProcessName: "proc_name",
			Tags:        []string{"tag1", "tag2", "tag3"},
			Arguments:   map[string]string{"tag1": "val", "tag2": "val2"},
			Mainline:    true,
			Local:       ms,
			Format:      LogFormat(gopb.LogFormat_LOG_FORMAT_TEXT),
		},
		client: mc,
		buffer: []*gopb.LogLine{},
		Base:   send.NewBase("test"),
	}
}

func startRPCService(ctx context.Context, service gopb.BuildloggerServer, port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return errors.WithStack(err)
	}

	s := grpc.NewServer()
	gopb.RegisterBuildloggerServer(s, service)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	return nil
}

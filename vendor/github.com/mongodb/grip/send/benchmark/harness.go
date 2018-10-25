package send

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const (
	one     = 1
	five    = 5
	ten     = 2 * five
	hundred = ten * ten

	executionTimeout = five * time.Minute
	minRuntime       = five * time.Second
	minIterations    = hundred
)

func defaultLevelInfo() send.LevelInfo {
	return send.LevelInfo{Default: level.Trace, Threshold: level.Trace}
}

func makeMessages(numMsgs int, size int, priority level.Priority) []message.Composer {
	messages := make([]message.Composer, numMsgs)
	for i := 0; i < numMsgs; i++ {
		text := strings.Repeat("a", size)
		messages[i] = message.NewDefaultMessage(priority, text)
	}
	return messages
}

func messageSizes() []int {
	return []int{100, 10000}
}

func messageCounts() []int {
	return []int{100, 1000, 10000}
}

func senderCases() map[string]benchCase {
	return map[string]benchCase{
		"BufferedSender":     bufferedSenderCase,
		"CallSiteFileLogger": callSiteFileLoggerCase,
		"FileLogger":         fileLoggerCase,
		"InMemorySender":     inMemorySenderCase,
		"JSONFileLoggerCase": jsonFileLoggerCase,
		"StreamLogger":       streamLoggerCase,
	}
}

func sendMessages(ctx context.Context, tm TimerManager, iters int, s send.Sender, msgs []message.Composer) error {
	tm.ResetTimer()
	defer tm.StopTimer()
	for n := 0; n < iters; n++ {
		for _, msg := range msgs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				s.Send(msg)
			}
		}
	}
	return nil
}

func bufferedSenderCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) (err error) {
	internal, err := send.NewInternalLogger("buffered", defaultLevelInfo())
	if err != nil {
		return err
	}
	minBufferLength := 5 * time.Second
	s := send.NewBufferedSender(internal, minBufferLength, 100000)
	defer func(s send.Sender) {
		if e := s.Close(); e != nil {
			err = e
		}
	}(s)

	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

func callSiteFileLoggerCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) error {
	file, err := ioutil.TempFile("", "bench_out.txt")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	s, err := send.NewCallSiteFileLogger("callsite", file.Name(), 1, defaultLevelInfo())
	if err != nil {
		return err
	}
	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

func fileLoggerCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) error {
	file, err := ioutil.TempFile("", "bench_out.txt")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	s, err := send.NewPlainFileLogger("plain", file.Name(), defaultLevelInfo())
	if err != nil {
		return err
	}
	err = s.SetFormatter(send.MakePlainFormatter())
	if err != nil {
		return err
	}
	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

func inMemorySenderCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) error {
	s, err := send.NewInMemorySender("inmemory", defaultLevelInfo(), 10000)
	if err != nil {
		return err
	}
	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

func internalSenderCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) error {
	s, err := send.NewInternalLogger("internal", defaultLevelInfo())
	if err != nil {
		return err
	}
	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

func jsonFileLoggerCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) error {
	file, err := ioutil.TempFile("", "bench_out.txt")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	s, err := send.NewJSONFileLogger("json", file.Name(), defaultLevelInfo())
	if err != nil {
		return err
	}
	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

func streamLoggerCase(ctx context.Context, tm TimerManager, iters int, size int, numMsgs int) error {
	s, err := send.NewStreamLogger("stream", &bytes.Buffer{}, defaultLevelInfo())
	if err != nil {
		return err
	}
	msgs := makeMessages(numMsgs, size, defaultLevelInfo().Default)
	return sendMessages(ctx, tm, iters, s, msgs)
}

type benchCase func(ctx context.Context, tm TimerManager, iters int, size int, numOps int) error

func getAllCases() []*caseDefinition {
	cases := make([]*caseDefinition, 0)
	for senderName, senderCase := range senderCases() {
		for _, msgSize := range messageSizes() {
			for _, msgCount := range messageCounts() {
				cases = append(cases,
					&caseDefinition{
						name:       fmt.Sprintf("%s/%dBytesPerMessage/Send%dMessages", senderName, msgSize, msgCount),
						bench:      senderCase,
						count:      one,
						numOps:     msgCount,
						size:       msgSize,
						runtime:    minRuntime,
						iterations: minIterations,
					},
				)
			}
		}
	}
	return cases
}

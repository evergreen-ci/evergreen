package util

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

type TestWrappedWriter struct {
	input []byte
}

func (self *TestWrappedWriter) Write(p []byte) (n int, err error) {
	self.input = p
	return len(p), nil
}

func (self *TestWrappedWriter) Flush() {
	self.input = []byte{}
}

func TestNewLineBufferingWriter(t *testing.T) {
	Convey("Using a LineBufferingWriter should", t, func() {
		testWriter := &TestWrappedWriter{
			input: []byte{},
		}
		bufferWriter := NewLineBufferingWriter(testWriter)
		Convey("flush properly", func() {
			bufferWriter.Write([]byte("hello"))
			So(bufferWriter.buf, ShouldNotBeEmpty)
			bufferWriter.Flush()
			So(bufferWriter.buf, ShouldBeEmpty)
		})
		Convey("write to writer if ending with a newline", func() {
			bufferWriter.Write([]byte("this has a newline\n"))
			So(bufferWriter.buf, ShouldBeEmpty)
			So(testWriter.input, ShouldNotBeEmpty)
			So(string(testWriter.input[:]), ShouldEqual, "this has a newline")
			testWriter.Flush()
			bufferWriter.Flush()
		})
		Convey("write to writer if there is no newline, but should when there is a newline", func() {
			bufferWriter.Write([]byte("this should stay in the buffer..."))
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(testWriter.input, ShouldBeEmpty)
			bufferWriter.Write([]byte("this should be appended to the previous\n"))
			So(bufferWriter.buf, ShouldBeEmpty)
			So(testWriter.input, ShouldNotBeEmpty)
			So(string(testWriter.input[:]), ShouldEqual, "this should stay in the buffer...this should be appended to the previous")
			testWriter.Flush()
			bufferWriter.Flush()
		})
		Convey("write out if the size of the input + buffer is greater than 4K", func() {
			first_input := make([]byte, 100)
			second_input := make([]byte, (4 * 1024))
			bufferWriter.Write(first_input)
			So(testWriter.input, ShouldBeEmpty)
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(len(bufferWriter.buf), ShouldEqual, 100)
			bufferWriter.Write(second_input)
			So(testWriter.input, ShouldNotBeEmpty)
			So(len(testWriter.input), ShouldEqual, 100)
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(len(bufferWriter.buf), ShouldEqual, (4 * 1024))
			testWriter.Flush()
			bufferWriter.Flush()
		})

	})
}

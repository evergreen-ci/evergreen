package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
		var err error
		testWriter := &TestWrappedWriter{
			input: []byte{},
		}
		bufferWriter := NewLineBufferingWriter(testWriter)
		Convey("flush properly", func() {
			_, err = bufferWriter.Write([]byte("hello"))
			So(err, ShouldBeNil)
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(bufferWriter.Flush(), ShouldBeNil)
			So(bufferWriter.buf, ShouldBeEmpty)
		})
		Convey("write to writer if ending with a newline", func() {
			_, err = bufferWriter.Write([]byte("this has a newline\n"))
			So(err, ShouldBeNil)
			So(bufferWriter.buf, ShouldBeEmpty)
			So(testWriter.input, ShouldNotBeEmpty)
			So(string(testWriter.input[:]), ShouldEqual, "this has a newline")
			testWriter.Flush()
			So(bufferWriter.Flush(), ShouldBeNil)
		})
		Convey("write to writer if there is no newline, but should when there is a newline", func() {
			_, err = bufferWriter.Write([]byte("this should stay in the buffer..."))
			So(err, ShouldBeNil)
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(testWriter.input, ShouldBeEmpty)
			_, err = bufferWriter.Write([]byte("this should be appended to the previous\n"))
			So(err, ShouldBeNil)
			So(bufferWriter.buf, ShouldBeEmpty)
			So(testWriter.input, ShouldNotBeEmpty)
			So(string(testWriter.input[:]), ShouldEqual, "this should stay in the buffer...this should be appended to the previous")
			testWriter.Flush()
			So(bufferWriter.Flush(), ShouldBeNil)
		})
		Convey("write out if the size of the input + buffer is greater than 4K", func() {
			first_input := make([]byte, 100)
			second_input := make([]byte, (4 * 1024))
			_, err = bufferWriter.Write(first_input)
			So(err, ShouldBeNil)
			So(testWriter.input, ShouldBeEmpty)
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(len(bufferWriter.buf), ShouldEqual, 100)
			_, err = bufferWriter.Write(second_input)
			So(err, ShouldBeNil)
			So(testWriter.input, ShouldNotBeEmpty)
			So(len(testWriter.input), ShouldEqual, 100)
			So(bufferWriter.buf, ShouldNotBeEmpty)
			So(len(bufferWriter.buf), ShouldEqual, (4 * 1024))
			testWriter.Flush()
			So(bufferWriter.Flush(), ShouldBeNil)
		})

	})
}

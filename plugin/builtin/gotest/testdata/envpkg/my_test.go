package envpkg

import (
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func TestPass(t *testing.T) {
	Convey("This should pass if environment var set", t, func() {
		So(os.Getenv("PATH"), ShouldEndWith, ":bacon")
	})
}

package envpkg

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPass(t *testing.T) {
	Convey("This should pass if environment var set", t, func() {
		So(os.Getenv("PATH"), ShouldEndWith, ":bacon")
	})
}

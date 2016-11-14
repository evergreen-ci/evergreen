package comm

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimeoutWatcher(t *testing.T) {
	Convey("With a timeout watcher at a set interval", t, func() {
		tw := TimeoutWatcher{}
		tw.duration = time.Second
		signalChan := make(chan Signal)
		Convey("timeout should only get sent after Checkin() is not called "+
			"within threshold", func() {
			started := time.Now()
			go tw.NotifyTimeouts(signalChan)
			go func() {
				for i := 0; i <= 40; i++ {
					time.Sleep(100 * time.Millisecond)
					tw.CheckIn()
				}
			}()
			outSignal := <-signalChan
			ended := time.Now()
			So(outSignal, ShouldEqual, IdleTimeout)
			So(ended, ShouldNotHappenWithin, 3900*time.Millisecond, started)
		})
	})
}

package gologger

func ExampleLogger() {
	// initialize
	logger := New()

	// basic usage (with colors)
	logger.Debug("debug") // 2013-08-17T09:27:50.711 DEBUG debug
	logger.Info("info")   // 2013-08-17T09:27:50.712 INFO  info
	logger.Warn("warn")   // 2013-08-17T09:27:50.712 WARN  warn
	logger.Error("error") // 2013-08-17T09:27:50.712 ERROR error

	// formatted
	logger.Debugf("got %d results", 10) // 2013-08-17T09:27:50.712 DEBUG got 10 results

	// levels
	logger.LogLevel = INFO
	logger.Debug("not printed") // # blank
	logger.Info("printed")      // 2013-08-17T09:36:58.407 INFO  printed

	// with prefix
	logger.Prefix = "some.host"
	logger.Info("info with host") // 2013-08-17T09:27:50.712 [some.host] INFO  info with host

	// turn off colors
	logger.Colored = false
	logger.Info("info") // 2013-08-17T09:27:50.712 [some.host] INFO  info
}

package apimodels

import (
	"fmt"
	"net/http"
	"time"
)

type MyWriter struct {
	under http.ResponseWriter
	code  int
}

func (self *MyWriter) Header() http.Header {
	return self.under.Header()
}

func (self *MyWriter) Write(data []byte) (int, error) {
	return self.under.Write(data)
}

func (self *MyWriter) WriteHeader(code int) {
	self.code = code
	self.under.WriteHeader(code)
}

type AccessLogger struct {
	router http.Handler
}

func NewAccessLogger(router http.Handler) *AccessLogger {
	return &AccessLogger{router}
}

func (self *AccessLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	myWriter := &MyWriter{w, 0}

	self.router.ServeHTTP(myWriter, r)

	var scheme string
	if r.TLS == nil {
		scheme = "http"
	} else {
		scheme = "https"
	}
	fmt.Printf("access: %v %v %v %v %v %v\n", time.Now(), r.RemoteAddr, r.URL,
		myWriter.code, time.Since(startTime), scheme)
}

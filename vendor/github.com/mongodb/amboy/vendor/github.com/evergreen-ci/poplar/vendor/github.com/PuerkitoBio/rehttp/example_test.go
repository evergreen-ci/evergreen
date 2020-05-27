package rehttp_test

import (
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
)

func Example() {
	tr := rehttp.NewTransport(
		nil, // will use http.DefaultTransport
		rehttp.RetryAll(rehttp.RetryMaxRetries(3), rehttp.RetryTemporaryErr()), // max 3 retries for Temporary errors
		rehttp.ConstDelay(time.Second),                                         // wait 1s between retries
	)
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second, // Client timeout applies to all retries as a whole
	}
	res, err := client.Get("http://example.com")
	if err != nil {
		// handle err
	}
	// handle response
	res.Body.Close()
}

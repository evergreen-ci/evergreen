package hec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	retryWaitTime = 1 * time.Second

	defaultMaxContentLength = 1000000

	defaultAcknowledgementTimeout = 90 * time.Second
)

type Client struct {
	HEC

	// HTTP Client for communication with (optional)
	httpClient *http.Client

	// Splunk Server URL for API requests (required)
	serverURL string

	// HEC Token (required)
	token string

	// Keep-Alive (optional, default: true)
	keepAlive bool

	// Channel (required for Raw mode)
	channel string

	// Max retrying times (optional, default: 2)
	retries int

	// Max content length (optional, default: 1000000)
	maxLength int

	// List of acknowledgement IDs provided by Splunk
	ackIDs []int

	// Mutex to allow threadsafe acknowledgement checking
	ackMux sync.Mutex
}

func NewClient(serverURL string, token string) HEC {
	id := uuid.New()

	return &Client{
		httpClient: http.DefaultClient,
		serverURL:  serverURL,
		token:      token,
		keepAlive:  true,
		channel:    id.String(),
		retries:    2,
		maxLength:  defaultMaxContentLength,
	}
}

func (hec *Client) SetHTTPClient(client *http.Client) {
	hec.httpClient = client
}

func (hec *Client) SetKeepAlive(enable bool) {
	hec.keepAlive = enable
}

func (hec *Client) SetChannel(channel string) {
	hec.channel = channel
}

func (hec *Client) SetMaxRetry(retries int) {
	hec.retries = retries
}

func (hec *Client) SetMaxContentLength(size int) {
	hec.maxLength = size
}

func (hec *Client) WriteEventWithContext(ctx context.Context, event *Event) error {
	if event.empty() {
		return nil // skip empty events
	}

	endpoint := "/services/collector?channel=" + hec.channel
	data, _ := json.Marshal(event)

	if len(data) > hec.maxLength {
		return ErrEventTooLong
	}
	return hec.write(ctx, endpoint, data)
}

func (hec *Client) WriteEvent(event *Event) error {
	return hec.WriteEventWithContext(context.Background(), event)
}

func (hec *Client) WriteBatchWithContext(ctx context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}

	endpoint := "/services/collector?channel=" + hec.channel
	var buffer bytes.Buffer
	var tooLongs []int

	for index, event := range events {
		if event.empty() {
			continue // skip empty events
		}

		data, _ := json.Marshal(event)
		if len(data) > hec.maxLength {
			tooLongs = append(tooLongs, index)
			continue
		}
		// Send out bytes in buffer immediately if the limit exceeded after adding this event
		if buffer.Len()+len(data) > hec.maxLength {
			if err := hec.write(ctx, endpoint, buffer.Bytes()); err != nil {
				return err
			}
			buffer.Reset()
		}
		buffer.Write(data)
	}

	if buffer.Len() > 0 {
		if err := hec.write(ctx, endpoint, buffer.Bytes()); err != nil {
			return err
		}
	}
	if len(tooLongs) > 0 {
		return ErrEventTooLong
	}
	return nil
}

func (hec *Client) WriteBatch(events []*Event) error {
	return hec.WriteBatchWithContext(context.Background(), events)
}

type EventMetadata struct {
	Host       *string
	Index      *string
	Source     *string
	SourceType *string
	Time       *time.Time
}

func (hec *Client) WriteRawWithContext(ctx context.Context, reader io.ReadSeeker, metadata *EventMetadata) error {
	endpoint := rawHecEndpoint(hec.channel, metadata)

	return breakStream(reader, hec.maxLength, func(chunk []byte) error {
		if err := hec.write(ctx, endpoint, chunk); err != nil {
			// Ignore NoData error (e.g. "\n\n" will cause NoData error)
			if res, ok := err.(*Response); !ok || res.Code != StatusNoData {
				return err
			}
		}
		return nil
	})
}

func (hec *Client) WriteRaw(reader io.ReadSeeker, metadata *EventMetadata) error {
	return hec.WriteRawWithContext(context.Background(), reader, metadata)
}

type acknowledgementRequest struct {
	Acks []int `json:"acks"`
}

// WaitForAcknowledgementWithContext blocks until the Splunk indexer has
// acknowledged that all previously submitted data has been successfully
// indexed or if the provided context is cancelled. This requires the HEC token
// configuration in Splunk to have indexer acknowledgement enabled.
func (hec *Client) WaitForAcknowledgementWithContext(ctx context.Context) error {
	// Make our own copy of the list of acknowledgement IDs and remove them
	// from the client while we check them.
	hec.ackMux.Lock()
	ackIDs := hec.ackIDs
	hec.ackIDs = nil
	hec.ackMux.Unlock()

	if len(ackIDs) == 0 {
		return nil
	}

	endpoint := "/services/collector/ack?channel=" + hec.channel

	for {
		ackRequestData, _ := json.Marshal(acknowledgementRequest{Acks: ackIDs})

		response, err := hec.makeRequest(ctx, endpoint, ackRequestData)
		if err != nil {
			// Put the remaining unacknowledged IDs back
			hec.ackMux.Lock()
			hec.ackIDs = append(hec.ackIDs, ackIDs...)
			hec.ackMux.Unlock()
			return err
		}

		for ackIDString, status := range response.Acks {
			if status {
				ackID, err := strconv.Atoi(ackIDString)
				if err != nil {
					return fmt.Errorf("could not convert ack ID to int: %v", err)
				}

				ackIDs = remove(ackIDs, ackID)
			}
		}

		if len(ackIDs) == 0 {
			break
		}

		// If the server did not indicate that all acknowledgements have been
		// made, check again after a short delay.
		select {
		case <-time.After(retryWaitTime):
			continue
		case <-ctx.Done():
			// Put the remaining unacknowledged IDs back
			hec.ackMux.Lock()
			hec.ackIDs = append(hec.ackIDs, ackIDs...)
			hec.ackMux.Unlock()
			return ctx.Err()
		}
	}

	return nil
}

// WaitForAcknowledgement blocks until the Splunk indexer has acknowledged
// that all previously submitted data has been successfully indexed or if the
// default acknowledgement timeout is reached. This requires the HEC token
// configuration in Splunk to have indexer acknowledgement enabled.
func (hec *Client) WaitForAcknowledgement() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAcknowledgementTimeout)
	defer cancel()
	return hec.WaitForAcknowledgementWithContext(ctx)
}

// breakStream breaks text from reader into chunks, with every chunk less than max.
// Unless a single line is longer than max, it always cut at end of lines ("\n")
func breakStream(reader io.ReadSeeker, max int, callback func(chunk []byte) error) error {

	var buf []byte = make([]byte, max+1)
	var writeAt int
	for {
		n, err := reader.Read(buf[writeAt:max])
		if n == 0 && err == io.EOF {
			break
		}

		// If last line does not end with LF, add one for it
		if err == io.EOF && buf[writeAt+n-1] != '\n' {
			n++
			buf[writeAt+n-1] = '\n'
		}

		data := buf[0 : writeAt+n]

		// Cut after the last LF character
		cut := bytes.LastIndexByte(data, '\n') + 1
		if cut == 0 {
			// This line is too long, but just let it break here
			cut = len(data)
		}
		if err := callback(buf[:cut]); err != nil {
			return err
		}

		writeAt = copy(buf, data[cut:])

		if err != nil && err != io.EOF {
			return err
		}
	}

	if writeAt != 0 {
		return callback(buf[:writeAt])
	}

	return nil
}

func responseFrom(body []byte) *Response {
	var res Response
	json.Unmarshal(body, &res)
	return &res
}

func (res *Response) Error() string {
	return res.Text
}

func (res *Response) String() string {
	b, _ := json.Marshal(res)
	return string(b)
}

func (hec *Client) makeRequest(ctx context.Context, endpoint string, data []byte) (*Response, error) {
	retries := 0
RETRY:
	req, err := http.NewRequest(http.MethodPost, hec.serverURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if hec.keepAlive {
		req.Header.Set("Connection", "keep-alive")
	}
	req.Header.Set("Authorization", "Splunk "+hec.token)
	res, err := hec.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}

	response := responseFrom(body)

	if res.StatusCode != http.StatusOK {
		if retriable(response.Code) && retries < hec.retries {
			retries++
			time.Sleep(retryWaitTime)
			goto RETRY
		}
	}

	return response, nil
}

func (hec *Client) write(ctx context.Context, endpoint string, data []byte) error {
	response, err := hec.makeRequest(ctx, endpoint, data)
	if err != nil {
		return err
	}

	// TODO: find out the correct code
	if response.Text != "Success" {
		return response
	}

	// Check for acknowledgement IDs and store them if provided
	if response.AckID != nil {
		hec.ackMux.Lock()
		defer hec.ackMux.Unlock()

		hec.ackIDs = append(hec.ackIDs, *response.AckID)
	}

	return nil
}

func rawHecEndpoint(channel string, metadata *EventMetadata) string {
	var buffer bytes.Buffer
	buffer.WriteString("/services/collector/raw?channel=" + channel)
	if metadata == nil {
		return buffer.String()
	}
	if metadata.Host != nil {
		buffer.WriteString("&host=" + *metadata.Host)
	}
	if metadata.Index != nil {
		buffer.WriteString("&index=" + *metadata.Index)
	}
	if metadata.Source != nil {
		buffer.WriteString("&source=" + *metadata.Source)
	}
	if metadata.SourceType != nil {
		buffer.WriteString("&sourcetype=" + *metadata.SourceType)
	}
	if metadata.Time != nil {
		buffer.WriteString("&time=" + epochTime(metadata.Time))
	}
	return buffer.String()
}

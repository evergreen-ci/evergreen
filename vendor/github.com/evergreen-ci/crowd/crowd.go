package crowd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

var ErrUnauthorized = errors.New("Unauthorized")

// User is a user's metadata returned by Crowd.
type User struct {
	Active       bool   `json:"active"`
	DispName     string `json:"display-name"`
	EmailAddress string `json:"email"`
	FirstName    string `json:"first-name"`
	LastName     string `json:"last-name"`
	Name         string `json:"name"`
}

// Session is a crowd session which contains a token which can be used to look up a user
type Session struct {
	Expand      string      `json:"active"`
	CreatedDate int64       `json:"created-date"`
	ExpiryDate  int64       `json:"expiry-date"`
	User        SessionUser `json:"user"`
	Link        SessionLink `json:"link"`
	Token       string      `json:"token"`
}

type SessionUser struct {
	Name string      `json:"name"`
	Link SessionLink `json:"link"`
}

type SessionLink struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

// Client is a wrapper for an HTTP client for communicating with Crowd's REST API.
type Client struct {
	crowdUsername string
	crowdPassword string
	apiRoot       *url.URL
}

// NewClient constructs a client that communicates with crowd's API endpoints at a base
// URL (e.g. https://crowd.10gen.com) with the given credentials.
func NewClient(crowdUsername string, crowdPassword string, baseUrl string) (*Client, error) {
	apiRoot, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}
	return &Client{crowdUsername, crowdPassword, apiRoot}, nil
}

// GetUser makes an API call to look up a user by name.
func (self *Client) GetUser(username string) (*User, error) {
	values := url.Values{}
	values.Add("username", username)
	subUrl, err := self.apiRoot.Parse("/crowd/rest/usermanagement/latest/user?" + values.Encode())
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	req, err := http.NewRequest("GET", subUrl.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create request: %v", err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(self.crowdUsername, self.crowdPassword)
	client := &http.Client{}
	start := time.Now()
	msg := message.Fields{
		"message":  "starting request to crowd",
		"path":     subUrl.String(),
		"method":   "GET",
		"function": "GetUser",
	}
	grip.Debug(msg)
	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("Error making http request: %v", err)
	}
	msg["message"] = "crowd request finished"
	msg["duration"] = time.Since(start)
	grip.Debug(msg)

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	} else if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Received unexpected status code from crowd: %d (couldn't parse body)", resp.StatusCode)
		}
		return nil, fmt.Errorf("Received unexpected status code from crowd: %d (%s)", resp.StatusCode, body)
	}
	result := User{}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error occurred reading data from response: %v", resp.StatusCode)
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("Error parsing json from crowd: %v", err)
	}

	return &result, nil
}

// GetUserFromToken makes an API call to look up a user by a session token.
func (self *Client) GetUserFromToken(token string) (*User, error) {
	// Note: at this point the token is the actual token string.

	subUrl, err := self.apiRoot.Parse("/crowd/rest/usermanagement/latest/session/" + token)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	req, err := http.NewRequest("GET", subUrl.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create request: %v", err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(self.crowdUsername, self.crowdPassword)
	client := &http.Client{}
	start := time.Now()
	msg := message.Fields{
		"message":  "starting request to crowd",
		"path":     subUrl.String(),
		"method":   "GET",
		"function": "GetUserFromToken",
	}
	grip.Debug(msg)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error making http request: %v", err)
	}
	msg["message"] = "crowd request finished"
	msg["duration"] = time.Since(start)
	grip.Debug(msg)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from crowd: %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error occurred reading data from response: %v", resp.StatusCode)
	}

	result := struct {
		User *User `json:"user"`
	}{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("Error parsing json from crowd: %v", err)
	}

	return result.User, nil
}

// CreateSession makes an API call to create a new session for the user with the given credentials.
func (self *Client) CreateSession(username, password string) (*Session, error) {
	subUrl, err := self.apiRoot.Parse("/crowd/rest/usermanagement/latest/session")
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}
	jsonBytes, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", subUrl.String(), ioutil.NopCloser(bytes.NewReader(jsonBytes)))
	req.ContentLength = int64(len(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("Could not create request: %v", err)
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(self.crowdUsername, self.crowdPassword)

	client := &http.Client{}
	start := time.Now()
	msg := message.Fields{
		"message":  "starting request to crowd",
		"path":     subUrl.String(),
		"method":   "POST",
		"function": "CreateSession",
	}
	grip.Debug(msg)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making http request: %v", err)
	}
	msg["message"] = "crowd request finished"
	msg["duration"] = time.Since(start)
	grip.Debug(msg)
	if resp == nil {
		return nil, fmt.Errorf("received nil response from %v", subUrl.String())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("CreateSession received unexpected status code from crowd: %d (couldn't parse body)", resp.StatusCode)
		}
		return nil, fmt.Errorf("CreateSession received unexpected status code from crowd: %d (%s)", resp.StatusCode, body)
	}
	session := &Session{}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("(%v) error occurred reading data from response: %v",
			resp.StatusCode, err)
	}

	if err = json.Unmarshal(body, session); err != nil {
		return nil, fmt.Errorf("error parsing json from crowd: %v", err)
	}
	return session, nil
}

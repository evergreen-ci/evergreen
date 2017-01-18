package thirdparty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/tychoish/grip"
)

type CrowdUser struct {
	Active       bool   `json:"active"`
	DispName     string `json:"display-name"`
	EmailAddress string `json:"email"`
	FirstName    string `json:"first-name"`
	LastName     string `json:"last-name"`
	Name         string `json:"name"`
}

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

type WrapCrowdUser struct {
	User CrowdUser `json:"user"`
}

func (self *CrowdUser) DisplayName() string {
	return self.DispName
}

func (self *CrowdUser) Email() string {
	return self.EmailAddress
}

func (self *CrowdUser) Username() string {
	return self.Name
}

type RESTCrowdService struct {
	crowdUsername string
	crowdPassword string
	apiRoot       *url.URL
}

//NewRESTCrowdService constructs a REST-based implementation that allows
//fetching user info from Crowd. Takes the username/password credentials, and
//a URL which is the base of the REST endpoint, e.g. "https://crowd.10gen.com/"

func NewRESTCrowdService(crowdUsername string, crowdPassword string, baseUrl string) (*RESTCrowdService, error) {
	apiRoot, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}
	return &RESTCrowdService{crowdUsername, crowdPassword, apiRoot}, nil
}

func (self *RESTCrowdService) GetUser(username string) (*CrowdUser, error) {
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
	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("Error making http request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received unexpected status code from crowd: %v", resp.StatusCode)
	}
	result := CrowdUser{}

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

func (self *RESTCrowdService) GetUserFromToken(token string) (*CrowdUser, error) {
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error making http request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received unexpected status code from crowd: %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error occurred reading data from response: %v", resp.StatusCode)
	}

	result := WrapCrowdUser{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("Error parsing json from crowd: %v", err)
	}

	return &(result.User), nil
}

func (self *RESTCrowdService) CreateSession(username, password string) (*Session, error) {
	grip.Debugf("Requesting user session for '%v' from crowd", username)
	subUrl, err := self.apiRoot.Parse("/crowd/rest/usermanagement/latest/session")
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}
	postData := map[string]string{
		"username": username,
		"password": password,
	}
	jsonBytes, err := json.Marshal(postData)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", subUrl.String(), ioutil.NopCloser(bytes.NewReader(jsonBytes)))
	if err != nil {
		return nil, fmt.Errorf("Could not create request: %v", err)
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(self.crowdUsername, self.crowdPassword)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making http request: %v", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("received nil response from %v", subUrl.String())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("(%v) received unexpected status code from crowd",
			resp.StatusCode)
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

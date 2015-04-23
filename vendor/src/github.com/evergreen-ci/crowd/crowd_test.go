package crowd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	testUser     = "crowduser"
	testPassword = "crowdpassword"
	testUserInfo = `{
		"display-name": "Jonny",
		"email": "john.doe@microsoft.com",
		"first-name" : "Jonathan",
		"last-name"  : "Doe",
		"name"       : "johndoe"
	}`
)

func TestCrowdClient(t *testing.T) {
	mc := mockCrowd()
	server := httptest.NewServer(mc)

	// Check that client gets an unauthorized error if user/password is wrong
	c, err := NewClient("wronguser", "wrongpw", server.URL)
	if err != nil {
		t.Error(err)
	}
	u, err := c.GetUser("bob")
	if err != ErrUnauthorized {
		t.Errorf("Expected to get Unauthorized error but got: %v", err)
	}

	c, err = NewClient(testUser, testPassword, server.URL)
	if err != nil {
		t.Error(err)
	}
	u, err = c.GetUser("johndoe")
	if err != nil {
		t.Errorf("Didn't expect error but got: %v", err)
	}
	if u.FirstName != "Jonathan" {
		t.Errorf("User information didn't match expected: %v", u.FirstName)
	}
}

func mockCrowd() http.Handler {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/crowd/rest/usermanagement/latest/user",
		func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != testUser || p != testPassword {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.FormValue("username") != "johndoe" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testUserInfo))
		})
	return serveMux
}

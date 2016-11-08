##Crowd

[![GoDoc](http://godoc.org/github.com/evergreen-ci/crowd?status.svg)](http://godoc.org/github.com/evergreen-ci/crowd)

Go library for communicating with Crowd's REST api.

```go
crowdClient := crowd.NewClient("crowduser",
                               "crowdpassword",
                               "https://crowd-server.com/")

// look up a user by username
user, err := crowdClient.GetUser("bob")

// create a new session
session, err := crowdClient.CreateSession("bob", "bobs_password_12345")

// look up a user by token
user, err := crowdClient.GetUserFromToken("ab4t9askjbrlkn33")
```

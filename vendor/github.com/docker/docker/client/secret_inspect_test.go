package client // import "github.com/docker/docker/client"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/swarm"
	"github.com/gotestyourself/gotestyourself/assert"
	is "github.com/gotestyourself/gotestyourself/assert/cmp"
	"github.com/pkg/errors"
)

func TestSecretInspectUnsupported(t *testing.T) {
	client := &Client{
		version: "1.24",
		client:  &http.Client{},
	}
	_, _, err := client.SecretInspectWithRaw(context.Background(), "nothing")
	assert.Check(t, is.Error(err, `"secret inspect" requires API version 1.25, but the Docker daemon API version is 1.24`))
}

func TestSecretInspectError(t *testing.T) {
	client := &Client{
		version: "1.25",
		client:  newMockClient(errorMock(http.StatusInternalServerError, "Server error")),
	}

	_, _, err := client.SecretInspectWithRaw(context.Background(), "nothing")
	if err == nil || err.Error() != "Error response from daemon: Server error" {
		t.Fatalf("expected a Server Error, got %v", err)
	}
}

func TestSecretInspectSecretNotFound(t *testing.T) {
	client := &Client{
		version: "1.25",
		client:  newMockClient(errorMock(http.StatusNotFound, "Server error")),
	}

	_, _, err := client.SecretInspectWithRaw(context.Background(), "unknown")
	if err == nil || !IsErrNotFound(err) {
		t.Fatalf("expected a secretNotFoundError error, got %v", err)
	}
}

func TestSecretInspectWithEmptyID(t *testing.T) {
	client := &Client{
		client: newMockClient(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("should not make request")
		}),
	}
	_, _, err := client.SecretInspectWithRaw(context.Background(), "")
	if !IsErrNotFound(err) {
		t.Fatalf("Expected NotFoundError, got %v", err)
	}
}

func TestSecretInspect(t *testing.T) {
	expectedURL := "/v1.25/secrets/secret_id"
	client := &Client{
		version: "1.25",
		client: newMockClient(func(req *http.Request) (*http.Response, error) {
			if !strings.HasPrefix(req.URL.Path, expectedURL) {
				return nil, fmt.Errorf("Expected URL '%s', got '%s'", expectedURL, req.URL)
			}
			content, err := json.Marshal(swarm.Secret{
				ID: "secret_id",
			})
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewReader(content)),
			}, nil
		}),
	}

	secretInspect, _, err := client.SecretInspectWithRaw(context.Background(), "secret_id")
	if err != nil {
		t.Fatal(err)
	}
	if secretInspect.ID != "secret_id" {
		t.Fatalf("expected `secret_id`, got %s", secretInspect.ID)
	}
}

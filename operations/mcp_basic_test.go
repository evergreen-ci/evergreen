package operations

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// Basic test for JSON-RPC parsing and unknown method handling
// This test is in a separate file to avoid init() conflicts with cli_integration_test.go
func TestMCPBasicFunctionality(t *testing.T) {
	server := &mcpServer{}
	var in bytes.Buffer
	var out bytes.Buffer

	// Write a malformed line
	in.WriteString("not-json\n")
	// Write an unknown method
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "nope"}
	b, _ := json.Marshal(req)
	in.WriteString(string(b) + "\n")

	err := server.runStdio(context.Background(), &in, &out)
	if err != nil {
		t.Fatalf("runStdio failed: %v", err)
	}

	// Ensure there is at least one response written
	if out.Len() == 0 {
		t.Fatalf("expected some output from server")
	}

	// Check that we got error responses
	scanner := bufio.NewScanner(&out)
	responses := 0
	for scanner.Scan() {
		var resp rpcResponse
		err := json.Unmarshal(scanner.Bytes(), &resp)
		if err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.Error == nil {
			t.Errorf("expected error response, got: %+v", resp)
		}
		responses++
	}
	if responses != 2 {
		t.Errorf("expected 2 responses, got %d", responses)
	}
}

func TestMCPDispatchBasics(t *testing.T) {
	server := &mcpServer{}

	t.Run("InvalidJSONRPCVersion", func(t *testing.T) {
		req := &rpcRequest{
			JSONRPC: "1.0",
			ID:      json.RawMessage("5"),
			Method:  "detectPatchId",
		}

		resp := server.dispatch(context.Background(), req)
		if resp.Error == nil {
			t.Errorf("expected error for invalid JSON-RPC version")
		}
		if resp.Error.Code != -32600 {
			t.Errorf("expected error code -32600, got %d", resp.Error.Code)
		}
	})

	t.Run("UnknownMethod", func(t *testing.T) {
		req := &rpcRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage("6"),
			Method:  "unknownMethod",
		}

		resp := server.dispatch(context.Background(), req)
		if resp.Error == nil {
			t.Errorf("expected error for unknown method")
		}
		if resp.Error.Code != -32601 {
			t.Errorf("expected error code -32601, got %d", resp.Error.Code)
		}
	})
}

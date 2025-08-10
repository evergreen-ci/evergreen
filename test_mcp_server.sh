#!/bin/bash

# Simple smoke test for the MCP server
# This script tests the basic JSON-RPC functionality without requiring real Evergreen credentials

set -e

echo "Testing MCP server smoke test..."

# Build the CLI if not already built
if [ ! -f "./bin/evergreen" ]; then
    echo "Building evergreen CLI..."
    go build -o ./bin/evergreen ./cmd/evergreen
fi

# Test 1: Unknown method should return method not found error
echo "Test 1: Testing unknown method..."
response=$(echo '{"jsonrpc":"2.0","id":1,"method":"unknownMethod","params":{}}' | timeout 5s ./bin/evergreen mcp-server --stdio 2>/dev/null || true)
echo "Response: $response"

if echo "$response" | grep -q '"error"' && echo "$response" | grep -q '"method not found"'; then
    echo "✓ Test 1 passed: Unknown method correctly returns error"
else
    echo "✗ Test 1 failed: Expected method not found error"
    exit 1
fi

# Test 2: Invalid JSON-RPC version should return error
echo -e "\nTest 2: Testing invalid JSON-RPC version..."
response=$(echo '{"jsonrpc":"1.0","id":2,"method":"detectPatchId","params":{}}' | timeout 5s ./bin/evergreen mcp-server --stdio 2>/dev/null || true)
echo "Response: $response"

if echo "$response" | grep -q '"error"' && echo "$response" | grep -q "jsonrpc must be"; then
    echo "✓ Test 2 passed: Invalid JSON-RPC version correctly returns error"
else
    echo "✗ Test 2 failed: Expected JSON-RPC version error"
    exit 1
fi

# Test 3: Malformed JSON should return parse error
echo -e "\nTest 3: Testing malformed JSON..."
response=$(echo 'not-valid-json' | timeout 5s ./bin/evergreen mcp-server --stdio 2>/dev/null || true)
echo "Response: $response"

if echo "$response" | grep -q '"error"' && echo "$response" | grep -q "parse error"; then
    echo "✓ Test 3 passed: Malformed JSON correctly returns parse error"
else
    echo "✗ Test 3 failed: Expected parse error"
    exit 1
fi

# Test 4: Valid method structure (will fail due to no credentials, but should parse correctly)
echo -e "\nTest 4: Testing valid method structure..."
response=$(echo '{"jsonrpc":"2.0","id":4,"method":"detectPatchId","params":{}}' | timeout 5s ./bin/evergreen mcp-server --stdio 2>/dev/null || true)
echo "Response: $response"

# This should either succeed or fail with a configuration/authentication error, not a parse/method error
if echo "$response" | grep -q '"jsonrpc":"2.0"' && echo "$response" | grep -q '"id":4'; then
    echo "✓ Test 4 passed: Valid method structure correctly parsed"
else
    echo "✗ Test 4 failed: Expected valid JSON-RPC response structure"
    exit 1
fi

echo -e "\n✓ All smoke tests passed!"
echo "The MCP server correctly handles JSON-RPC protocol basics."
echo ""
echo "To test with real Evergreen credentials:"
echo "1. Set up your ~/.evergreen.yml configuration"
echo "2. Run: echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"detectPatchId\",\"params\":{}}' | ./bin/evergreen mcp-server --stdio"
echo "3. Run: ./bin/evergreen patch-failed-logs --patch <your-patch-id>"

# Using Evergreen MCP Server with Augment

This guide shows how to set up and test the Evergreen MCP (Model Context Protocol) server with Augment for intelligent patch and task introspection.

## Prerequisites

- Evergreen CLI built with MCP server support
- Augment installed and configured
- Evergreen credentials configured in `~/.evergreen.yml`
- Network access to your Evergreen instance

## 1. Build and Test the MCP Server

First, build the Evergreen CLI with MCP support:

```bash
cd /Users/w.trocki/Projects/evergreen
go build -o ./bin/evergreen ./cmd/evergreen
```

This creates `bin/evergreen` with the MCP server command:
- `mcp-server-sdk` - Uses official MCP Go SDK

Test that the MCP server starts correctly:

```bash
./bin/evergreen mcp-server-sdk --help
```

Run the smoke test to validate basic functionality:

```bash
./test_mcp_server.sh
```

## 2. Configure Augment MCP Integration

Add the Evergreen MCP server to your Augment configuration. The exact location depends on your Augment setup, but typically you'll add this to your MCP servers configuration:

```json
{
  "mcpServers": {
    "evergreen": {
      "command": "/Users/w.trocki/Projects/evergreen/bin/evergreen",
      "args": ["mcp-server-sdk", "--stdio"],
      "description": "Evergreen patch and task introspection"
    }
  }
}
```

**Alternative configuration formats:**

For Augment desktop app, you might need to add this in the MCP settings UI:
- **Server Name:** `evergreen`
- **Command:** `/Users/w.trocki/Projects/evergreen/bin/evergreen`
- **Arguments:** `["mcp-server-sdk", "--stdio"]`

## 3. Available MCP Methods

The Evergreen MCP server exposes these methods:

### `detectPatchId`
Automatically detect the current patch ID using precedence:
1. Explicit patch parameter
2. `EVG_PATCH_ID` environment variable
3. Most recent patch (optionally filtered by project)

### `listPatchTasks`
List tasks for a patch, with optional filtering for failed tasks only.

### `getTaskLogs`
Retrieve logs for a specific task with configurable tail limit and log type.

### `fetchFailedLogsForPatch`
Fetch logs for all failed tasks in a patch with bounded concurrency.

## 4. Testing with Augment

Once configured, you can ask Augment natural language questions:

### Patch Detection
```
"Can you detect my current Evergreen patch ID?"
"What's my latest patch for the mongodb project?"
```

### Task Listing
```
"Show me all failed tasks in my current patch"
"List all tasks in patch 507f1f77bcf86cd799439011"
"What tasks are failing in my latest patch?"
```

### Log Retrieval
```
"Get the logs for task ID 507f1f77bcf86cd799439012"
"Show me the last 50 lines of logs for the failing compile task"
"Fetch logs for all failed tasks in my current patch"
```

### Analysis and Debugging
```
"Analyze the failed tasks in my patch and summarize the errors"
"What are the common failure patterns in my patch?"
"Help me debug the test failures in my latest patch"
```

## 5. Manual Testing (Before Augment Integration)

Test the MCP server manually to ensure it's working:

```bash
# Start the server
./bin/evergreen mcp-server-sdk --stdio

# In another terminal, send test requests:

# Initialize and detect patch ID
printf '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}\n{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "detectPatchId", "arguments": {}}}\n' | ./bin/evergreen mcp-server-sdk --stdio

# List failed tasks
printf '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}\n{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "listPatchTasks", "arguments": {"failedOnly": true}}}\n' | ./bin/evergreen mcp-server-sdk --stdio
```

## 6. Environment Configuration

### Automatic Patch Detection

Set the `EVG_PATCH_ID` environment variable for automatic patch detection:

```bash
export EVG_PATCH_ID="your-patch-id-here"
./bin/evergreen mcp-server-sdk --stdio
```

### Project-Specific Detection

Use the `--project` flag to filter patches by project:

```bash
./bin/evergreen mcp-server-sdk --stdio --project mongodb
```

### Custom Patch Limits

Control how many recent patches to consider:

```bash
./bin/evergreen mcp-server-sdk --stdio --max-patches 20
```

## 7. Troubleshooting

### Common Issues

**Authentication errors:**
- Ensure `~/.evergreen.yml` is properly configured
- Check that your API key is valid and not expired

**No patches found:**
- Verify you have recent patches in your account
- Try specifying a project with `--project` flag
- Check that you're authenticated to the correct Evergreen instance

**Connection timeouts:**
- Verify network connectivity to your Evergreen instance
- Check firewall settings

### Debug Mode

For verbose logging, you can modify the MCP server startup to include debug output:

```bash
GRIP_LEVEL=debug ./bin/evergreen mcp-server --stdio
```

## 8. Non-MCP Alternative

If you prefer direct CLI usage without MCP:

```bash
# Get failed logs for a specific patch
./bin/evergreen patch-failed-logs --patch 507f1f77bcf86cd799439011 --tail 200

# Use with automatic patch detection
EVG_PATCH_ID=507f1f77bcf86cd799439011 ./bin/evergreen patch-failed-logs --tail 100
```

## Next Steps

Once you have the MCP server working with Augment:

1. **Create workflows** for common debugging tasks
2. **Set up aliases** for frequently used patch IDs
3. **Integrate with CI/CD** by setting `EVG_PATCH_ID` in your build environment
4. **Extend functionality** by contributing additional MCP methods

The MCP integration enables powerful AI-assisted debugging and analysis of your Evergreen patches and tasks!

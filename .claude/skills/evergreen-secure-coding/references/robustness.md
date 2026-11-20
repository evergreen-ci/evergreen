# Robustness: Request Limits, Webhooks, Debug Ports, Concurrency

## Rules

### 1. Bound everything a caller controls

- Wrap request bodies before reading them: `r.Body = http.MaxBytesReader(w, r.Body, max)`, or `utility.NewRequestReader(r)` / `NewRequestReaderWithSize`. This matters most in unauthenticated and pre-auth code such as webhooks and auth middleware. `utility.ReadJSON` and `gimlet.GetJSONUnlimited` do not limit size.
- Cap list arguments, page sizes, limits, and fan-out in REST and GraphQL. A request that fans out into one DB query per element needs a maximum.
- Don't recurse on attacker-controlled data such as strings, paths, or dependency graphs without a depth limit or cycle detection. A Go stack overflow kills the process.

### 2. Verify inbound webhooks completely

- GitHub: validate the signature with the configured secret, and fail closed if the secret is empty.
- AWS SNS: after `VerifyPayload()`, also check that `TopicArn` is in the configured allowlist. A valid signature only proves the message came from *some* SNS topic.
- Check the event type before acting on it, and authorize the actor named in the event (for example, the commenter on a PR comment trigger) against the base repository.

### 3. Debug and admin listeners bind to loopback

Debug, profiling, and admin listeners bind to `127.0.0.1` (`net.JoinHostPort("127.0.0.1", port)`), never all interfaces. If remote access is needed, require authentication on that listener.

### 4. Maps under RWMutex

Writing to or deleting from a map while holding only `RLock` crashes the process ("concurrent map writes"). Take `Lock` for any mutation, and re-check after upgrading.

## Tests

Send a body over the limit, a webhook signed for another SNS topic, and a deeply nested or cyclic input. Assert each is rejected without a crash.

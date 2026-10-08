# Outbound Requests (SSRF)

Evergreen runs inside a cloud network with instance metadata services, internal admin ports, and private services. Any server-side fetch of a URL that a user, task, or project can influence must assume the URL targets those.

Sources of such URLs include anything stored in or returned from user, task, or project data, and redirects returned by any of these.

## Rules

### 1. Use one guarded client; don't hand-roll checks

Don't send user-influenced URLs with `http.Get`, `http.Post`, `http.DefaultClient`, `utility.GetHTTPClient()`, or a bare `&http.Client{}`. Use a client built like the webhook sender in `util/webhook_grip.go`:

- `transport.Proxy = nil`, because a proxy reaches destinations the dial-time check never sees
- `transport.DialContext` validates every resolved IP at connect time, which also defeats DNS rebinding
- `CheckRedirect` re-validates every hop, or redirects are refused
- timeouts and a response size limit

Validate at save time too, with `util.ValidateWebhookURL`, so users get early feedback. Save-time validation alone is never enough, because DNS answers change.

### 2. A complete blocklist, applied after normalizing the IP

If you touch IP-blocking logic, normalize first: unmap IPv4-mapped IPv6 addresses (`netip.Addr.Unmap`). Then reject:

- loopback, unspecified, link-local unicast and multicast, and multicast
- private ranges: RFC1918 plus IPv6 ULA `fc00::/7` (`IsPrivate`)
- CGNAT `100.64.0.0/10` and other non-global ranges
- the IPv4 and IPv6 instance-metadata addresses

Prefer `netip` predicates over string comparisons. Never skip IPv6 addresses (`To4() == nil`) as if they were safe.

### 3. Don't reflect what you fetched unless you must

When the response is only used for a status, don't store or return the body. Returning fetched content to the caller needs the strictest checks plus a content-type allowlist (see web-output.md).

### 4. Don't send credentials to user-chosen URLs

Requests to user-chosen URLs must not carry Evergreen service credentials, cookies, or other users' secrets. Per-subscription secrets belong only to that subscription's owner.

## Tests

Cover loopback, `169.254.169.254`, an IPv6 metadata address, an IPv4-mapped IPv6 address, an RFC1918 address, a hostname that resolves to a private IP, and a public URL that redirects to a private one.

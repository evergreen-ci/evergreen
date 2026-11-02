# Web Output: Notifications, Links, and Served Content

Evergreen data reaches browsers (Spruce, Parsley, legacy UI), email clients, Slack, and Jira. Patch descriptions, commit messages, task and host names, and URLs are attacker-controlled text in all of these.

## Rules

### 1. Escape for the destination

- HTML email and HTML pages use `html/template` with plain string fields. Never convert non-constant data to `template.HTML`, `template.JS`, `template.URL`, or `template.HTMLAttr`.
- Don't build HTML with `fmt.Sprintf`. If you must, wrap every value in `template.HTMLEscapeString`.
- `text/template` is for plain text only. Never send its output as HTML.
- Slack and Jira have their own markup. Escape `<`, `>`, and `&` for Slack, and break Jira markup (`{noformat}`, `[link|url]`) in user text.

### 2. URLs that will be rendered as links need a scheme allowlist

Any user-supplied URL that will be rendered as an `href` must be validated with `util.CheckURL`, which requires http or https and a host, or with an explicit `u.Scheme` allowlist, on **every** write path for that field. URL parsing alone does not restrict the scheme.

### 3. Serving stored files: server picks the content type

When serving stored content from the Evergreen origin:

- Parse with `mime.ParseMediaType`, then compare the media type exactly against an allowlist of inert types.
- Default to `application/octet-stream` with `Content-Disposition: attachment`.
- Always set `X-Content-Type-Options: nosniff`.

### 4. CORS: exact origins only

Reflect `Origin` into `Access-Control-Allow-Origin` only after an exact-match lookup against configured origins. Regex, prefix, suffix, and substring matches allow `trusted.example.com.attacker.net`.

### 5. Cookies and CSRF

Session cookies are `HttpOnly`, `Secure`, and have an explicit `SameSite`. Security keys fail closed when unset (see secrets.md).

## Tests

Use a patch description or name containing `<img src=x onerror=alert(1)>` and a URL of `javascript:alert(1)`. Assert the rendered output is escaped and the URL is rejected.

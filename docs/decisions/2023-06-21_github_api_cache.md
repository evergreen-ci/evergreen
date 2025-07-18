# 2023-06-21 GitHub API Response Caching

- status: accepted
- date: 2023-06-21
- authors: Jonathan Brill

## Context and Problem Statement

Evergreen has been exhausting its GitHub API limit on a regular basis. Switching to a GitHub App will provide a higher limit, but it's a ways away and it can't be the only solution.

## Decision Outcome

GitHub's API provides support for [conditional requests](https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#conditional-requests). If we specify the `If-None-Match` and `If-Modified-Since` headers and the response is a 304 then we can use the cached version without it counting against the rate-limit. The protocol for private caching is defined in [rfc7234](https://datatracker.ietf.org/doc/html/rfc7234). [This article](https://medium.com/pixelpoint/best-practices-for-cache-control-settings-for-your-website-ff262b38c5a2) is a primer on their use.

[Our SDK](https://github.com/google/go-github#conditional-requests) recommends the [httpcache package](https://github.com/gregjones/httpcache) which implements most of the lower-level details for us. It's used by instantiating its transport and including it in the transport chain the GitHub client's client uses. It provides a default in-memory cache but that would only cache on a per app-server basis and would be lost on restart. The package also provides a number of cache backends (redis, memcache, s3, etc.) and a [cache interface](https://github.com/gregjones/httpcache/blob/901d90724c7919163f472a9812253fb26761123d/httpcache.go#LL30C1-L39C2) for custom backends. We implemented a cache backend using a regular collection in the database as a quick-and-dirty solution for persisting the cache and letting the app servers share state. Although [GridFS](https://www.mongodb.com/docs/manual/core/gridfs/) might have made sense for this as it allows files greater than 16MB, its updates are not atomic so files might be partially overwritten.

One wrinkle is that GitHub sometimes sets the Cache-Control [max-age directive](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control#response_directives) which tells the cache the response is to be considered [fresh](https://developer.mozilla.org/en-US/docs/Web/HTTP/Caching#fresh_and_stale_based_on_age) for the next n seconds. If a response is fresh we won't even send a request to ascertain if the resource has changed, which is good for GitHub but could cause subtle bugs for our use-case. This is overcome by a custom transport that injects a `max-age=0` [request directive](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control#request_directives) into every request, which overrides the response's max-age and causes any cached response to be considered stale so we'll check in with GitHub no matter how long it's been.

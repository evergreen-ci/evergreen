---
hide_table_of_contents: true
---

# REST v2 API

## General Functionality

### Errors

When an error is encountered during a request, the API returns a JSON
object with the HTTP status code and a message describing the error of
the form:

    {
     "status": <http_status_code>,
     "error": <error message>
    }

### Pagination

API Routes that fetch many objects return them in a JSON array and
support paging through subsets of the total result set. When there are
additional results for the query, access to them is populated in a [Link
HTTP header](https://www.w3.org/wiki/LinkHeader). This header has the
form:

    "Link" : <http://<EVERGREEN_HOST>/rest/v2/path/to/resource?start_at=<pagination_key>&limit=<objects_per_page>; rel="next"

    <http://<EVERGREEN_HOST>/rest/v2/path/to/resource?start_at=<pagination_key>&limit=<objects_per_page>; rel="prev"

### Dates

Date fields are returned and accepted in ISO-8601 UTC extended format.
They contain 3 fractional seconds with a 'dot' separator.

### Empty Fields

A returned object will always contain its complete list of fields. Any
field that does not have an associated value will be filled with JSON's
null value.

## API Docs

import ApiDocMdx from '@theme/ApiDocMdx';

<ApiDocMdx id="evergreen-openapi" />

## Deprecated endpoints

### TaskStats (DEPRECATED)
**IMPORTANT: The task stats REST API has been deprecated, please use [Trino task stats](../Project-Configuration/Evergreen-Data-for-Analytics.md) instead.**

### Notifications  (DEPRECATED)

Create custom notifications for email or Slack issues. 

We are investigating moving this out of Evergreen (EVG-21065) and won't be supporting future work for this. 
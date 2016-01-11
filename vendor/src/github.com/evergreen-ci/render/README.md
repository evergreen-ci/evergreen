##Render  

[![GoDoc](http://godoc.org/github.com/evergreen-ci/render?status.svg)](http://godoc.org/github.com/evergreen-ci/render)

Library for caching/executing templates to render HTTP responses.

API is similar to [unrolled/render](http://github.com/unrolled/render) but cached/executed templates are each a collection of several files instead of all sharing the same set of files to allow for inheritance/embedding, similar to the approach suggested in http://stackoverflow.com/a/11468132.

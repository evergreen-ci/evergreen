argo
====

argo implements access to .ar archives in Go.

 * This library was modelled after Go's own `archive/tar` library, so the code & example resembles it closely. I have included Go's copyright and used a similar BSD-style licence.
 * At this stage argo only implements the 'common' format as used for .deb files.
 * Note that argo is not currently supporting either workaround for long filenames as defined by GNU ar or BSD ar. Please get in touch if you require this feature.

Please see [godoc for documentation](http://godoc.org/github.com/laher/argo/ar), including [an example](http://godoc.org/github.com/laher/argo/ar#example-package) and references.

argo has been tested and checked using `go vet`, `go fmt`, `go test -cover`, [errcheck](https://github.com/kisielk/errcheck) and [golint](https://github.com/golang/lint/golint).  See [gocover for test coverage](http://gocover.io/github.com/laher/argo/ar) - ![Coverage Status](http://gocover.io/_badge/github.com/laher/argo/ar?).

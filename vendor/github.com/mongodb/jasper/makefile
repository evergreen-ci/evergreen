buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

_testPackages := ./ ./cli ./rpc

testArgs := -v
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count='$(RUN_COUNT)'
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
endif

benchPattern := ./

compile:
	go build $(_testPackages)
compile-base:
	go build ./
race:
	@mkdir -p $(buildDir)
	go test $(testArgs) -race $(_testPackages) | tee $(buildDir)/race.sink.out
	@! grep -s -q -e "^FAIL" $(buildDir)/race.sink.out && ! grep -s -q "^WARNING: DATA RACE" $(buildDir)/race.sink.out
test:
	@mkdir -p $(buildDir)
	go test -timeout 20m $(testArgs) $(if $(DISABLE_COVERAGE),, -cover) $(_testPackages) | tee $(buildDir)/test.sink.out
	@! grep -s -q -e "^FAIL" $(buildDir)/test.sink.out
.PHONY: benchmark
benchmark:
	@mkdir -p $(buildDir)
	go test $(testArgs) -timeout 30m -bench=$(benchPattern) $(if $(RUN_TEST),, -run=^^$$) | tee $(buildDir)/bench.sink.out
	@grep -s -q -e "^PASS" $(buildDir)/bench.sink.out
coverage:$(buildDir)/cover.out
	@go tool cover -func=$< | sed -E 's%github.com/.*/jasper/%%' | column -t
coverage-html:$(buildDir)/cover.html

$(buildDir):$(srcFiles) compile
	@mkdir -p $@
$(buildDir)/cover.out:$(buildDir) $(testFiles) .FORCE
	go test $(testArgs) -coverprofile $@ -cover $(_testPackages)
$(buildDir)/cover.html:$(buildDir)/cover.out
	go tool cover -html=$< -o $@

.FORCE:


proto:
	@mkdir -p rpc/internal
	protoc --go_out=plugins=grpc:rpc/internal *.proto
clean:
	rm -rf *.pb.go

vendor-clean:
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/evergreen-ci/gimlet
	rm -rf vendor/github.com/mongodb/amboy/vendor/golang.org/x/tools
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/satori/go.uuid/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/montanaflynn/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/
	rm -rf vendor/github.com/mongodb/grip/buildscripts/
	rm -rf vendor/github.com/tychoish/bond/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/tychoish/bond/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/tychoish/bond/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/tychoish/bond/vendor/github.com/mholt/archiver/
	rm -rf vendor/github.com/tychoish/bond/vendor/github.com/mongodb/amboy/
	rm -rf vendor/github.com/tychoish/bond/vendor/github.com/satori/go.uuid/
	rm -rf vendor/github.com/tychoish/lru/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/tychoish/lru/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/
	rm -rf vendor/github.com/mholt/archiver/rar.go
	rm -rf vendor/github.com/mholt/archiver/tarbz2.go
	rm -rf vendor/github.com/mholt/archiver/tarlz4.go
	rm -rf vendor/github.com/mholt/archiver/tarsz.go
	rm -rf vendor/github.com/mholt/archiver/tarxz.go
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
	find vendor -type d -empty | xargs rm -rf 

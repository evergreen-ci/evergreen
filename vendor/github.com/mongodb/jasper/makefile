name := jasper
buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
packages := $(name) cli rpc rest options mock testutil internal-executor
testPackages := $(packages)
projectPath := github.com/mongodb/jasper

_compilePackages := $(subst $(name),,$(subst -,/,$(foreach target,$(testPackages),./$(target))))
coverageOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage.html)

# start environment setup
gobin := $(GO_BIN_PATH)
ifeq (,$(gobin))
gobin := go
endif
gopath := $(GOPATH)
gocache := $(abspath $(buildDir)/.cache)
ifeq ($(OS),Windows_NT)
gocache := $(shell cygpath -m $(gocache))
gopath := $(shell cygpath -m $(gopath))
endif
goEnv := GOPATH=$(gopath) GOCACHE=$(gocache) $(if $(GO_BIN_PATH),PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)")
# end environment setup

compile:
	$(goEnv) $(gobin) build $(_compilePackages)
compile-base:
	$(goEnv) $(gobin) build  ./

# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	
coverage-%:$(buildDir)/output.%.coverage
	
html-coverage-%:$(buildDir)/output.%.coverage.html
	
lint-%:$(buildDir)/output.%.lint
	
# end convienence targets

# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=5m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gosec" --disable="gocyclo"
lintArgs += --enable="goimports" --enable="misspell" --enable="gofmt" --enable="ineffassign"

#  add and configure additional linters
lintArgs += --line-length=100 --dupl-threshold=175
#  golint doesn't handle splitting package comments between multiple files.
lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
#  no need to check the error of closer read operations in defer cases
lintArgs += --exclude="error return value not checked \(defer.*"
lintArgs += --exclude="should check returned error before deferring .*\.Close"
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(goEnv) $(gobin) get $(subst $(gopath)/src/,,$@)
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir) $(buildDir)/.lintSetup
	$(goEnv) $(gobin) build -o $@ $<
$(buildDir)/.lintSetup:$(lintDeps) $(buildDir)
	$(goEnv) $(gopath)/bin/gometalinter --install >/dev/null && touch $@
# end lint suppressions

# benchmark setup targets
$(buildDir)/run-benchmarks:cmd/run-benchmarks/run_benchmarks.go $(buildDir)
	$(goEnv) $(gobin) build -o $@ $<
# end benchmark setup targets

# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testArgs := -v
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count=$(RUN_COUNT)
endif
ifeq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
endif
# test execution and output handlers
$(buildDir):
	@mkdir -p $@
$(buildDir)/output.%.test:$(buildDir) .FORCE
	$(goEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@(! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@) || ! grep -s -q "no test files" $@
$(buildDir)/output.%.coverage:$(buildDir) .FORCE
	$(goEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(goEnv) $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(goEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir) .FORCE
	@$(goEnv) ./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
#  targets to process and generate coverage reports
# end test and coverage artifacts


# user-facing targets for basic build and development operations
proto:
	@mkdir -p rpc/internal
	protoc --go_out=plugins=grpc:rpc/internal *.proto
lint:$(buildDir) $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
	
test:$(buildDir) $(foreach target,$(testPackages),$(buildDir)/output.$(target).test)
	
benchmarks:$(buildDir)/run-benchmarks $(buildDir) .FORCE
	./$(buildDir)/run-benchmarks $(run-benchmark)
coverage:$(buildDir) $(coverageOutput)
coverage-html:$(buildDir) $(coverageHtmlOutput)
phony += lint $(buildDir) test coverage coverage-html
.PHONY: $(phony) .FORCE
.PRECIOUS:$(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(testPackages),$(buildDir)/output.$(target).test)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
# end front-ends

.FORCE:

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
	rm -rf vendor/github.com/evergreen-ci/certdepot/mgo_depot.go
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/vendor/golang.org/x/crypto
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/mongodb/amboy/vendor/golang.org/x/tools/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/go.mongodb.org/mongo-driver/
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
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/evergreen-ci/aviation/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/golang/protobuf/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/golang.org/x/net/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/golang.org/x/text/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/montanaflynn/stats/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pkg/errors/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/testify/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/crypto/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/net/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/sys/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/text/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/aviation/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/golang/protobuf/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/amboy/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/golang.org/x/net/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/golang.org/x/text/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/mongo-go-driver/mongo/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/pail/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
	find vendor/ -type d -empty | xargs rm -rf
	find vendor/ -type d -name '.git' | xargs rm -rf

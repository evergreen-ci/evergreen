buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

packages := ./ ./internal ./fetcher
lintPackages := timber fetcher
# override the go binary path if set
ifneq ($(GO_BIN_PATH),)
gobin := $(GO_BIN_PATH)
else
gobin := go
endif
gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
gopath := $(shell cygpath -m $(gopath))
endif
ifeq ($(gopath),)
gopath := $($(gobin) env GOPATH)
endif
goEnv := GOPATH=$(gopath)$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)")

# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=13m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gosec" --disable="gocyclo"
lintArgs += --disable="staticcheck" --disable="maligned"
lintArgs += --skip="build"
lintArgs += --exclude="rpc/internal/.*.pb.go"
#   enable and configure additional linters
lintArgs += --line-length=100 --dupl-threshold=150
#   some test cases are structurally similar, and lead to dupl linter
#   warnings, but are important to maintain separately, and would be
#   difficult to test without a much more complex reflection/code
#   generation approach, so we ignore dupl errors in tests.
lintArgs += --exclude="warning: duplicate of .*_test.go"
#   go lint warns on an error in docstring format, erroneously because
#   it doesn't consider the entire package.
lintArgs += --exclude="warning: package comment should be of the form \"Package .* ...\""
#   known issues that the linter picks up that are not relevant in our cases
lintArgs += --exclude="file is not goimported" # top-level mains aren't imported
lintArgs += --exclude="error return value not checked .defer.*"
lintArgs += --exclude="deadcode"
# end linting configuration


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(goEnv) $(gobin) get $(subst $(gopath)/src/,,$@)
# end dependency installation tools

# lint setup targets
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
$(buildDir)/.lintSetup:$(lintDeps)
	@mkdir -p $(buildDir)
	$(goEnv) $(gopath)/bin/gometalinter --force --install >/dev/null && touch $@
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) build -o $@ $<

# end lint setup targets

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

compile:
	$(goEnv) $(gobin) build $(packages)
race:
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) test $(testArgs) -race $(packages) | tee $(buildDir)/race.out
	@grep -s -q -e "^PASS" $(buildDir)/race.out && ! grep -s -q "^WARNING: DATA RACE" $(buildDir)/race.out
test:
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) test $(testArgs) $(if $(DISABLE_COVERAGE),, -cover) $(packages) | tee $(buildDir)/test.out
	@grep -s -q -e "^PASS" $(buildDir)/test.out
coverage:$(buildDir)/cover.out
	@$(goEnv) $(gobin) tool cover -func=$< | sed -E 's%github.com/.*/jasper/%%' | column -t
coverage-html:$(buildDir)/cover.html
lint:$(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
phony += lint lint-deps build build-race race test coverage coverage-html
.PRECIOUS:$(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint


$(buildDir):$(srcFiles) compile
	@mkdir -p $@
$(buildDir)/cover.out:$(buildDir) $(testFiles) .FORCE
	$(goEnv) $(gobin) test $(testArgs) -coverprofile $@ -cover $(packages)
$(buildDir)/cover.html:$(buildDir)/cover.out
	$(goEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(goEnv) ./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(goEnv) ./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(lintPackages)"
#  targets to process and generate coverage reports
# end test and coverage artifacts

.FORCE:


proto:vendor/cedar.proto
	@mkdir -p internal
	protoc --go_out=plugins=grpc:internal vendor/cedar.proto
	mv internal/vendor/cedar.pb.go internal/cedar.pb.go
	rm -rf internal/vendor
clean:
	rm -rf internal/*.pb.go
	rm -f vendor/cedar.proto

vendor/cedar.proto:
	curl -L https://raw.githubusercontent.com/evergreen-ci/cedar/master/buildlogger.proto -o $@
vendor:
	glide install -s


.PHONY:vendor
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/stretchr/testify/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pkg/errors/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/net/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/sys/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/text/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
	find vendor/ -name .git | xargs rm -rf

# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
html-coverage-%:$(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets

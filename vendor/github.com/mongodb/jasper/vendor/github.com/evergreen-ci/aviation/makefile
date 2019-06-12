buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

packages := aviation
#
# override the go binary path if set
ifneq (,$(GO_BIN_PATH))
gobin := $(GO_BIN_PATH)
else
gobin := go
endif


# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=13m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gosec" --disable="gocyclo" --enable="golint"
lintArgs += --enable="megacheck" --enable="unused" --enable="gosimple"
lintArgs += --skip="build"
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
# end linting configuration

# start dependency configuration
#   to avoid complicated vendoring, this package does not vendor its dependencies
deps := github.com/mongodb/grip
deps += github.com/evergreen-ci/gimlet
deps += github.com/stretchr/testify
#   grpc and dependencies
deps += google.golang.org/grpc
# end dependency configuration


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
gopath := $(shell $(gobin) env GOPATH)
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
deps := $(addprefix $(gopath)/src/,$(deps))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(gobin) get $(subst $(gopath)/src/,,$@)
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	 $(gobin) build -o $@ $<
$(buildDir)/.lintSetup:$(lintDeps)
	@-$(gopath)/bin/gometalinter --install >/dev/null && touch $@
# end dependency installation tools


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
ifneq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif
# test execution and output handlers
$(buildDir)/:
	mkdir -p $@
$(buildDir)/output.%.test:$(buildDir)/ $(deps) .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$*,) | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.test:$(buildDir)/ $(deps) .FORCE
	$(gobin) test $(testArgs) ./... | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.%.coverage:$(buildDir)/ $(deps) .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$*,) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(packages)"
#  targets to process and generate coverage reports
$(buildDir):$(srcFiles) compile
	@mkdir -p $@
$(buildDir)/cover.out:$(buildDir) $(testFiles) .FORCE
	$(gobin) test $(testArgs) -covermode=count -coverprofile $@ -cover ./
$(buildDir)/cover.html:$(buildDir)/cover.out
	$(gobin) tool cover -html=$< -o $@
# end test and coverage artifacts


# start vendoring configuration
#    begin with configuration of dependencies
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/errors/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
phony += vendor-clean
# end vendoring tooling configuration


# userfacing targets for basic build and development operations
compile:
	$(gobin) build ./
test:$(buildDir)/test.out
$(buildDir)/test.out:.FORCE
	@mkdir -p $(buildDir)
	$(gobin) test $(testArgs) ./ | tee $@
	@grep -s -q -e "^PASS" $@
coverage:$(buildDir)/cover.out
	@$(gobin) tool cover -func=$< | sed -E 's%github.com/.*/ftdc/%%' | column -t
coverage-html:$(buildDir)/cover.html

benchmark:
	$(gobin) test -v -benchmem -bench=. -run="Benchmark.*" -timeout=20m
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)

phony += lint lint-deps build build-race race test coverage coverage-html
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE

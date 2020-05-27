buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

testPackages := ./ ./services
packages := aviation services
#
# override the go binary path if set
ifneq (,$(GO_BIN_PATH))
gobin := $(GO_BIN_PATH)
else
gobin := go
endif


# start lint setup targets
lintDeps := $(buildDir)/golangci-lint $(buildDir)/.lintSetup $(buildDir)/run-linter
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	@mkdir -p $(buildDir)
	@touch $@
$(buildDir)/golangci-lint:
	@curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.10.2 >/dev/null 2>&1 && touch $@
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	@mkdir -p $(buildDir)
	$(gobin) build -o $@ $<
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
ifneq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif
# test execution and output handlers
$(buildDir)/output.%.test:$(buildDir) .FORCE
	$(gobin) test $(testArgs) $(testPackages)$(if $(subst $(name),,$*),$*,) | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.test:$(buildDir) .FORCE
	$(gobin) test $(testArgs) $(testPackages)... | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.%.coverage:$(buildDir) .FORCE
	$(gobin) test $(testArgs) $(testPackages)$(if $(subst $(name),,$*),$*,) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$(packages)'
#  targets to process and generate coverage reports
$(buildDir):$(srcFiles) compile
	@mkdir -p $@
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


clean:
	rm -rf $(lintDeps)


# userfacing targets for basic build and development operations
compile:
	$(gobin) build $(testPackages)
test:$(buildDir)/output.test
	
benchmark:
	$(gobin) test -v -benchmem -bench=. -run="Benchmark.*" -timeout=20m
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)

phony += lint build test coverage coverage-html
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE

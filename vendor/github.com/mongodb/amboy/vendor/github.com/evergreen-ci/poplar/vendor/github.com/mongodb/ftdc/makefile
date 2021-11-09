# start project configuration
name := ftdc
buildDir := build
packages := $(name) events hdrhist metrics util
orgPath := github.com/mongodb
projectPath := $(orgPath)/$(name)
# end project configuration

# test data downloads
metrics.ftdc:
	curl -LO "https://ftdc-test-files.s3.amazonaws.com/metrics.ftdc"
perf_metrics.ftdc:
	curl -LO "https://ftdc-test-files.s3.amazonaws.com/perf_metrics.ftdc"
$(buildDir)/output.ftdc.test:perf_metrics.ftdc metrics.ftdc
# end test files

# start environment setup
ifneq (,$(GO_BIN_PATH))
 gobin := $(GO_BIN_PATH)
else
 gobin := $(shell if [ -x /opt/golang/go1.9/bin/go ]; then echo /opt/golang/go1.9/bin/go; fi)
 ifeq (,$(gobin))
   gobin := go
 endif
endif

gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
 gobin := $(shell cygpath -m $(gobin))
 ifneq (,$(gopath))
  gopath := $(shell cygpath -m $(gopath))
 endif
endif
goEnv := GOPATH=$(gopath) $(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)")
# end environment setup

# start dependency installation tools
#   implementation details for being able to lazily install dependencies.
#   this block has no project specific configuration but defines
#   variables that project specific information depends on
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
raceOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).race)
testBin := $(foreach target,$(packages),$(buildDir)/test.$(target))
raceBin := $(foreach target,$(packages),$(buildDir)/race.$(target))
lintTargets := $(foreach target,$(packages),lint-$(target))
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "./scripts/*" -not -path "*\#*")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
# end dependency installation tools


# start lint setup targets
lintDeps := $(buildDir)/golangci-lint $(buildDir)/.lintSetup $(buildDir)/run-linter
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	@mkdir -p $(buildDir)
	@touch $@
$(buildDir)/golangci-lint:
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.10.2 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) build -o $@ $<
# end lint setup targets


# userfacing targets for basic build and development operations
build:$(srcFiles)
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) build $(subst $(name),,$(subst -,/,$(foreach pkg,$(packages),./$(pkg))))
test:$(testOutput)
lint:$(lintTargets)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
list-tests:
	@echo -e "test targets:" $(foreach target,$(packages),\\n\\ttest-$(target))
phony := lint build test coverage coverage-html
.PRECIOUS:$(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/test.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


# convenience targets for runing tests and coverage tasks on a
# specific package.
race-%:$(buildDir)/output.%.race
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
html-coverage-%:$(buildDir)/output.%.coverage $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start vendoring configuration
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
	find vendor/ -name '.git' | xargs rm -rf
	find vendor/ -type d -empty | xargs rm -rf
#   add phony targets
phony += vendor-clean
# end vendoring tooling configuration


# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testRunEnv := GOPATH=$(gopath)
testArgs := -v
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count=$(RUN_COUNT)
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
ifneq (,$(TEST_TIMEOUT))
testArgs += -timeout=$(TEST_TIMEOUT)
else
testArgs += -timeout=30m
endif
# testing targets
$(buildDir)/output.%.test: .FORCE
	@mkdir -p $(buildDir)
	$(goEnv) $(testRunEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
$(buildDir)/output.%.coverage: $(buildDir)/ .FORCE
	@mkdir -p $(buildDir)
	$(goEnv) $(testRunEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	$(goEnv) @-[ -f $@ ] && $(goEnv) $(testRunEnv) $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(goEnv) $(testRunEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintBin="$(buildDir)/golangci-lint" --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintBin="$(buildDir)/golangci-lint" --packages='$(packages)'
# end test and coverage artifacts


# clean and other utility targets
clean:
	rm -rf $(lintDeps) $(buildDir)/test.* $(buildDir)/coverage.* $(buildDir)/race.* $(clientBuildDir)
clean-results:
	rm -rf $(buildDir)/output.*
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE

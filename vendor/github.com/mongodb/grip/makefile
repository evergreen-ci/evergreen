# project configuration
name := grip
buildDir := build
packages := recovery logging message send slogger $(name)
orgPath := github.com/mongodb
projectPath := $(orgPath)/$(name)
# end project configuration

# lint setup targets
lintDeps := $(buildDir)/golangci-lint $(buildDir)/.lintSetup $(buildDir)/run-linter
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	@mkdir -p $(buildDir)
	@touch $@
$(buildDir)/golangci-lint:$(buildDir)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.23.8 >/dev/null 2>&1
$(buildDir)/run-linter:buildscripts/run-linter.go $(buildDir)/.lintSetup
	@mkdir -p $(buildDir)
	go build -o $@ $<
# end lint setup targets
#
######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
gopath := $(shell go env GOPATH)
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
raceOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).race)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
# end dependency installation tools


# userfacing targets for basic build and development operations
lint:$(buildDir)/output.lint
build: $(gopath)/src/$(projectPath)
	go build $(subst $(name),,$(subst -,/,$(foreach pkg,$(packages),./$(pkg))))
build-race: $(gopath)/src/$(projectPath)
	go build -race $(subst -,/,$(foreach pkg,$(packages),./$(pkg)))
test:$(testOutput)
race:$(raceOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := lint build build-race race test benchmark-send coverage coverage-html
.PRECIOUS:$(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/test.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/race.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


# implementation details for building the binary and creating a
# convienent link in the working directory
$(gopath)/src/$(orgPath):
	@mkdir -p $@
$(gopath)/src/$(projectPath):$(gopath)/src/$(orgPath)
	@[ -L $@ ] || ln -s $(shell pwd) $@
$(name):$(buildDir)/$(name)
	@[ -L $@ ] || ln -s $< $@
$(buildDir)/$(name):$(gopath)/src/$(projectPath)
	go build -o $@ main/$(name).go
$(buildDir)/$(name).race:$(gopath)/src/$(projectPath)
	go build -race -o $@ main/$(name).go

# convenience targets for runing tests and coverage tasks on a
# specific package.
makeArgs := --no-print-directory
race-%:$(buildDir)/output.%.race
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
benchmark-send:
	@mkdir -p build
	go test -v -bench=$(if $(RUN_BENCH),$(RUN_BENCH),BenchmarkAllSenders) ./send/ ./send/benchmark/ -run=^^$$
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
html-coverage-%:$(buildDir)/output.%.coverage $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testArgs := -test.v --test.timeout=5m
ifneq (,$(RUN_TEST))
testArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(RUN_CASE))
testArgs += -testify.m='$(RUN_CASE)'
endif
$(buildDir)/output.%.test: .FORCE
	@mkdir -p $(buildDir)
	go test $(if $(DISABLE_COVERAGE),,-covermode=count) $(testArgs) ./$(subst -,/,$*) | tee $@
$(buildDir)/output.$(name).test: .FORCE
	@mkdir -p $(buildDir)
	go test $(testArgs) ./ | tee $@

$(buildDir)/output.%.race: .FORCE
	@mkdir -p $(buildDir)
	go test -race $(testArgs) ./$(subst -,/,$*) 2>&1 | tee $@
$(buildDir)/output.$(name).race: .FORCE
	@mkdir -p $(buildDir)
	go test -race $(testArgs) ./ 2>&1 | tee $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter .FORCE
	@./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter .FORCE
	@./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$(packages)'
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage: .FORCE
	@mkdir -p $(buildDir)
	go test $(testArgs) -test.coverprofile=$@ | tee $(subst coverage,test,$@)
	@-[ -f $@ ] && go tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	@mkdir -p $(buildDir)
	go tool cover -html=$< -o $@
# end test and coverage artifacts


# start vendoring configuration
#    begin with configuration of dependencies
vendor-clean:
	rm -rf vendor/github.com/mattn/go-xmpp/_example/
	rm -rf vendor/github.com/bluele/slack/examples/
	sed -ri 's%(\tlog.*)%// \1%g' `find vendor/github.com/nutmegdevelopment/sumologic/upload/upload.go`
	sed -ri 's/json:"(.*)"/json:"\1" bson:"\1,omitempty"/' `grep -R -l 'json:\".*\" [^bson]' vendor/github.com/shirou/gopsutil/*` || true
	find vendor/ -name "*.go" | xargs gofmt -w -r '"golang.org/x/net/context" -> "context"'
	find vendor/github.com/shirou/gopsutil/ -name "*.go" | xargs -n 1 gofmt -w || true
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -fr
#   add phony targets
phony += vendor-clean
# end vendoring tooling configuration


# clean and other utility targets
clean:
	rm -rf $(name) $(lintDeps) $(buildDir)/output.*
phony += clean
# end dependency targets


# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE

# start project configuration
name := gimlet
buildDir := build
packages := $(name)
orgPath := github.com/tychoish
projectPath := $(orgPath)/$(name)
# end project configuration


# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=1m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gas"
lintArgs += --skip="$(buildDir)" --skip="buildscripts"
#  add and configure additional linters
lintArgs += --enable="go fmt -s" --enable="goimports"
lintArgs += --linter='misspell:misspell ./*.go:PATH:LINE:COL:MESSAGE' --enable=misspell
lintArgs += --line-length=100 --dupl-threshold=175 --cyclo-over=17
#  two similar functions triggered the duplicate warning, but they're not.
lintArgs += --exclude="duplicate of registry.go"
lintArgs += --exclude="don.t use underscores.*_DependencyState.*"
lintArgs += --exclude="file is not goimported" # test files aren't imported
#  golint doesn't handle splitting package comments between multiple files.
lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
lintArgs += --exclude "error return value not checked \(defer.*"
# end lint suppressions

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
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*")
testOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/test.$(target).out))
raceOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/race.$(target).out))
coverageOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/coverage.$(target).out))
coverageHtmlOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/coverage.$(target).html))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	go get $(subst $(gopath)/src/,,$@)
# end dependency installation tools
t:
	@echo $(testOutput)

# userfacing targets for basic build and development operations
lint:$(gopath)/src/$(projectPath) $(lintDeps)
	$(gopath)/bin/gometalinter $(lintArgs) ./...
lint-deps:$(lintDeps)
build:$(deps) $(srcFiles) $(gopath)/src/$(projectPath)
	go build ./.
build-race:$(deps) $(srcFiles) $(gopath)/src/$(projectPath)
	go build -race $(subst -,/,$(foreach pkg,$(packages),./$(pkg)))
test:$(testOutput)
race:$(raceOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := lint build build-race race test coverage coverage-html
phony += deps test-deps lint-deps
.PRECIOUS: $(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
# end front-ends


# implementation details for building the binary and creating a
# convienent link in the working directory
$(gopath)/src/$(orgPath):
	@mkdir -p $@
$(gopath)/src/$(projectPath):$(gopath)/src/$(orgPath)
	@[ -L $@ ] || ln -s $(shell pwd) $@
$(buildDir)/$(name):$(gopath)/src/$(projectPath) $(srcFiles) $(deps)
	go build -o $@ main/$(name).go
$(buildDir)/$(name).race:$(gopath)/src/$(projectPath) $(srcFiles) $(deps)
	go build -race -o $@ main/$(name).go
# end main build


# convenience targets for runing tests and coverage tasks on a
# specific package.
makeArgs := --no-print-directory
race-%:
	@$(MAKE) $(makeArgs) $(buildDir)/race.$(subst -,/,$*).out
	@grep -e "^PASS" $(buildDir)/race.$(subst /,-,$*).out
test-%:
	@$(MAKE) $(makeArgs) $(buildDir)/test.$(subst -,/,$*).out
	@grep -e "^PASS" $(buildDir)/test.$(subst /,-,$*).out
coverage-%:
	@$(MAKE) $(makeArgs) $(buildDir)/coverage.$(subst /,-,$*).out
html-coverage-%:
	@$(MAKE) $(makeArgs) $(buildDir)/coverage.$(subst /,-,$*).html
# end convienence targets


# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testRunDeps := $(testSrcFiles) build
testArgs := -v --timeout=1m
#    implementation for package coverage and test running,mongodb to produce
#    and save test output.
$(buildDir)/coverage.%.html:$(buildDir)/coverage.%.out
	go tool cover -html=$(buildDir)/coverage.$(subst /,-,$*).out -o $(buildDir)/coverage.$(subst /,-,$*).html
$(buildDir)/coverage.%.out:$(testRunDeps)
	go test $(testArgs) -covermode=count -coverprofile=$(buildDir)/coverage.$(subst /,-,$*).out $(projectPath)/$(subst -,/,$*)
	@-[ -f $(buildDir)/coverage.$(subst /,-,$*).out ] && go tool cover -func=$(buildDir)/coverage.$(subst /,-,$*).out | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/coverage.$(name).out:$(testRunDeps)
	go test -covermode=count -coverprofile=$@ $(projectPath)
	@-[ -f $@ ] && go tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/test.%.out:$(testRunDeps)
	go test $(testArgs) ./$(subst -,/,$*) | tee $(buildDir)/test.$(subst /,-,$*).out
$(buildDir)/race.%.out:$(testRunDeps)
	go test $(testArgs) -race ./$(subst -,/,$*) | tee $(buildDir)/race.$(subst /,-,$*).out
$(buildDir)/test.$(name).out:$(testRunDeps)
	go test $(testArgs) ./ | tee $@
$(buildDir)/race.$(name).out:$(testRunDeps)
	go test $(testArgs) -race ./ | tee $@
# end test and coverage artifacts


# start vendoring configuration
vendor-clean:
	rm -rf vendor/gopkg.in/mgo.v2/harness/
	rm -rf vendor/github.com/stretchr/testify/vendor/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/davecgh/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pmezard/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
phony += vendor-clean
# end vendoring tooling configuration


# clean and other utility targets
clean:
	rm -rf $(name) $(lintDeps) $(buildDir)/test.* $(buildDir)/coverage.* $(buildDir)/race.*
phony += clean
# end dependency targets

# configure phony targets
.PHONY:$(phony)

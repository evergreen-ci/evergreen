buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

_testPackages := ./ ./events ./metrics 
packages := _testPackages 

ifeq (,$(SILENT))
testArgs := -v
endif

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

ifneq (,$(RUN_BENCH))
benchArgs += -bench="$(RUN_BENCH)"
benchArgs += -run='$(RUN_BENCH)'
else
benchArgs += -bench=.
benchArgs += -run='Benchmark.*'
endif

# start environment setup
gopath := $(GOPATH)
gocache := $(abspath $(buildDir)/.cache)
ifeq ($(OS),Windows_NT)
gocache := $(shell cygpath -m $(gocache))
gopath := $(shell cygpath -m $(gopath))
endif
buildEnv := GOCACHE=$(gocache)
# end environment setup

# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=5m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gosec" --disable="gocyclo" --enable="goimports" --disable="golint"
lintArgs += --skip="$(buildDir)" --skip="buildscripts"
#  add and configure additional linters
lintArgs += --line-length=100 --dupl-threshold=175 --cyclo-over=30
#  golint doesn't handle splitting package comments between multiple files.
lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
#  no need to check the error of closer read operations in defer cases
lintArgs += --exclude="error return value not checked \(defer.*"
lintArgs += --exclude="should check returned error before deferring .*\.Close"
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	go get $(subst $(gopath)/src/,,$@)
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	$(buildEnv) go build -o $@ $<
$(buildDir)/.lintSetup:$(lintDeps)
	@mkdir -p $(buildDir)
	@-$(gopath)/bin/gometalinter --install >/dev/null && touch $@
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(packages)"
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end lint suppressions


compile:
	go build $(_testPackages)
test:metrics.ftdc perf_metrics.ftdc perf_metrics_small.ftdc
	@mkdir -p $(buildDir)
	go test $(testArgs) $(_testPackages) | tee $(buildDir)/test.ftdc.out
	@grep -s -q -e "^PASS" $(buildDir)/test.ftdc.out
coverage:$(buildDir)/cover.out
	@go tool cover -func=$< | sed -E 's%github.com/.*/ftdc/%%' | column -t
coverage-html:$(buildDir)/cover.html
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
benchmark:
	go test -v -benchmem $(benchArgs) -timeout=20m ./...


$(buildDir):$(srcFiles) compile
	@mkdir -p $@
$(buildDir)/cover.out:$(buildDir) $(testFiles) .FORCE
	go test $(testArgs) -covermode=count -coverprofile $@ -cover ./
$(buildDir)/cover.html:$(buildDir)/cover.out
	go tool cover -html=$< -o $@


test-%:
	@mkdir -p $(buildDir)
	go test $(testArgs) ./$* | tee $(buildDir)/test.*.out
	@grep -s -q -e "^PASS" $(buildDir)/test.*.out
coverage-%:$(buildDir)/cover.%.out
	@go tool cover -func=$< | sed -E 's%github.com/.*/ftdc/%%' | column -t
html-coverage-%:$(buildDir)/cover.%.html
$(buildDir)/cover.%.out:$(buildDir) $(testFiles) .FORCE
	go test $(testArgs) -covermode=count -coverprofile $@ -cover ./$*
$(buildDir)/cover.%.html:$(buildDir)/cover.%.out
	go tool cover -html=$< -o $@

.FORCE:



metrics.ftdc:
	wget "https://ftdc-test-files.s3.amazonaws.com/metrics.ftdc"
perf_metrics.ftdc:
	wget "https://ftdc-test-files.s3.amazonaws.com/perf_metrics.ftdc"
perf_metrics_small.ftdc:
	wget "https://ftdc-test-files.s3.amazonaws.com/perf_metrics_small.ftdc"

vendor-clean:
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/davecgh/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/testify/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pmezard/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/google/go-cmp/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/kr/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/
	rm -rf vendor/gopkg.in/mgo.v2/harness/
	rm -rf vendor/gopkg.in/mgo.v2/testdb/
	rm -rf vendor/gopkg.in/mgo.v2/testserver/
	rm -rf vendor/gopkg.in/mgo.v2/internal/json/testdata
	rm -rf vendor/gopkg.in/mgo.v2/.git/
	rm -rf vendor/gopkg.in/mgo.v2/txn/
	rm -rf vendor/github.com/papertrail/go-tail/vendor/github.com/spf13/pflag/
	rm -rf vendor/github.com/papertrail/go-tail/main.go
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf

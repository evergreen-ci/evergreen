buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*" -path "./bsonx*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
bsonxFiles := $(shell find ./bsonx -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

_testPackages := ./ ./events ./metrics ./bsonx

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


compile:
	go build $(_testPackages)
test:metrics.ftdc perf_metrics.ftdc perf_metrics_small.ftdc
	@mkdir -p $(buildDir)
	go test $(testArgs) $(_testPackages) | tee $(buildDir)/test.ftdc.out
	@grep -s -q -e "^PASS" $(buildDir)/test.ftdc.out
coverage:$(buildDir)/cover.out
	@go tool cover -func=$< | sed -E 's%github.com/.*/ftdc/%%' | column -t
coverage-html:$(buildDir)/cover.html $(buildDir)/cover.bsonx.html

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
html-coverage-%:coverage-%
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
	sed -ri 's/bson:"(.*),omitempty"/bson:"\1"/' `find vendor/github.com/mongodb/grip/vendor/github.com/shirou/gopsutil/ -name "*go"` || true
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf

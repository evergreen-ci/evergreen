module github.com/evergreen-ci/evergreen

go 1.24.0

require (
	github.com/99designs/gqlgen v0.17.86
	github.com/PuerkitoBio/rehttp v1.4.0
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/config v1.32.7
	github.com/aws/aws-sdk-go-v2/credentials v1.19.7
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.279.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.41.1
	github.com/aws/smithy-go v1.24.0
	github.com/cheynewallace/tabby v1.1.1
	github.com/docker/docker v28.5.2+incompatible
	github.com/docker/go-connections v0.6.0
	github.com/dustin/go-humanize v1.0.1
	github.com/evergreen-ci/birch v0.0.0-20250224221624-64f481f4b888
	github.com/evergreen-ci/certdepot v0.0.0-20251209180210-3f52e45cc5a2
	github.com/evergreen-ci/gimlet v0.0.0-20260113164336-bfe84f40e50d
	github.com/evergreen-ci/pail v0.0.0-20260205200523-4c59d67e678a
	github.com/evergreen-ci/poplar v0.0.0-20251209144431-fdec8d7b2505
	github.com/evergreen-ci/shrub v0.0.0-20251017154811-4d3ed1599154
	github.com/evergreen-ci/utility v0.0.0-20260116164328-250718d590d2
	github.com/gonzojive/httpcache v0.0.0-20220509000156-e80a5e6a69fe
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/sessions v1.3.0
	github.com/jpillora/backoff v1.0.0
	github.com/jpillora/longestcommon v0.0.0-20161227235612-adb9d91ee629
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/mongodb/amboy v0.0.0-20251209174146-73c46bb64973
	github.com/mongodb/anser v0.0.0-20251209174952-11a8088811aa
	github.com/mongodb/grip v0.0.0-20251203205830-b5c5c666ab94
	github.com/pkg/errors v0.9.1
	github.com/ravilushqa/otelgqlgen v0.19.0
	github.com/robbiet480/go.sns v0.0.0-20210223081447-c7c9eb6836cb
	github.com/robfig/cron v1.2.0
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06
	github.com/smartystreets/goconvey v1.8.1
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli v1.22.17
	github.com/vektah/gqlparser/v2 v2.5.31
	go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws v0.64.0
	go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux v0.64.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0
	go.opentelemetry.io/otel/metric v1.39.0
	go.opentelemetry.io/otel/sdk v1.39.0
	go.opentelemetry.io/otel/sdk/metric v1.39.0
	go.opentelemetry.io/otel/trace v1.39.0
	go.opentelemetry.io/proto/otlp v1.9.0
	golang.org/x/crypto v0.47.0
	golang.org/x/oauth2 v0.34.0
	golang.org/x/text v0.33.0
	golang.org/x/tools v0.40.0 // indirect
	gonum.org/v1/gonum v0.17.0
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Microsoft/go-winio v0.4.21 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/andygrunwald/go-jira v1.17.0
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.53.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/ses v1.34.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.42.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.6
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/go-systemd/v22 v22.6.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dghubble/oauth1 v0.7.3 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dsnet/compress v0.0.2-0.20230904184137-39efe44ab707 // indirect
	github.com/evergreen-ci/aviation v0.0.0-20260115180700-baca116d6d12 // indirect
	github.com/evergreen-ci/baobab v1.0.1-0.20220107150152-03b522479f52 // indirect
	github.com/evergreen-ci/bond v0.0.0-20251209195750-b541586174f7 // indirect
	github.com/evergreen-ci/lru v0.0.0-20251209201855-89a4cc8d867f // indirect
	github.com/evergreen-ci/negroni v1.0.1-0.20211028183800-67b6d7c2c035 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/fuyufjh/splunk-hec-go v0.4.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/godbus/dbus/v5 v5.2.0 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/compress v1.18.3 // indirect
	github.com/klauspost/pgzip v1.2.6
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/mattn/go-xmpp v0.0.1 // indirect
	github.com/mholt/archiver/v3 v3.5.1
	github.com/mongodb/ftdc v0.0.0-20251208183831-018e343a1aac // indirect
	github.com/nwaples/rardecode v1.1.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.3.0 // indirect
	github.com/patrickmn/go-cache v0.0.0-20180815053127-5633e0862627 // indirect
	github.com/peterhellberg/link v1.2.0 // indirect
	github.com/phyber/negroni-gzip v1.0.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.7 // indirect
	github.com/slack-go/slack v0.17.3 // indirect
	github.com/smarty/assertions v1.15.0 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/trivago/tgo v1.0.7 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/urfave/negroni v1.0.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/contrib v1.36.0 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0
	golang.org/x/sys v0.40.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)

require (
	github.com/aws/aws-sdk-go-v2/service/route53 v1.62.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.95.1
	github.com/aws/aws-sdk-go-v2/service/ssm v1.67.8
	github.com/bradleyfalzon/ghinstallation/v2 v2.17.0
	github.com/evergreen-ci/evg-lint v0.0.0-20251215145242-23eaa365e48f
	github.com/evergreen-ci/plank v0.0.0-20251203163536-53406252f581
	github.com/evergreen-ci/test-selection-client v0.0.0-20251016163227-83399b69e34c
	github.com/fraugster/parquet-go v0.11.0
	github.com/google/go-github/v70 v70.0.0
	github.com/gorilla/csrf v1.7.3
	github.com/gorilla/handlers v1.5.2
	github.com/kanopy-platform/kanopy-oidc-lib v0.1.3
	github.com/mongodb/jasper v0.0.0-20260115181313-e094cd64f89f
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/sirupsen/logrus v1.9.4
	go.opentelemetry.io/contrib/detectors/aws/ec2/v2 v2.1.0
	go.uber.org/automaxprocs v1.6.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/apache/thrift v0.16.0 // indirect
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.39.8 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/cgroups/v3 v3.1.2 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/coreos/go-oidc/v3 v3.16.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/google/go-github/v73 v73.0.0 // indirect
	github.com/google/go-github/v75 v75.0.0 // indirect
	github.com/google/go-github/v79 v79.0.0 // indirect
	github.com/lestrrat-go/httprc v1.0.6 // indirect
	github.com/lestrrat-go/jwx/v2 v2.1.6 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/okta/okta-jwt-verifier-golang/v2 v2.1.1 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/urfave/cli/v3 v3.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0 // indirect
	gopkg.in/go-jose/go-jose.v2 v2.6.3 // indirect
)

require (
	filippo.io/edwards25519 v1.0.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.21.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.17 // indirect
	github.com/coreos/go-oidc v2.5.0+incompatible
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/papertrail/go-tail v0.0.0-20180509224916-973c153b0431 // indirect
	github.com/pquerna/cachecontrol v0.2.0 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	go.mongodb.org/mongo-driver v1.17.8
	go.step.sm/crypto v0.31.0 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)

replace github.com/fraugster/parquet-go => github.com/evergreen-ci/parquet-go v0.0.0-20260116211725-cd13d4127a88

tool github.com/99designs/gqlgen

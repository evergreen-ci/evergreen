module github.com/evergreen-ci/evergreen

go 1.17

require (
	github.com/99designs/gqlgen v0.14.0
	github.com/PuerkitoBio/rehttp v1.1.0
	github.com/aws/aws-sdk-go v1.41.16
	github.com/cheynewallace/tabby v1.1.1
	github.com/docker/docker v17.12.0-ce-rc1.0.20200916142827-bd33bbf0497b+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/evergreen-ci/birch v0.0.0-20211025210128-7f3409c2b515
	github.com/evergreen-ci/certdepot v0.0.0-20211028185643-bccec0b3cb21
	github.com/evergreen-ci/cocoa v0.0.0-20211028185655-0a7d8e6e14e7
	github.com/evergreen-ci/gimlet v0.0.0-20211029160936-5b64c7b33753
	github.com/evergreen-ci/go-test2json v0.0.0-20180702150328-5b6cfd2e8cb0
	github.com/evergreen-ci/juniper v0.0.0-20210624185208-0fd21a88954c
	github.com/evergreen-ci/pail v0.0.0-20211028170419-8efd623fd305
	github.com/evergreen-ci/poplar v0.0.0-20211028171636-d45516ea1ce5
	github.com/evergreen-ci/shrub v0.0.0-20211025143051-a8d91b2e29fd
	github.com/evergreen-ci/timber v0.0.0-20211102205838-842ec45e96fb
	github.com/evergreen-ci/utility v0.0.0-20211026201827-97b21fa2660a
	github.com/google/go-github/v34 v34.0.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/gophercloud/gophercloud v0.1.0
	github.com/gorilla/csrf v1.7.1
	github.com/gorilla/sessions v1.2.1
	github.com/jpillora/backoff v1.0.0
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.4.2
	github.com/mongodb/amboy v0.0.0-20211101172957-63ecb128b0be
	github.com/mongodb/anser v0.0.0-20211101170237-60a39d433c32
	github.com/mongodb/ftdc v0.0.0-20211028165431-67f017692185
	github.com/mongodb/grip v0.0.0-20211101151816-abbea0c0d465
	github.com/pkg/errors v0.9.1
	github.com/robbiet480/go.sns v0.0.0-20210223081447-c7c9eb6836cb
	github.com/robfig/cron v1.2.0
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06
	github.com/shirou/gopsutil v3.21.10+incompatible
	github.com/smartystreets/goconvey v1.7.2
	github.com/stretchr/testify v1.7.0
	github.com/tychoish/tarjan v0.0.0-20170824211642-fcd3f3321826
	github.com/urfave/cli v1.22.5
	github.com/vektah/gqlparser/v2 v2.2.0
	github.com/vmware/govmomi v0.27.1
	go.mongodb.org/mongo-driver v1.7.3
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/oauth2 v0.0.0-20211028175245-ba495a64dcb5
	golang.org/x/tools v0.1.7
	gonum.org/v1/gonum v0.9.3
	google.golang.org/api v0.60.0
	google.golang.org/grpc v1.42.0
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

require (
	cloud.google.com/go v0.97.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20200615164410-66371956d46c // indirect
	github.com/Microsoft/go-winio v0.4.17 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/agnivade/levenshtein v1.1.0 // indirect
	github.com/andybalholm/brotli v1.0.3 // indirect
	github.com/andygrunwald/go-jira v1.14.0 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bluele/slack v0.0.0-20180528010058-b4b4d354a079 // indirect
	github.com/containerd/cgroups v1.0.2 // indirect
	github.com/containerd/containerd v1.5.7 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dghubble/oauth1 v0.7.0 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/evergreen-ci/aviation v0.0.0-20211026175554-41a4410c650f // indirect
	github.com/evergreen-ci/baobab v1.0.1-0.20211025210153-3206308845c1 // indirect
	github.com/evergreen-ci/lru v0.0.0-20211029170532-008d075b972d // indirect
	github.com/evergreen-ci/mrpc v0.0.0-20211025143107-842bca81a3f8 // indirect
	github.com/evergreen-ci/negroni v1.0.1-0.20211028183800-67b6d7c2c035 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/fuyufjh/splunk-hec-go v0.3.4-0.20210909061418-feecd03924b7 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.1 // indirect
	github.com/go-ldap/ldap/v3 v3.4.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/goccy/go-json v0.3.5 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-github v17.0.0+incompatible // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gax-go/v2 v2.1.1 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20181017120253-0766667cb4d1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.7 // indirect
	github.com/lestrrat-go/httpcc v1.0.0 // indirect
	github.com/lestrrat-go/iter v1.0.0 // indirect
	github.com/lestrrat-go/jwx v1.1.1 // indirect
	github.com/lestrrat-go/option v1.0.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-xmpp v0.0.0-20211029151415-912ba614897a // indirect
	github.com/mholt/archiver v2.0.1-0.20180417220235-e4ef56d48eb0+incompatible
	github.com/mholt/archiver/v3 v3.5.0 // indirect
	github.com/mongodb/jasper v0.0.0-20211103140906-f5e338d1959b
	github.com/nwaples/rardecode v1.1.2 // indirect
	github.com/okta/okta-jwt-verifier-golang v1.1.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/papertrail/go-tail v0.0.0-20180509224916-973c153b0431 // indirect
	github.com/patrickmn/go-cache v0.0.0-20180815053127-5633e0862627 // indirect
	github.com/peterhellberg/link v1.1.0 // indirect
	github.com/phyber/negroni-gzip v1.0.0 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.9 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rs/cors v1.8.0 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shirou/gopsutil/v3 v3.21.10 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/smartystreets/assertions v1.2.0 // indirect
	github.com/square/certstrap v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	github.com/tklauser/numcpus v0.3.0 // indirect
	github.com/trivago/tgo v1.0.7 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/urfave/negroni v1.0.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.0.2 // indirect
	github.com/xdg-go/stringprep v1.0.2 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/youmark/pkcs8 v0.0.0-20201027041543-1326539a0a0a // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/exp v0.0.0-20200224162631-6cc2880d07d6 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/net v0.0.0-20211029224645-99673261e6eb // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20211031064116-611d5d643895 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20211101144312-62acf1d99145 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

require github.com/evergreen-ci/bond v0.0.0-20211101153628-3fabd9ffaead // indirect

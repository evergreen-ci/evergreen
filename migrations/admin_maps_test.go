package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type adminMapSuite struct {
	migrationSuite
}

func TestAdminMapMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := db.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &adminMapSuite{}
	s.env = &mock.Environment{}
	s.session = session
	s.database = database.Name
	s.cancel = cancel

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *adminMapSuite) SetupTest() {
	s.NoError(evgdb.ClearCollections(adminCollection))

	data := db.Document{
		"_id": globalDocId,
		"credentials": db.Document{
			"cred1key": "cred1val",
		},
		"expansions": db.Document{
			"exp1key": "exp1val",
		},
		"keys": db.Document{
			"key1key": "key1val",
		},
		"plugins": db.Document{
			"plugin1": db.Document{
				"plugin1key": db.Document{
					"stringField": "stringVal",
					"arrayField":  []string{"one", "two"},
				},
			},
		},
	}
	s.NoError(evgdb.Insert(adminCollection, data))
}

func (s *adminMapSuite) TestMigration() {
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 10,
		id:    "migration-admin_map_restructure",
	}

	gen, err := adminMapRestructureGenerator(anser.GetEnvironment(), args)
	s.NoError(err)
	gen.Run(context.Background())
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run(context.Background())
		s.NoError(j.Error())
	}

	var configs []db.Document
	err = evgdb.FindAllQ(adminCollection, evgdb.Q{}, &configs)
	s.NoError(err)
	s.Require().Len(configs, 1)
	config := configs[0]

	credentials := config["credentials_new"].([]interface{})
	credential := credentials[0].(db.Document)
	s.Equal("cred1key", credential["key"])
	s.Equal("cred1val", credential["value"])

	expansions := config["expansions_new"].([]interface{})
	expansion := expansions[0].(db.Document)
	s.Equal("exp1key", expansion["key"])
	s.Equal("exp1val", expansion["value"])

	keys := config["keys_new"].([]interface{})
	key := keys[0].(db.Document)
	s.Equal("key1key", key["key"])
	s.Equal("key1val", key["value"])

	plugins := config["plugins_new"].([]interface{})
	plugin := plugins[0].(db.Document)
	s.Equal("plugin1", plugin["key"])
	pluginConfigs := plugin["value"].([]interface{})
	pluginConfig := pluginConfigs[0].(db.Document)
	s.Equal("plugin1key", pluginConfig["key"])
	s.EqualValues(db.Document{
		"stringField": "stringVal",
		"arrayField":  []interface{}{"one", "two"},
	}, pluginConfig["value"])
}

func (s *adminMapSuite) TearDownSuite() {
	s.cancel()
}

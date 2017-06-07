package data

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

var numDistros = 10

func TestFindAllDistros(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindAllDistros")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(session.DB(testConfig.Database.DB).DropDatabase(), t, "Error dropping database")

	sc := &DBConnector{}

	var distros []*distro.Distro
	for i := 0; i < numDistros; i++ {
		d := &distro.Distro{
			Id: fmt.Sprintf("distro_%d", rand.Int()),
		}
		distros = append(distros, d)
		assert.Nil(t, d.Insert())
	}

	found, err := sc.FindAllDistros()
	assert.Nil(t, err)
	assert.Len(t, found, numDistros)
}

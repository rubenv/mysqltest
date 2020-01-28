package mysqltest_test

import (
	"testing"

	"github.com/rubenv/mysqltest"
	"github.com/stretchr/testify/assert"
)

func TestPostgreSQL(t *testing.T) {
	assert := assert.New(t)

	pg, err := mysqltest.Start()
	assert.NoError(err)
	assert.NotNil(pg)

	_, err = pg.DB.Exec("CREATE TABLE test (val text)")
	assert.NoError(err)

	err = pg.Stop()
	assert.NoError(err)
}

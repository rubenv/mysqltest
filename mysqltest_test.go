package mysqltest_test

import (
	"testing"

	"github.com/rubenv/mysqltest"
	"github.com/stretchr/testify/assert"
)

func TestMySQL(t *testing.T) {
	assert := assert.New(t)

	mysql, err := mysqltest.Start()
	assert.NoError(err)
	assert.NotNil(mysql)

	_, err = mysql.DB.Exec("CREATE TABLE test (val text)")
	assert.NoError(err)

	err = mysql.Stop()
	assert.NoError(err)
}

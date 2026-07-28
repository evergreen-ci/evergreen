package artifact

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateAndValidateSignToken(t *testing.T) {
	secret := []byte("my-app-secret")
	taskID := "task123"
	execution := 2
	fileName := "report.html"

	t.Run("ValidToken", func(t *testing.T) {
		token, expiry := GenerateSignToken(secret, taskID, execution, fileName)
		assert.True(t, ValidateSignToken(secret, taskID, execution, fileName, token, strconv.FormatInt(expiry, 10)))
	})

	t.Run("WrongSecret", func(t *testing.T) {
		token, expiry := GenerateSignToken(secret, taskID, execution, fileName)
		assert.False(t, ValidateSignToken([]byte("wrong-secret"), taskID, execution, fileName, token, strconv.FormatInt(expiry, 10)))
	})

	t.Run("ExpiredToken", func(t *testing.T) {
		key := deriveKey(secret)
		pastExpiry := time.Now().Add(-1 * time.Hour).Unix()
		expiredToken := computeMAC(key, taskID, execution, fileName, pastExpiry)
		assert.False(t, ValidateSignToken(secret, taskID, execution, fileName, expiredToken, strconv.FormatInt(pastExpiry, 10)))
	})
}

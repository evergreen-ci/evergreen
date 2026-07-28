package artifact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
)

const artifactSignPurpose = "artifact-sign"

func deriveKey(appSecret []byte) []byte {
	mac := hmac.New(sha256.New, appSecret)
	mac.Write([]byte(artifactSignPurpose))
	return mac.Sum(nil)
}

func computeMAC(key []byte, taskID string, execution int, fileName string, expiry int64) string {
	msg := fmt.Sprintf("%s\n%d\n%s\n%d", taskID, execution, fileName, expiry)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateSignToken creates a token and expiry for an artifact sign URL.
// The token expires after PresignMinimumValidTime.
func GenerateSignToken(appSecret []byte, taskID string, execution int, fileName string) (token string, expiry int64) {
	key := deriveKey(appSecret)
	expiry = time.Now().Add(evergreen.PresignMinimumValidTime).Unix()
	token = computeMAC(key, taskID, execution, fileName, expiry)
	return token, expiry
}

// ValidateSignToken checks that the token is valid and not expired.
func ValidateSignToken(appSecret []byte, taskID string, execution int, fileName string, token string, expiryStr string) bool {
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expiry {
		return false
	}
	key := deriveKey(appSecret)
	expected := computeMAC(key, taskID, execution, fileName, expiry)
	return hmac.Equal([]byte(expected), []byte(token))
}

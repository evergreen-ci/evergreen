package verify

import (
	"github.com/lestrrat-go/jwx/jwa"
	"testing"
)

func TestHMACVerify(t *testing.T) {
	type dummyStruct struct {
		dummy1 int
		dummy2 float64
	}
	dummy := &dummyStruct{1, 3.4}
	t.Run("HMAC Verifier Creation Error", func(t *testing.T) {
		_, err := newHMAC(jwa.ES256)
		if err == nil {
			t.Fatal("HMAC Verifier Object creation should fail")
		}
	})
	t.Run("HMAC Verifier Sign Error", func(t *testing.T) {
		pVerifier, err := newHMAC(jwa.HS512)
		if err != nil {
			t.Fatalf("Signer creation failure: %v", jwa.HS512)
		}
		err = pVerifier.Verify([]byte("payload"), []byte("signature"), dummy)
		if err == nil {
			t.Fatal("HMAC Verification should fail")
		}

	})
}

package aviation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestCertificateUserValidationInterceptors(t *testing.T) {
	const username = "testUser"

	user := gimlet.NewBasicUser(username, "test", "test@test.com", "abc123", nil)
	um, err := gimlet.NewBasicUserManager([]gimlet.User{user})
	require.NoError(t, err)

	for _, test := range []struct {
		name string
		ctx  context.Context
		err  bool
	}{
		{
			name: "ValidUser",
			ctx:  peer.NewContext(context.TODO(), newPeer(username)),
		},
		{
			name: "MissingPeer",
			ctx:  context.TODO(),
			err:  true,
		},
		{
			name: "NilTLS",
			ctx:  peer.NewContext(context.TODO(), &peer.Peer{}),
			err:  true,
		},
		{
			name: "UserDNE",
			ctx:  peer.NewContext(context.TODO(), newPeer("DNE")),
			err:  true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Run("Unary", func(t *testing.T) {
				interceptor := MakeCertificateUserValidationUnaryInterceptor(um)
				_, err = interceptor(test.ctx, nil, nil, mockUnaryHandler)

				if test.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
			t.Run("Stream", func(t *testing.T) {
				interceptor := MakeCertificateUserValidationStreamInterceptor(um)
				err = interceptor(nil, &mockServerStream{ctx: test.ctx}, nil, mockStreamHandler)

				if test.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})

		})
	}
}

func newPeer(username string) *peer.Peer {
	return &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{
						{
							Subject: pkix.Name{
								CommonName: username,
							},
						},
					},
				},
			},
		},
	}
}

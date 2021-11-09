package certdepot

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/square/certstrap/pkix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	tempDir, err := ioutil.TempDir(".", "cert-test")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tempDir))
	}()
	d, err := NewFileDepot(tempDir)
	require.NoError(t, err)

	opts := &CertificateOptions{
		Organization:       "mongodb",
		Country:            "USA",
		Locality:           "NYC",
		OrganizationalUnit: "evergreen",
		Province:           "Manhattan",
		Expires:            24 * time.Hour,

		// irrelevant information should be ignored
		IP:     []string{"0.0.0.0"},
		Domain: []string{"evergreen"},
		//URI:          []string{"evergreen.mongodb.com"},
		Host:         "evergreen",
		CA:           "ca",
		CAPassphrase: "passphrase",
		Intermediate: true,
	}

	for _, test := range []struct {
		name       string
		changeOpts func()
		keyTest    func()
		hasErr     bool
	}{
		{
			name:       "NoCommonName",
			changeOpts: func() {},
			hasErr:     true,
		},
		{
			name:       "NewCA",
			changeOpts: func() { opts.CommonName = "ca" },
			keyTest: func() {
				var key *pkix.Key

				key, err = GetPrivateKey(d, opts.CommonName)
				require.NoError(t, err)
				privKey, ok := key.Private.(*rsa.PrivateKey)
				require.True(t, ok)
				assert.True(t, privKey.D.BitLen() <= 2048)
			},
		},
		{
			name: "ExistingKey",
			changeOpts: func() {
				var existingKey *pkix.Key
				var data []byte

				existingKey, err = pkix.CreateRSAKey(2048)
				require.NoError(t, err)
				data, err = existingKey.ExportPrivate()
				require.NoError(t, err)
				keyFile := filepath.Join(tempDir, "ca2key")
				require.NoError(t, ioutil.WriteFile(keyFile, data, 0777))
				opts.CommonName = "ca2"
				opts.Key = keyFile
			},
			keyTest: func() {
				var existingKey, key *pkix.Key
				var data []byte

				data, err = ioutil.ReadFile(opts.Key)
				require.NoError(t, err)
				existingKey, err = pkix.NewKeyFromPrivateKeyPEM(data)
				require.NoError(t, err)
				key, err = GetPrivateKey(d, opts.CommonName)
				require.NoError(t, err)
				assert.Equal(t, existingKey, key)
			},
		},
		{
			name: "NonDefaultKeySize",
			changeOpts: func() {
				opts.CommonName = "ca3"
				opts.KeyBits = 1024
				opts.Key = ""
			},
			keyTest: func() {
				var key *pkix.Key

				key, err = GetPrivateKey(d, opts.CommonName)
				require.NoError(t, err)
				privKey, ok := key.Private.(*rsa.PrivateKey)
				require.True(t, ok)
				assert.True(t, privKey.D.BitLen() <= opts.KeyBits)
			},
		},
		{
			name: "Passphrase",
			changeOpts: func() {
				opts.CommonName = "ca4"
				opts.KeyBits = 0
				opts.Passphrase = "passphrase"
			},
			keyTest: func() {
				_, err = GetPrivateKey(d, opts.CommonName)
				assert.Error(t, err)
				_, err = GetEncryptedPrivateKey(d, opts.CommonName, []byte(opts.Passphrase))
				assert.NoError(t, err)

			},
		},
		{
			name: "AlreadyExistingCA",
			changeOpts: func() {
				opts.CommonName = "ca"
				opts.Passphrase = ""
			},
			hasErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.changeOpts()
			err = opts.Init(d)

			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				rawCert, err := getRawCertificate(d, opts.CommonName)
				require.NoError(t, err)
				assert.True(t, rawCert.IsCA)
				assert.Equal(t, opts.CommonName, rawCert.Subject.CommonName)
				assert.Equal(t, []string{opts.Organization}, rawCert.Subject.Organization)
				assert.Equal(t, []string{opts.Country}, rawCert.Subject.Country)
				assert.Equal(t, []string{opts.Locality}, rawCert.Subject.Locality)
				assert.Equal(t, []string{opts.OrganizationalUnit}, rawCert.Subject.OrganizationalUnit)
				assert.Equal(t, []string{opts.Province}, rawCert.Subject.Province)
				assert.Empty(t, rawCert.IPAddresses)
				assert.Empty(t, rawCert.DNSNames)
				//assert.Empty(t, rawCert.URIs)
				assert.Equal(t, rawCert.Subject, rawCert.Issuer)
				assert.True(t, rawCert.NotBefore.Before(time.Now()))
				assert.True(t, rawCert.NotAfter.After(time.Now().Add(23*time.Hour)))
				assert.True(t, rawCert.NotAfter.Before(time.Now().Add(25*time.Hour)))

				test.keyTest()
			}
		})
	}
}

func TestCertRequest(t *testing.T) {
	tempDir, err := ioutil.TempDir(".", "cert-test")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tempDir))
	}()
	d, err := NewFileDepot(tempDir)
	require.NoError(t, err)

	opts := &CertificateOptions{
		Organization:       "mongodb",
		Country:            "USA",
		Locality:           "NYC",
		OrganizationalUnit: "evergreen",
		Province:           "Manhattan",
		Expires:            24 * time.Hour,
		IP:                 []string{"0.0.0.0", "1.1.1.1"},
		//URI:                []string{"https://www.evergreen.mongodb.com", "https://www.cedar.mongodb.com"},

		// irrelevant information should be ignored
		Host:         "evergreen",
		CA:           "ca",
		CAPassphrase: "passphrase",
		Intermediate: true,
	}

	for _, test := range []struct {
		name       string
		csrName    string
		changeOpts func()
		keyTest    func()
		hasErr     bool
	}{
		{
			name:       "NoCommonNameOrDomain",
			changeOpts: func() {},
			hasErr:     true,
		},
		{
			name:    "NewCSRWithOutCommonName",
			csrName: "evergreen",
			changeOpts: func() {
				opts.Domain = []string{"evergreen"}
			},
			keyTest: func() {
				var key *pkix.Key

				key, err = GetPrivateKey(d, opts.Domain[0])
				require.NoError(t, err)
				privKey, ok := key.Private.(*rsa.PrivateKey)
				require.True(t, ok)
				assert.True(t, privKey.D.BitLen() <= 2048)
			},
		},
		{
			name:    "NewCSRWithCommonName",
			csrName: "test",
			changeOpts: func() {
				opts.CommonName = "test"
			},
			keyTest: func() {
				var key *pkix.Key

				key, err = GetPrivateKey(d, opts.CommonName)
				require.NoError(t, err)
				privKey, ok := key.Private.(*rsa.PrivateKey)
				require.True(t, ok)
				assert.True(t, privKey.D.BitLen() <= 2048)
			},
		},
		{
			name:    "ExistingKey",
			csrName: "test2",
			changeOpts: func() {
				var existingKey *pkix.Key
				var data []byte

				existingKey, err = pkix.CreateRSAKey(2048)
				require.NoError(t, err)
				data, err = existingKey.ExportPrivate()
				require.NoError(t, err)
				keyFile := filepath.Join(tempDir, "test2key")
				require.NoError(t, ioutil.WriteFile(keyFile, data, 0777))
				opts.CommonName = "test2"
				opts.Key = keyFile
			},
			keyTest: func() {
				var existingKey, key *pkix.Key
				var data []byte

				data, err = ioutil.ReadFile(opts.Key)
				require.NoError(t, err)
				existingKey, err = pkix.NewKeyFromPrivateKeyPEM(data)
				require.NoError(t, err)
				key, err = GetPrivateKey(d, opts.CommonName)
				require.NoError(t, err)
				assert.Equal(t, existingKey, key)
			},
		},
		{
			name:    "NonDefaultKeySize",
			csrName: "test3",
			changeOpts: func() {
				opts.CommonName = "test3"
				opts.KeyBits = 1024
				opts.Key = ""
			},
			keyTest: func() {
				var key *pkix.Key

				key, err = GetPrivateKey(d, opts.CommonName)
				require.NoError(t, err)
				privKey, ok := key.Private.(*rsa.PrivateKey)
				require.True(t, ok)
				assert.True(t, privKey.D.BitLen() <= opts.KeyBits)
			},
		},
		{
			name:    "Passphrase",
			csrName: "test4",
			changeOpts: func() {
				opts.CommonName = "test4"
				opts.KeyBits = 0
				opts.Passphrase = "passphrase"
			},
			keyTest: func() {
				_, err = GetPrivateKey(d, opts.CommonName)
				assert.Error(t, err)
				_, err = GetEncryptedPrivateKey(d, opts.CommonName, []byte(opts.Passphrase))
				assert.NoError(t, err)
			},
		},
		{
			name: "AlreadyExistingCSR",
			changeOpts: func() {
				opts.CommonName = "test"
				opts.Passphrase = ""
			},
			hasErr: true,
		},
		{
			name: "InvalidIps",
			changeOpts: func() {
				opts.IP = []string{"invalid"}
			},
			hasErr: true,
		},
		/*
			{
				name: "InvalidURIs",
				changeOpts: func() {
					opts.IP = nil
					opts.URI = []string{"invalid"}
				},
				hasErr: true,
			},
		*/
	} {
		t.Run(test.name, func(t *testing.T) {
			opts.Reset()
			defer opts.Reset()
			test.changeOpts()
			err = opts.CertRequest(d)

			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				csr, err := GetCertificateSigningRequest(d, test.csrName)
				require.NoError(t, err)
				rawCSR, err := csr.GetRawCertificateSigningRequest()
				require.NoError(t, err)
				assert.Equal(t, test.csrName, rawCSR.Subject.CommonName)
				assert.Equal(t, []string{opts.Organization}, rawCSR.Subject.Organization)
				assert.Equal(t, []string{opts.Country}, rawCSR.Subject.Country)
				assert.Equal(t, []string{opts.Locality}, rawCSR.Subject.Locality)
				assert.Equal(t, []string{opts.OrganizationalUnit}, rawCSR.Subject.OrganizationalUnit)
				assert.Equal(t, []string{opts.Province}, rawCSR.Subject.Province)
				assert.Equal(t, convertIPs(opts.IP), rawCSR.IPAddresses)
				//assert.Equal(t, convertURIs(opts.URI), rawCSR.URIs)
				assert.Equal(t, opts.Domain, rawCSR.DNSNames)

				test.keyTest()
			}
		})
	}
}

func TestInMemory(t *testing.T) {
	// Resets depot state.
	clearByName := func(d Depot, name string) error {
		catcher := grip.NewBasicCatcher()
		if csrTag := CsrTag(name); d.Check(csrTag) {
			catcher.Add(d.Delete(csrTag))
		}
		if keyTag := PrivKeyTag(name); d.Check(keyTag) {
			catcher.Add(d.Delete(keyTag))
		}
		if crtTag := CrtTag(name); d.Check(crtTag) {
			catcher.Add(d.Delete(crtTag))
		}
		return catcher.Resolve()
	}

	for testName, testCase := range map[string]func(t *testing.T, d Depot, opts *CertificateOptions){
		"CertRequestInMemory": func(t *testing.T, d Depot, opts *CertificateOptions) {
			checkMatchingCSR := func(t *testing.T, opts *CertificateOptions, rawCSR *x509.CertificateRequest) {
				assert.Equal(t, opts.CommonName, rawCSR.Subject.CommonName)
				assert.Equal(t, []string{opts.Organization}, rawCSR.Subject.Organization)
				assert.Equal(t, []string{opts.Country}, rawCSR.Subject.Country)
				assert.Equal(t, []string{opts.Locality}, rawCSR.Subject.Locality)
				assert.Equal(t, []string{opts.OrganizationalUnit}, rawCSR.Subject.OrganizationalUnit)
				assert.Equal(t, []string{opts.Province}, rawCSR.Subject.Province)
				assert.Equal(t, convertIPs(opts.IP), rawCSR.IPAddresses)
				assert.Equal(t, opts.Domain, rawCSR.DNSNames)
			}

			exportCSRAndKey := func(t *testing.T, csr *pkix.CertificateSigningRequest, key *pkix.Key) ([]byte, []byte) {
				pemCSR, err := csr.Export()
				require.NoError(t, err)
				pemKey, err := key.ExportPrivate()
				require.NoError(t, err)
				return pemCSR, pemKey
			}

			for subTestName, subTestCase := range map[string]func(t *testing.T, name string){
				"Succeeds": func(t *testing.T, name string) {
					csr, _, err := opts.CertRequestInMemory()
					require.NoError(t, err)

					rawCSR, err := csr.GetRawCertificateSigningRequest()
					require.NoError(t, err)

					checkMatchingCSR(t, opts, rawCSR)

					assert.False(t, CheckCertificateSigningRequest(d, name))
					assert.False(t, CheckPrivateKey(d, name))
				},
				"SucceedsAfterCertRequestInDepot": func(t *testing.T, name string) {
					require.NoError(t, opts.CertRequest(d))
					csr, key, err := opts.CertRequestInMemory()
					require.NoError(t, err)

					pemCSR, pemKey := exportCSRAndKey(t, csr, key)

					dbCSR, err := d.Get(CsrTag(name))
					require.NoError(t, err)

					dbKey, err := d.Get(PrivKeyTag(name))
					require.NoError(t, err)

					assert.Equal(t, dbCSR, pemCSR)
					assert.Equal(t, dbKey, pemKey)
				},
				"ReturnsIdenticalAsSubsequentCertRequest": func(t *testing.T, name string) {
					csr, key, err := opts.CertRequestInMemory()
					require.NoError(t, err)
					pemCSR, pemKey := exportCSRAndKey(t, csr, key)

					require.NoError(t, opts.CertRequest(d))
					dbCSR, err := d.Get(CsrTag(name))
					require.NoError(t, err)
					dbKey, err := d.Get(PrivKeyTag(name))
					require.NoError(t, err)

					assert.Equal(t, dbCSR, pemCSR)
					assert.Equal(t, dbKey, pemKey)
				},
				"ReturnsIdenticalOnSubsequentCalls": func(t *testing.T, name string) {
					csr, key, err := opts.CertRequestInMemory()
					require.NoError(t, err)
					pemCSR, pemKey := exportCSRAndKey(t, csr, key)

					sameCSR, sameKey, err := opts.CertRequestInMemory()
					require.NoError(t, err)
					pemSameCSR, pemSameKey := exportCSRAndKey(t, sameCSR, sameKey)

					assert.Equal(t, pemCSR, pemSameCSR)
					assert.Equal(t, pemKey, pemSameKey)
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					name, err := opts.getFormattedCertificateRequestName()
					require.NoError(t, err)
					opts.Reset()
					require.NoError(t, clearByName(d, name))
					defer func() {
						opts.Reset()
						assert.NoError(t, clearByName(d, name))
					}()

					subTestCase(t, name)
				})
			}
		},
		"PutCertRequest": func(t *testing.T, d Depot, opts *CertificateOptions) {
			for subTestName, subTestCase := range map[string]func(t *testing.T, name string){
				"FailsWithoutCertRequestInMemory": func(t *testing.T, name string) {
					assert.Error(t, opts.PutCertRequestFromMemory(d))
				},
				"FailsAfterCertRequest": func(t *testing.T, name string) {
					require.NoError(t, opts.CertRequest(d))
					assert.Error(t, opts.PutCertRequestFromMemory(d))
				},
				"SucceedsAfterCertRequestInMemory": func(t *testing.T, name string) {
					csr, key, err := opts.CertRequestInMemory()
					require.NoError(t, err)
					require.NoError(t, opts.PutCertRequestFromMemory(d))

					pemCSR, err := csr.Export()
					require.NoError(t, err)
					depotCSR, err := d.Get(CsrTag(name))
					require.NoError(t, err)
					assert.Equal(t, pemCSR, depotCSR)

					pemKey, err := key.ExportPrivate()
					require.NoError(t, err)
					depotKey, err := d.Get(PrivKeyTag(name))
					require.NoError(t, err)
					assert.Equal(t, pemKey, depotKey)
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					name, err := opts.getFormattedCertificateRequestName()
					require.NoError(t, err)
					opts.Reset()
					require.NoError(t, clearByName(d, name))
					defer func() {
						opts.Reset()
						assert.NoError(t, clearByName(d, name))
					}()

					subTestCase(t, name)
				})
			}
		},
		"SignInMemory": func(t *testing.T, d Depot, opts *CertificateOptions) {
			checkMatchingCert := func(t *testing.T, opts *CertificateOptions, rawCrt *x509.Certificate) {
				assert.Equal(t, opts.CommonName, rawCrt.Subject.CommonName)
				assert.Equal(t, []string{opts.Organization}, rawCrt.Subject.Organization)
				assert.Equal(t, []string{opts.Country}, rawCrt.Subject.Country)
				assert.Equal(t, []string{opts.Locality}, rawCrt.Subject.Locality)
				assert.Equal(t, []string{opts.OrganizationalUnit}, rawCrt.Subject.OrganizationalUnit)
				assert.Equal(t, []string{opts.Province}, rawCrt.Subject.Province)
				assert.Equal(t, convertIPs(opts.IP), rawCrt.IPAddresses)
				assert.Equal(t, opts.Domain, rawCrt.DNSNames)
			}

			for subTestName, subTestCase := range map[string]func(t *testing.T, name string){
				"FailsWithoutCSR": func(t *testing.T, name string) {
					assert.Error(t, opts.PutCertRequestFromMemory(d))
				},
				"SucceedsAfterCertRequestInMemory": func(t *testing.T, name string) {
					_, _, err := opts.CertRequestInMemory()
					require.NoError(t, err)
					crt, err := opts.SignInMemory(d)
					require.NoError(t, err)

					rawCrt, err := crt.GetRawCertificate()
					require.NoError(t, err)

					checkMatchingCert(t, opts, rawCrt)
				},
				"SucceedsAfterCertRequest": func(t *testing.T, name string) {
					require.NoError(t, opts.CertRequest(d))
					crt, err := opts.SignInMemory(d)
					require.NoError(t, err)

					rawCrt, err := crt.GetRawCertificate()
					require.NoError(t, err)
					checkMatchingCert(t, opts, rawCrt)
				},
				"ReturnsIdenticalAsSubsequentSign": func(t *testing.T, name string) {
					_, _, err := opts.CertRequestInMemory()
					require.NoError(t, err)

					crt, err := opts.SignInMemory(d)
					require.NoError(t, err)

					pemCrt, err := crt.Export()
					require.NoError(t, err)

					require.NoError(t, opts.Sign(d))

					dbCrt, err := d.Get(CrtTag(name))
					require.NoError(t, err)

					assert.Equal(t, dbCrt, pemCrt)
				},
				"ReturnsIdenticalOnSubsequentCalls": func(t *testing.T, name string) {
					_, _, err := opts.CertRequestInMemory()
					require.NoError(t, err)

					crt, err := opts.SignInMemory(d)
					require.NoError(t, err)

					pemCrt, err := crt.Export()
					require.NoError(t, err)

					sameCrt, err := opts.SignInMemory(d)
					require.NoError(t, err)

					pemSameCrt, err := sameCrt.Export()
					require.NoError(t, err)

					assert.Equal(t, pemCrt, pemSameCrt)
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					name, err := opts.getFormattedCertificateRequestName()
					require.NoError(t, err)
					opts.Reset()
					require.NoError(t, clearByName(d, name))
					defer func() {
						opts.Reset()
						assert.NoError(t, clearByName(d, name))
					}()

					subTestCase(t, name)
				})
			}
		},
		"PutCertFromMemory": func(t *testing.T, d Depot, opts *CertificateOptions) {
			for subTestName, subTestCase := range map[string]func(t *testing.T, name string){
				"FailsWithoutSignInMemory": func(t *testing.T, name string) {
					assert.Error(t, opts.PutCertFromMemory(d))
				},
				"SucceedsWithSignInMemory": func(t *testing.T, name string) {
					_, _, err := opts.CertRequestInMemory()
					require.NoError(t, err)
					cert, err := opts.SignInMemory(d)
					require.NoError(t, err)
					require.NoError(t, opts.PutCertFromMemory(d))

					pemCert, err := cert.Export()
					require.NoError(t, err)
					depotCert, err := d.Get(CrtTag(name))
					require.NoError(t, err)
					assert.Equal(t, pemCert, depotCert)
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					name, err := opts.getFormattedCertificateRequestName()
					require.NoError(t, err)
					opts.Reset()
					require.NoError(t, clearByName(d, name))

					defer func() {
						opts.Reset()
						assert.NoError(t, clearByName(d, name))
					}()

					subTestCase(t, name)
				})
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			caOpts := &CertificateOptions{
				Organization:       "mongodb",
				Country:            "USA",
				Locality:           "NYC",
				OrganizationalUnit: "evergreen",
				Province:           "Manhattan",
				Expires:            24 * time.Hour,
				IP:                 []string{"0.0.0.0", "1.1.1.1"},
				Host:               "evergreen",
				CA:                 "ca",
				CommonName:         "ca",
				CAPassphrase:       "passphrase",
			}

			tempDir, err := ioutil.TempDir(".", "cert-test")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tempDir))
			}()
			d, err := NewFileDepot(tempDir)
			require.NoError(t, err)

			require.NoError(t, caOpts.Init(d))

			opts := &CertificateOptions{
				CA:                 "ca",
				CommonName:         "test",
				Host:               "test",
				Domain:             []string{"test"},
				Expires:            24 * time.Hour,
				Organization:       "10gen",
				Country:            "Canada",
				Locality:           "Toronto",
				Province:           "Ontario",
				OrganizationalUnit: "perf",
				IP:                 []string{"0.0.0.0", "1.1.1.1"},
			}

			testCase(t, d, opts)
		})

	}
}

func TestSign(t *testing.T) {
	tempDir, err := ioutil.TempDir(".", "cert-test")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tempDir))
	}()
	d, err := NewFileDepot(tempDir)
	require.NoError(t, err)

	caOpts := &CertificateOptions{
		CommonName:         "ca",
		Organization:       "cedar",
		Country:            "EC",
		Locality:           "Quito",
		OrganizationalUnit: "dag",
		Province:           "Pichincha",
		Expires:            48 * time.Hour,
	}
	csrOpts := &CertificateOptions{
		CommonName:         "exists",
		Organization:       "mongodb",
		Country:            "USA",
		Locality:           "Manhattan",
		OrganizationalUnit: "evergreen",
		Province:           "NYC",
		Expires:            24 * time.Hour,
		IP:                 []string{"0.0.0.0", "1.1.1.1"},
		//URI:                []string{"https://www.evergreen.mongodb.com", "https://www.cedar.mongodb.com"},
	}
	crtOpts := &CertificateOptions{
		CA:      "ca",
		Host:    "exists",
		Expires: 24 * time.Hour,

		// irrelevant information should be ignored
		Organization:       "10gen",
		Country:            "CA",
		Locality:           "Toronto",
		OrganizationalUnit: "perf",
		Province:           "Ontario",
		IP:                 []string{"0.0.0.0", "1.1.1.1"},
		//URI:                []string{"https://www.evergreen.mongodb.com", "https://www.cedar.mongodb.com"},
	}
	require.NoError(t, caOpts.Init(d))
	require.NoError(t, csrOpts.CertRequest(d))
	require.NoError(t, crtOpts.Sign(d))

	csrOpts.CommonName = ""
	crtOpts.CA = ""
	crtOpts.Host = ""

	for _, test := range []struct {
		name       string
		changeOpts func()
		hasErr     bool
	}{
		{
			name: "NoHostName",
			changeOpts: func() {
				crtOpts.CA = "ca"
			},
			hasErr: true,
		},
		{
			name: "NoCAName",
			changeOpts: func() {
				crtOpts.CA = ""
				crtOpts.Host = "test"
			},
			hasErr: true,
		},
		{
			name: "CADoesNotExist",
			changeOpts: func() {
				csrOpts.CommonName = "test"
				require.NoError(t, csrOpts.CertRequest(d))
				crtOpts.CA = "ca2"
				crtOpts.Host = "test"
			},
			hasErr: true,
		},
		{
			name: "CSRDoesNotExist",
			changeOpts: func() {
				crtOpts.CA = "ca"
				crtOpts.Host = "test2"
			},
			hasErr: true,
		},
		{
			name: "NewCertificate",
			changeOpts: func() {
				csrOpts.CommonName = "test2"
				require.NoError(t, csrOpts.CertRequest(d))
				crtOpts.CA = "ca"
				crtOpts.Host = "test2"
			},
		},
		{
			name: "NewCertificateWithCAPassphrase",
			changeOpts: func() {
				caOpts.CommonName = "ca2"
				caOpts.Passphrase = "passphrase"
				require.NoError(t, caOpts.Init(d))
				csrOpts.CommonName = "test3"
				require.NoError(t, csrOpts.CertRequest(d))
				crtOpts.CA = "ca2"
				crtOpts.Host = "test3"
				crtOpts.CAPassphrase = "passphrase"
			},
		},
		{
			name: "NewIntermediateCertificate",
			changeOpts: func() {
				caOpts.CommonName = "ca"
				csrOpts.CommonName = "test4"
				require.NoError(t, csrOpts.CertRequest(d))
				crtOpts.CA = "ca"
				crtOpts.Host = "test4"
				crtOpts.CAPassphrase = ""
				crtOpts.Intermediate = true
			},
		},
		{
			name: "AlreadyExistingCertificate",
			changeOpts: func() {
				crtOpts.CA = "ca"
				crtOpts.Host = "exists"
			},
			hasErr: true,
		},
		{
			name: "CANameNotACA",
			changeOpts: func() {
				csrOpts.CommonName = "test5"
				require.NoError(t, csrOpts.CertRequest(d))
				crtOpts.CA = "exists"
				crtOpts.Host = "test5"
			},
			hasErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			crtOpts.Reset()
			csrOpts.Reset()
			defer func() {
				csrOpts.Reset()
				crtOpts.Reset()
			}()
			test.changeOpts()
			err = crtOpts.Sign(d)

			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				rawCert, err := getRawCertificate(d, crtOpts.Host)
				require.NoError(t, err)
				assert.Equal(t, crtOpts.Intermediate, rawCert.IsCA)
				assert.Equal(t, caOpts.CommonName, rawCert.Issuer.CommonName)
				assert.Equal(t, []string{caOpts.Organization}, rawCert.Issuer.Organization)
				assert.Equal(t, []string{caOpts.Country}, rawCert.Issuer.Country)
				assert.Equal(t, []string{caOpts.Locality}, rawCert.Issuer.Locality)
				assert.Equal(t, []string{caOpts.OrganizationalUnit}, rawCert.Issuer.OrganizationalUnit)
				assert.Equal(t, []string{caOpts.Province}, rawCert.Issuer.Province)
				assert.Equal(t, csrOpts.CommonName, rawCert.Subject.CommonName)
				assert.Equal(t, []string{csrOpts.Organization}, rawCert.Subject.Organization)
				assert.Equal(t, []string{csrOpts.Country}, rawCert.Subject.Country)
				assert.Equal(t, []string{csrOpts.Locality}, rawCert.Subject.Locality)
				assert.Equal(t, []string{csrOpts.OrganizationalUnit}, rawCert.Subject.OrganizationalUnit)
				assert.Equal(t, []string{csrOpts.Province}, rawCert.Subject.Province)
				assert.Equal(t, convertIPs(csrOpts.IP), rawCert.IPAddresses)
				//assert.Equal(t, convertURIs(csrOpts.URI), rawCert.URIs)
				assert.Equal(t, csrOpts.Domain, rawCert.DNSNames)
				assert.True(t, rawCert.NotBefore.Before(time.Now()))
				assert.True(t, rawCert.NotAfter.After(time.Now().Add(23*time.Hour)))
				assert.True(t, rawCert.NotAfter.Before(time.Now().Add(25*time.Hour)))
			}
		})
	}
}

func TestCreateCertificateOnExpiration(t *testing.T) {
	ctx := context.TODO()
	tempDir, err := ioutil.TempDir(".", "cert-test")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tempDir))
	}()

	caName := "ca"
	serviceName := "service"
	user := "user"
	d, err := BootstrapDepot(ctx, BootstrapDepotConfig{
		FileDepot:   tempDir,
		CAName:      caName,
		ServiceName: serviceName,
		CAOpts: &CertificateOptions{
			CommonName: caName,
			Expires:    365 * 24 * time.Hour,
		},
		ServiceOpts: &CertificateOptions{
			CA:         caName,
			CommonName: serviceName,
			Host:       serviceName,
			Expires:    24 * time.Hour,
		},
	})
	require.NoError(t, err)

	// user cert DNE exist
	opts := &CertificateOptions{
		CA:         caName,
		CommonName: user,
		Host:       user,
		Expires:    24 * time.Hour,
	}
	created, err := opts.CreateCertificateOnExpiration(d, time.Hour)
	assert.NoError(t, err)
	assert.True(t, created)
	rawUserCrt, err := getRawCertificate(d, user)
	require.NoError(t, err)
	assert.Equal(t, user, rawUserCrt.Subject.CommonName)
	assert.Equal(t, caName, rawUserCrt.Issuer.CommonName)
	assert.True(t, rawUserCrt.NotBefore.Before(time.Now()))
	assert.True(t, rawUserCrt.NotAfter.After(time.Now().Add(23*time.Hour)))
	assert.False(t, rawUserCrt.IsCA)

	// user cert exists and not expiring
	created, err = opts.CreateCertificateOnExpiration(d, time.Hour)
	assert.NoError(t, err)
	assert.False(t, created)
	rawUserCrt, err = getRawCertificate(d, user)
	require.NoError(t, err)
	assert.True(t, rawUserCrt.NotAfter.After(time.Now().Add(23*time.Hour)))

	// user cert exists and expiring
	opts.Reset()
	opts.Expires = time.Hour
	created, err = opts.CreateCertificateOnExpiration(d, 25*time.Hour)
	assert.NoError(t, err)
	assert.True(t, created)
	rawUserCrt, err = getRawCertificate(d, user)
	require.NoError(t, err)
	assert.Equal(t, user, rawUserCrt.Subject.CommonName)
	assert.Equal(t, caName, rawUserCrt.Issuer.CommonName)
	assert.True(t, rawUserCrt.NotBefore.Before(time.Now()))
	assert.True(t, rawUserCrt.NotAfter.Before(time.Now().Add(time.Hour)))
	assert.False(t, rawUserCrt.IsCA)
}

func convertIPs(ips []string) []net.IP {
	converted := make([]net.IP, len(ips))
	for i, ip := range ips {
		converted[i] = net.ParseIP(ip).To4()
	}

	return converted
}

/*
func convertURIs(uris []string) []*url.URL {
	converted := make([]*url.URL, len(uris))
	for i, uri := range uris {
		converted[i], _ = url.Parse(uri)
	}

	return converted
}
*/

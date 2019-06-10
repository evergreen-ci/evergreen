package certdepot

import (
	"crypto/x509"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
	"github.com/square/certstrap/pkix"
)

// CertificateOptions contains options to use for Init, CertRequest, and Sign.
type CertificateOptions struct {
	//
	// Options specific to Init and CertRequest.
	//
	// Passprhase to encrypt private-key PEM block.
	Passphrase string `bson:"passphrase,omitempty" json:"passphrase,omitempty" yaml:"passphrase,omitempty"`
	// Size (in bits) of RSA keypair to generate (defaults to 2048).
	KeyBits int `bson:"key_bits,omitempty" json:"key_bits,omitempty" yaml:"key_bits,omitempty"`
	// Sets the Organization (O) field of the certificate.
	Organization string `bson:"o,omitempty" json:"o,omitempty" yaml:"o,omitempty"`
	// Sets the Country (C) field of the certificate.
	Country string `bson:"c,omitempty" json:"c,omitempty" yaml:"c,omitempty"`
	// Sets the Locality (L) field of the certificate.
	Locality string `bson:"l,omitempty" json:"l,omitempty" yaml:"l,omitempty"`
	// Sets the Common Name (CN) field of the certificate.
	CommonName string `bson:"cn,omitempty" json:"cn,omitempty" yaml:"cn,omitempty"`
	// Sets the Organizational Unit (OU) field of the certificate.
	OrganizationalUnit string `bson:"ou,omitempty" json:"ou,omitempty" yaml:"ou,omitempty"`
	// Sets the State/Province (ST) field of the certificate.
	Province string `bson:"st,omitempty" json:"st,omitempty" yaml:"st,omitempty"`
	// IP addresses to add as subject alt name.
	IP []string `bson:"ip,omitempty" json:"ip,omitempty" yaml:"ip,omitempty"`
	// DNS entries to add as subject alt name.
	Domain []string `bson:"dns,omitempty" json:"dns,omitempty" yaml:"dns,omitempty"`
	// URI values to add as subject alt name.
	URI []string `bson:"uri,omitempty" json:"uri,omitempty" yaml:"uri,omitempty"`
	// Path to private key PEM file (if blank, will generate new keypair).
	Key string `bson:"key,omitempty" json:"key,omitempty" yaml:"key,omitempty"`

	//
	// Options specific to Init and Sign.
	//
	// How long until the certificate expires.
	Expires time.Duration `bson:"expires,omitempty" json:"expires,omitempty" yaml:"expires,omitempty"`

	//
	// Options specific to Sign.
	//
	// Host name of the certificate to be signed.
	Host string `bson:"host,omitempty" json:"host,omitempty" yaml:"host,omitempty"`
	// Name of CA to issue cert with.
	CA string `bson:"ca,omitempty" json:"ca,omitempty" yaml:"ca,omitempty"`
	// Passphrase to decrypt CA's private-key PEM block.
	CAPassphrase string `bson:"ca_passphrase,omitempty" json:"ca_passphrase,omitempty" yaml:"ca_passphrase,omitempty"`
	// Whether generated certificate should be an intermediate.
	Intermediate bool `bson:"intermediate,omitempty" json:"intermediate,omitempty" yaml:"intermediate,omitempty"`

	csr *pkix.CertificateSigningRequest
	key *pkix.Key
	crt *pkix.Certificate
}

// Init initializes a new CA.
func (opts *CertificateOptions) Init(d depot.Depot) error {
	if opts.CommonName == "" {
		return errors.New("must provide common name of CA")
	}
	formattedName := strings.Replace(opts.CommonName, " ", "_", -1)

	if depot.CheckCertificate(d, formattedName) || depot.CheckPrivateKey(d, formattedName) {
		return errors.New("CA with specified name already exists")
	}

	key, err := opts.getOrCreatePrivateKey()
	if err != nil {
		return errors.WithStack(err)
	}

	expiresTime := time.Now().Add(opts.Expires)
	crt, err := pkix.CreateCertificateAuthority(
		key,
		opts.OrganizationalUnit,
		expiresTime,
		opts.Organization,
		opts.Country,
		opts.Province,
		opts.Locality,
		opts.CommonName,
	)
	if err != nil {
		return errors.Wrap(err, "problem creating certificate authority")
	}

	if err = depot.PutCertificate(d, formattedName, crt); err != nil {
		return errors.Wrap(err, "problem saving certificate authority")
	}

	if opts.Passphrase != "" {
		if err = depot.PutEncryptedPrivateKey(d, formattedName, key, []byte(opts.Passphrase)); err != nil {
			return errors.Wrap(err, "problem saving encrypted private key")
		}
	} else {
		if err = depot.PutPrivateKey(d, formattedName, key); err != nil {
			return errors.Wrap(err, "problem saving private key")
		}
	}

	// create an empty CRL, this is useful for Java apps which mandate a CRL
	crl, err := pkix.CreateCertificateRevocationList(key, crt, expiresTime)
	if err != nil {
		return errors.Wrap(err, "problem creating certificate revocation list")
	}
	if err = depot.PutCertificateRevocationList(d, formattedName, crl); err != nil {
		return errors.Wrap(err, "problem saving certificate revocation list")
	}

	if md, ok := d.(*mongoDepot); ok {
		if err = md.PutTTL(formattedName, expiresTime); err != nil {
			return errors.Wrap(err, "problem setting certificate TTL")
		}
	}
	return nil
}

// Reset clears the cached results of CertificateOptions so that the options can
// be changed after a certificate has already been requested or signed. For
// example, if the options have been modified, a new certificate request can be
// made with the new options by using Reset.
func (opts *CertificateOptions) Reset() {
	opts.csr = nil
	opts.key = nil
	opts.crt = nil
}

// CertRequest creates a new certificate signing request (CSR) and key and puts
// them in the depot.
func (opts *CertificateOptions) CertRequest(d depot.Depot) error {
	if _, _, err := opts.CertRequestInMemory(d); err != nil {
		return errors.Wrap(err, "problem creating cert request and key")
	}

	return opts.PutCertRequestFromMemory(d)
}

func (opts *CertificateOptions) certRequestedInMemory() bool {
	return opts.csr != nil && opts.key != nil
}

// CertRequestInMemory is the same as CertRequest but returns the resulting
// certificate signing request and private key without putting them in the
// depot. Use PutCertRequestFromMemory to put the certificate in the depot.
func (opts *CertificateOptions) CertRequestInMemory(d depot.Depot) (*pkix.CertificateSigningRequest, *pkix.Key, error) {
	if opts.certRequestedInMemory() {
		return opts.csr, opts.key, nil
	}

	ips, err := pkix.ParseAndValidateIPs(strings.Join(opts.IP, ","))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "problem parsing and validating IPs: %s", opts.IP)
	}

	uris, err := pkix.ParseAndValidateURIs(strings.Join(opts.URI, ","))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "problem parsing and validating URIs: %s", opts.URI)
	}

	name, err := opts.getCertificateRequestName()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	key, err := opts.getOrCreatePrivateKey()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	csr, err := pkix.CreateCertificateSigningRequest(
		key,
		opts.OrganizationalUnit,
		ips,
		opts.Domain,
		uris,
		opts.Organization,
		opts.Country,
		opts.Province,
		opts.Locality,
		name,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "problem creating certificate request")
	}

	opts.csr = csr
	opts.key = key

	return csr, key, nil
}

// PutCertRequestFromMemory stores the certificate request and key generated
// from the options in the depot.
func (opts *CertificateOptions) PutCertRequestFromMemory(d depot.Depot) error {
	if !opts.certRequestedInMemory() {
		return errors.New("must make cert request first before putting into depot")
	}

	formattedName, err := opts.getFormattedCertificateRequestName()
	if err != nil {
		return errors.Wrap(err, "problem getting formatted name")
	}

	if depot.CheckCertificateSigningRequest(d, formattedName) || depot.CheckPrivateKey(d, formattedName) {
		return errors.New("certificate request has existed")
	}

	if err = depot.PutCertificateSigningRequest(d, formattedName, opts.csr); err != nil {
		return errors.Wrap(err, "problem saving certificate request")
	}

	if opts.Passphrase != "" {
		if err = depot.PutEncryptedPrivateKey(d, formattedName, opts.key, []byte(opts.Passphrase)); err != nil {
			return errors.Wrap(err, "problem saving encrypted private key")
		}
	} else {
		if err = depot.PutPrivateKey(d, formattedName, opts.key); err != nil {
			return errors.Wrap(err, "problem saving private key error")
		}
	}

	return nil
}

// Sign signs a CSR with a given CA for a new certificate.
func (opts *CertificateOptions) Sign(d depot.Depot) error {
	_, err := opts.SignInMemory(d)
	if err != nil {
		return errors.Wrap(err, "problem signing certificate request")
	}

	return opts.PutCertFromMemory(d)
}

func (opts *CertificateOptions) signedInMemory() bool {
	return opts.crt != nil
}

// SignInMemory is the same as Sign but returns the resulting certificate
// without putting it in the depot. Use PutCertFromMemory to put the certificate
// in the depot.
func (opts *CertificateOptions) SignInMemory(d depot.Depot) (*pkix.Certificate, error) {
	if opts.signedInMemory() {
		return opts.crt, nil
	}
	if opts.Host == "" {
		return nil, errors.New("must provide name of host")
	}
	if opts.CA == "" {
		return nil, errors.New("must provide name of CA")
	}
	formattedReqName := strings.Replace(opts.Host, " ", "_", -1)
	formattedCAName := strings.Replace(opts.CA, " ", "_", -1)

	var csr *pkix.CertificateSigningRequest
	if opts.certRequestedInMemory() {
		csr = opts.csr
	} else {
		var err error
		csr, err = depot.GetCertificateSigningRequest(d, formattedReqName)
		if err != nil {
			return nil, errors.Wrap(err, "problem getting host's certificate signing request")
		}
	}
	crt, err := depot.GetCertificate(d, formattedCAName)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting CA certificate")
	}

	// validate that crt is allowed to sign certificates
	rawCrt, err := crt.GetRawCertificate()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting raw CA certificate")
	}
	// we punt on checking BasicConstraintsValid and checking MaxPathLen. The goal
	// is to prevent accidentally creating invalid certificates, not protecting
	// against malicious input.
	if !rawCrt.IsCA {
		return nil, errors.Errorf("%s is not allowed to sign certificates", opts.CA)
	}

	var key *pkix.Key
	if opts.CAPassphrase == "" {
		key, err = depot.GetPrivateKey(d, formattedCAName)
		if err != nil {
			return nil, errors.Wrap(err, "problem getting unencrypted (assumed) CA key")
		}
	} else {
		key, err = depot.GetEncryptedPrivateKey(d, formattedCAName, []byte(opts.CAPassphrase))
		if err != nil {
			return nil, errors.Wrap(err, "problem getting encrypted CA key")
		}
	}

	expiresTime := time.Now().Add(opts.Expires)
	var crtOut *pkix.Certificate
	if opts.Intermediate {
		crtOut, err = pkix.CreateIntermediateCertificateAuthority(crt, key, csr, expiresTime)
	} else {
		crtOut, err = pkix.CreateCertificateHost(crt, key, csr, expiresTime)
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem creating certificate")
	}

	opts.crt = crtOut

	return crtOut, nil
}

// PutCertFromMemory stores the certificate generated from the options in the
// depot, along with the expiration TTL on the certificate.
func (opts *CertificateOptions) PutCertFromMemory(d depot.Depot) error {
	if !opts.signedInMemory() {
		return errors.New("must sign cert first before putting into depot")
	}
	formattedReqName := strings.Replace(opts.Host, " ", "_", -1)

	if depot.CheckCertificate(d, formattedReqName) {
		return errors.New("certificate has existed")
	}

	if err := depot.PutCertificate(d, formattedReqName, opts.crt); err != nil {
		return errors.Wrap(err, "problem saving certificate")
	}

	if md, ok := d.(*mongoDepot); ok {
		rawCrt, err := opts.crt.GetRawCertificate()
		if err != nil {
			return errors.Wrap(err, "problem getting raw certificate")
		}
		if err = md.PutTTL(formattedReqName, rawCrt.NotAfter); err != nil {
			return errors.Wrap(err, "problem saving certificate TTL")
		}
	}

	return nil
}

func (opts CertificateOptions) getFormattedCertificateRequestName() (string, error) {
	name, err := opts.getCertificateRequestName()
	if err != nil {
		return "", errors.Wrap(err, "could not get name for certificate request")
	}
	filenameAcceptable, err := regexp.Compile("[^a-zA-Z0-9._-]")
	if err != nil {
		return "", errors.Wrap(err, "error compiling regex")
	}
	return string(filenameAcceptable.ReplaceAll([]byte(name), []byte("_"))), nil
}

func (opts CertificateOptions) getCertificateRequestName() (string, error) {
	switch {
	case opts.CommonName != "":
		return opts.CommonName, nil
	case len(opts.Domain) != 0:
		return opts.Domain[0], nil
	default:
		return "", errors.New("must provide a common name or domain")
	}
}

func (opts CertificateOptions) getOrCreatePrivateKey() (*pkix.Key, error) {
	var key *pkix.Key
	if opts.Key != "" {
		keyBytes, err := ioutil.ReadFile(opts.Key)
		if err != nil {
			return nil, errors.Wrapf(err, "problem reading key from %s", opts.Key)
		}
		key, err = pkix.NewKeyFromPrivateKeyPEM(keyBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "problem getting key from PEM")
		}
	} else {
		if opts.KeyBits == 0 {
			opts.KeyBits = 2048
		}
		var err error
		key, err = pkix.CreateRSAKey(opts.KeyBits)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating RSA key")
		}
	}
	return key, nil
}

func getNameAndKey(tag *depot.Tag) (string, string) {
	if name := depot.GetNameFromCrtTag(tag); name != "" {
		return name, userCertKey
	}
	if name := depot.GetNameFromPrivKeyTag(tag); name != "" {
		return name, userPrivateKeyKey
	}
	if name := depot.GetNameFromCsrTag(tag); name != "" {
		return name, userCertReqKey
	}
	if name := depot.GetNameFromCrlTag(tag); name != "" {
		return name, userCertRevocListKey
	}
	return "", ""
}

// CreateCertificate is a convenience function for creating a certificate
// request and signing it.
func (opts *CertificateOptions) CreateCertificate(d depot.Depot) error {
	if err := opts.CertRequest(d); err != nil {
		return errors.Wrap(err, "problem creating the certificate request")
	}
	if err := opts.Sign(d); err != nil {
		return errors.Wrap(err, "problem signing the certificate request")
	}

	return nil
}

// CreateCertificateOnExpiration checks if a certificate does not exist or if
// it expires within the duration `after` and creates a new certificate if
// either condition is met. True is returned if a certificate is created,
// false otherwise. If the certificate is a CA, the behavior is undefined.
func (opts *CertificateOptions) CreateCertificateOnExpiration(d depot.Depot, after time.Duration) (bool, error) {
	dne := true
	var created bool
	var err error

	if depot.CheckCertificate(d, opts.CommonName) {
		dne, err = DeleteOnExpiration(d, opts.CommonName, after)
		if err != nil {
			return created, errors.Wrap(err, "problem deleting expiring certificate")
		}
	}

	if dne {
		err = opts.CreateCertificate(d)
		created = true
	}

	return created, errors.Wrap(err, "problem creating certificate")
}

// ValidityBounds returns the date range for which the certificate is valid.
func ValidityBounds(d depot.Depot, name string) (time.Time, time.Time, error) {
	rawCert, err := getRawCertificate(d, name)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "problem getting raw certificate")
	}

	return rawCert.NotBefore, rawCert.NotAfter, nil
}

// DeleteOnExpiration deletes the given certificate from the depot if it has an
// expiration date within the duration `after`. True is returned if the
// certificate is deleted, false otherwise.
func DeleteOnExpiration(d depot.Depot, name string, after time.Duration) (bool, error) {
	var deleted bool

	if !depot.CheckCertificate(d, name) {
		return deleted, nil
	}

	rawCert, err := getRawCertificate(d, name)
	if err != nil {
		return deleted, errors.Wrap(err, "problem getting raw certificate")
	}

	if rawCert.NotAfter.Before(time.Now().Add(after)) {
		err = depot.DeleteCertificate(d, name)
		if err != nil {
			return deleted, errors.Wrap(err, "problem deleting expiring certificate")
		}

		if !rawCert.IsCA {
			err = depot.DeleteCertificateSigningRequest(d, name)
			if err != nil {
				return deleted, errors.Wrap(err, "problem deleting expiring certificate signing request")
			}
		}

		err = d.Delete(depot.PrivKeyTag(name))
		if err != nil {
			return deleted, errors.Wrap(err, "problem deleting expiring certificate key")
		}

		deleted = true
	}

	return deleted, nil
}

func getRawCertificate(d depot.Depot, name string) (*x509.Certificate, error) {
	cert, err := depot.GetCertificate(d, name)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting certificate from the depot")
	}

	rawCert, err := cert.GetRawCertificate()
	return rawCert, errors.WithStack(err)
}

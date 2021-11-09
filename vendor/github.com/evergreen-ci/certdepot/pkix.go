package certdepot

import (
	"time"

	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
	"github.com/square/certstrap/pkix"
)

// CrtTag returns a tag corresponding to a certificate.
func CrtTag(prefix string) *depot.Tag {
	return depot.CrtTag(prefix)
}

// PrivKeyTag returns a tag corresponding to a private key.
func PrivKeyTag(prefix string) *depot.Tag {
	return depot.PrivKeyTag(prefix)
}

// CsrTag returns a tag corresponding to a certificate signature request.
func CsrTag(prefix string) *depot.Tag {
	return depot.CsrTag(prefix)
}

// CrlTag returns a tag corresponding to a certificate revocation list.
func CrlTag(prefix string) *depot.Tag {
	return depot.CrlTag(prefix)
}

// GetNameFromCrtTag returns the name from a certificate tag.
func GetNameFromCrtTag(tag *depot.Tag) string {
	return depot.GetNameFromCrtTag(tag)
}

// GetNameFromPrivKeyTag returns the name from a private key tag.
func GetNameFromPrivKeyTag(tag *depot.Tag) string {
	return depot.GetNameFromPrivKeyTag(tag)
}

// GetNameFromCsrTag returns the name from a certificate request tag.
func GetNameFromCsrTag(tag *depot.Tag) string {
	return depot.GetNameFromCsrTag(tag)
}

// GetNameFromCrlTag returns the name from a certificate revocation list tag.
func GetNameFromCrlTag(tag *depot.Tag) string {
	return depot.GetNameFromCrlTag(tag)
}

// PutCertificate creates a certificate for a given name in the depot.
func PutCertificate(d Depot, name string, crt *pkix.Certificate) error {
	return depot.PutCertificate(d, name, crt)
}

// CheckCertificate checks the depot for existence of a certificate for a given
// name.
func CheckCertificate(d Depot, name string) bool {
	return depot.CheckCertificate(d, name)
}

// CheckCertificateWithError checks the depot for existence of a certificate
// for a given name. In the event of an internal error, an error is returned.
func CheckCertificateWithError(d Depot, name string) (bool, error) {
	exists, err := d.CheckWithError(CrtTag(name))
	return exists, errors.Wrap(err, "checking certificate")
}

// GetCertificate retrieves a certificate for a given name from the depot.
func GetCertificate(d Depot, name string) (crt *pkix.Certificate, err error) {
	return depot.GetCertificate(d, name)
}

// DeleteCertificate removes a certificate for a given name from the depot.
func DeleteCertificate(d Depot, name string) error {
	return depot.DeleteCertificate(d, name)
}

// PutCertificateSigningRequest creates a certificate signing request for a
// given name and csr in the depot.
func PutCertificateSigningRequest(d Depot, name string, csr *pkix.CertificateSigningRequest) error {
	return depot.PutCertificateSigningRequest(d, name, csr)
}

// CheckCertificateSigningRequest checks the depot for existence of a
// certificate signing request for a given name.
func CheckCertificateSigningRequest(d Depot, name string) bool {
	return depot.CheckCertificateSigningRequest(d, name)
}

// CheckCertificateSigningRequestWithError checks the depot for existence of a
// certificate signing request for a given name. In the event of an internal
// error, an error is returned.
func CheckCertificateSigningRequestWithError(d Depot, name string) (bool, error) {
	exists, err := d.CheckWithError(CsrTag(name))
	return exists, errors.Wrap(err, "checking certificate signing request")
}

// GetCertificateSigningRequest retrieves a certificate signing request for a
// given name from the depot.
func GetCertificateSigningRequest(d Depot, name string) (crt *pkix.CertificateSigningRequest, err error) {
	return depot.GetCertificateSigningRequest(d, name)
}

// DeleteCertificateSigningRequest removes a certificate signing request for a
// given name from the depot.
func DeleteCertificateSigningRequest(d Depot, name string) error {
	return depot.DeleteCertificateSigningRequest(d, name)
}

// PutPrivateKey creates a private key for a given name in the depot.
func PutPrivateKey(d Depot, name string, key *pkix.Key) error {
	return depot.PutPrivateKey(d, name, key)
}

// CheckPrivateKey checks the depot for existence of a private key for a given
// given name.
func CheckPrivateKey(d Depot, name string) bool {
	return depot.CheckPrivateKey(d, name)
}

// CheckPrivateKeyWithError checks the depot for existence of a private key for
// a given name. In the event of an internal error, an error is returned.
func CheckPrivateKeyWithError(d Depot, name string) (bool, error) {
	exists, err := d.CheckWithError(PrivKeyTag(name))
	return exists, errors.Wrap(err, "checking private key")
}

// GetPrivateKey retrieves a private key for a given name from the depot.
func GetPrivateKey(d Depot, name string) (key *pkix.Key, err error) {
	return depot.GetPrivateKey(d, name)
}

// DeletePrivateKey removes a private key for a given name from the depot. This
// works for both encrypted and unencrypted private keys.
func DeletePrivateKey(d Depot, name string) error {
	return d.Delete(depot.PrivKeyTag(name))
}

// PutEncryptedPrivateKey creates an encrypted private key for a given name in
// the depot.
func PutEncryptedPrivateKey(d Depot, name string, key *pkix.Key, passphrase []byte) error {
	return depot.PutEncryptedPrivateKey(d, name, key, passphrase)
}

// GetEncryptedPrivateKey retrieves an encrypted private key for a given name
// from the depot.
func GetEncryptedPrivateKey(d Depot, name string, passphrase []byte) (key *pkix.Key, err error) {
	return depot.GetEncryptedPrivateKey(d, name, passphrase)
}

// PutCertificateRevocationList creates a CRL for a given name in the depot.
func PutCertificateRevocationList(d Depot, name string, crl *pkix.CertificateRevocationList) error {
	return depot.PutCertificateRevocationList(d, name, crl)
}

// GetCertificateRevocationList gets a CRL for a given name in the depot.
func GetCertificateRevocationList(d Depot, name string) (*pkix.CertificateRevocationList, error) {
	return depot.GetCertificateRevocationList(d, name)
}

// DeleteCertificateRevocationList removes a CRL for a given name in the depot.
func DeleteCertificateRevocationList(d Depot, name string) error {
	return d.Delete(depot.CrlTag(name))
}

// PutTTL puts a new TTL for a given name in the mongo depot.
func putTTL(d Depot, name string, expiration time.Time) error {
	md, ok := d.(*mongoDepot)
	if !ok {
		return errors.New("cannot put TTL if depot is not a mongo depot")
	}
	return md.PutTTL(name, expiration)
}

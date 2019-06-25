package certdepot

import (
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

// CsrTag returns a tag corresponding to a certificate signature request file.
func CsrTag(prefix string) *depot.Tag {
	return depot.CsrTag(prefix)
}

// CrlTag returns a tag corresponding to a certificate revocation list.
func CrlTag(prefix string) *depot.Tag {
	return depot.CrlTag(prefix)
}

// GetNameFromCrtTag returns the host name from a certificate file tag.
func GetNameFromCrtTag(tag *depot.Tag) string {
	return depot.GetNameFromCrtTag(tag)
}

// GetNameFromPrivKeyTag returns the host name from a private key file tag.
func GetNameFromPrivKeyTag(tag *depot.Tag) string {
	return depot.GetNameFromPrivKeyTag(tag)
}

// GetNameFromCsrTag returns the host name from a certificate request file tag.
func GetNameFromCsrTag(tag *depot.Tag) string {
	return depot.GetNameFromCsrTag(tag)
}

// GetNameFromCrlTag returns the host name from a certificate revocation list
// file tag.
func GetNameFromCrlTag(tag *depot.Tag) string {
	return depot.GetNameFromCrlTag(tag)
}

// PutCertificate creates a certificate file for a given name in the depot.
func PutCertificate(d Depot, name string, crt *pkix.Certificate) error {
	return depot.PutCertificate(depot.Depot(d), name, crt)
}

// CheckCertificate checks the depot for existence of a certificate file for a
// given name.
func CheckCertificate(d Depot, name string) bool {
	return depot.CheckCertificate(depot.Depot(d), name)
}

// GetCertificate retrieves a certificate file for a given name from the depot.
func GetCertificate(d Depot, name string) (crt *pkix.Certificate, err error) {
	return depot.GetCertificate(depot.Depot(d), name)
}

// DeleteCertificate removes a certificate file for a given name from the
// depot.
func DeleteCertificate(d Depot, name string) error {
	return depot.DeleteCertificate(depot.Depot(d), name)
}

// PutCertificateSigningRequest creates a certificate signing request file for
// a given name and csr in the depot.
func PutCertificateSigningRequest(d Depot, name string, csr *pkix.CertificateSigningRequest) error {
	return depot.PutCertificateSigningRequest(depot.Depot(d), name, csr)
}

// CheckCertificateSigningRequest checks the depot for existence of a
// certificate signing request file for a given host name.
func CheckCertificateSigningRequest(d Depot, name string) bool {
	return depot.CheckCertificateSigningRequest(depot.Depot(d), name)
}

// GetCertificateSigningRequest retrieves a certificate signing request file
// for a given host name from the depot.
func GetCertificateSigningRequest(d Depot, name string) (crt *pkix.CertificateSigningRequest, err error) {
	return depot.GetCertificateSigningRequest(depot.Depot(d), name)
}

// DeleteCertificateSigningRequest removes a certificate signing request file
// for a given host name from the depot.
func DeleteCertificateSigningRequest(d Depot, name string) error {
	return depot.DeleteCertificateSigningRequest(depot.Depot(d), name)
}

// PutPrivateKey creates a private key file for a given name in the depot.
func PutPrivateKey(d Depot, name string, key *pkix.Key) error {
	return depot.PutPrivateKey(depot.Depot(d), name, key)
}

// CheckPrivateKey checks the depot for existence of a private key file for a
// given name.
func CheckPrivateKey(d Depot, name string) bool {
	return depot.CheckPrivateKey(depot.Depot(d), name)
}

// GetPrivateKey retrieves a private key file for a given name from the depot.
func GetPrivateKey(d Depot, name string) (key *pkix.Key, err error) {
	return depot.GetPrivateKey(depot.Depot(d), name)
}

// DeletePrivateKey removes a private key file for a given host name from the
// depot. This works for both encrypted and unencrypted private keys.
func DeletePrivateKey(d Depot, name string) error {
	return d.Delete(depot.PrivKeyTag(name))
}

// PutEncryptedPrivateKey creates an encrypted private key file for a given
// name in the depot.
func PutEncryptedPrivateKey(d Depot, name string, key *pkix.Key, passphrase []byte) error {
	return depot.PutEncryptedPrivateKey(depot.Depot(d), name, key, passphrase)
}

// GetEncryptedPrivateKey retrieves an encrypted private key file for a given
// name from the depot.
func GetEncryptedPrivateKey(d Depot, name string, passphrase []byte) (key *pkix.Key, err error) {
	return depot.GetEncryptedPrivateKey(depot.Depot(d), name, passphrase)
}

// PutCertificateRevocationList creates a CRL file for a given name in the
// depot.
func PutCertificateRevocationList(d Depot, name string, crl *pkix.CertificateRevocationList) error {
	return depot.PutCertificateRevocationList(depot.Depot(d), name, crl)
}

// GetCertificateRevocationList gets a CRL file for a given name in the depot.
func GetCertificateRevocationList(d Depot, name string) (*pkix.CertificateRevocationList, error) {
	return depot.GetCertificateRevocationList(depot.Depot(d), name)
}

// DeleteCertificateRevocationList removes a CRL file for a given name in the
// depot.
func DeleteCertificateRevocationList(d Depot, name string) error {
	return d.Delete(depot.CrlTag(name))
}

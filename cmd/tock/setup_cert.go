package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"time"
)

// signName is the local code-signing identity: the common name of the
// certificate tock setup makes (packaging/cs.cnf makes the same one).
const signName = "tock-local-codesign"

// newSigningCert makes the self-signed code-signing certificate: an RSA key
// as PKCS#1 PEM and the certificate as PEM, valid ten years from now. Leaf
// only, for code signing and nothing else.
func newSigningCert(now time.Time, r io.Reader) (keyPEM, certPEM []byte, err error) {
	if r == nil {
		r = rand.Reader
	}
	key, err := rsa.GenerateKey(r, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("setup: key: %w", err)
	}
	serial, err := rand.Int(r, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, fmt.Errorf("setup: serial: %w", err)
	}
	pub := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	ski := sha1.Sum(pub)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: signName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
		IsCA:                  false,
		SubjectKeyId:          ski[:],
	}
	der, err := x509.CreateCertificate(r, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("setup: certificate: %w", err)
	}
	// PKCS#1: security import -f openssl takes no PKCS#8
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

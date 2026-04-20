package certissuance

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestIssueFromCSR(t *testing.T) {
	caCertPEM, caKeyPEM := generateTestCA(t)
	svc, err := NewServiceFromPEM(caCertPEM, caKeyPEM, 24*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("NewServiceFromPEM failed: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "node-123"},
		DNSNames: []string{"node-123.internal"},
	}, leafKey)
	if err != nil {
		t.Fatalf("CreateCertificateRequest failed: %v", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	issued, err := svc.IssueFromCSR(IssueRequest{
		NodeID:   "node-123",
		TenantID: "tenant-abc",
		CSRPEM:   csrPEM,
	})
	if err != nil {
		t.Fatalf("IssueFromCSR failed: %v", err)
	}

	if issued.SerialNumber == "" {
		t.Fatalf("expected serial number")
	}
	if issued.CertPEM == "" || issued.CAPEM == "" {
		t.Fatalf("expected cert and ca pem")
	}
	if !issued.NotAfter.After(issued.NotBefore) {
		t.Fatalf("expected valid notBefore/notAfter")
	}

	block, _ := pem.Decode([]byte(issued.CertPEM))
	if block == nil {
		t.Fatalf("failed to decode issued cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate failed: %v", err)
	}
	if cert.Subject.CommonName != "node-123" {
		t.Fatalf("unexpected CN: %s", cert.Subject.CommonName)
	}
	if len(cert.ExtKeyUsage) == 0 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatalf("expected client auth EKU")
	}
}

func generateTestCA(t *testing.T) ([]byte, []byte) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "aria-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, caKey.Public(), caKey)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	caKeyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey failed: %v", err)
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: caKeyDER})
	return caCertPEM, caKeyPEM
}

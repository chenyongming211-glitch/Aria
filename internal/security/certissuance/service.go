package certissuance

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// Service signs agent client certificates from CSR requests.
type Service struct {
	caCert      *x509.Certificate
	caSigner    crypto.Signer
	caPEM       string
	validity    time.Duration
	renewBefore time.Duration
}

type IssueRequest struct {
	NodeID   string
	TenantID string
	CSRPEM   []byte
}

type IssuedCertificate struct {
	SerialNumber string
	CertPEM      string
	CAPEM        string
	NotBefore    time.Time
	NotAfter     time.Time
}

func NewServiceFromPEM(caCertPEM, caKeyPEM []byte, validity, renewBefore time.Duration) (*Service, error) {
	if validity <= 0 {
		validity = 24 * time.Hour * 30
	}
	if renewBefore <= 0 {
		renewBefore = 72 * time.Hour
	}

	certBlock, _ := pem.Decode(caCertPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("invalid CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("invalid CA private key PEM")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		// Fallback for traditional EC/RSA encodings.
		if ecKey, ecErr := x509.ParseECPrivateKey(keyBlock.Bytes); ecErr == nil {
			privateKey = ecKey
		} else if rsaKey, rsaErr := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); rsaErr == nil {
			privateKey = rsaKey
		} else {
			return nil, fmt.Errorf("parse CA private key: %w", err)
		}
	}
	signer, ok := privateKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("CA private key does not implement crypto.Signer")
	}

	return &Service{
		caCert:      caCert,
		caSigner:    signer,
		caPEM:       string(caCertPEM),
		validity:    validity,
		renewBefore: renewBefore,
	}, nil
}

func (s *Service) RenewBefore() time.Duration { return s.renewBefore }

func (s *Service) IssueFromCSR(req IssueRequest) (*IssuedCertificate, error) {
	if len(req.CSRPEM) == 0 {
		return nil, fmt.Errorf("csr is required")
	}

	csrBlock, _ := pem.Decode(req.CSRPEM)
	if csrBlock == nil {
		return nil, fmt.Errorf("invalid CSR PEM")
	}
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("invalid CSR signature: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("serial generation failed: %w", err)
	}

	notBefore := time.Now().UTC().Add(-5 * time.Minute)
	notAfter := notBefore.Add(s.validity)

	leafTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
		URIs:         csr.URIs,
	}

	der, err := x509.CreateCertificate(rand.Reader, leafTemplate, s.caCert, csr.PublicKey, s.caSigner)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	return &IssuedCertificate{
		SerialNumber: serial.Text(16),
		CertPEM: string(pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		})),
		CAPEM:     s.caPEM,
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}, nil
}

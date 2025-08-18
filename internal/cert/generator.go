package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CertificateConfig holds configuration for certificate generation
type CertificateConfig struct {
	CommonName         string
	Organization       string
	OrganizationalUnit string
	Country            string
	Province           string
	Locality           string
	ApplicationURI     string
	Email              string
	ValidityDays       int
	KeySize            int
	DNSNames           []string
	IPAddresses        []net.IP
}

// ForceGenerateCertificates always generates new certificate and key files,
// overwriting any existing files at the standard storage location.
func ForceGenerateCertificates() (certPath, keyPath string, err error) {
	storageDir, err := GetMobileStoragePath()
	if err != nil {
		return "", "", err
	}

	// Only ensure and return CA files (no leaf cert/key per user request)
	caCrt, caKey, err := EnsureLocalCA(storageDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to ensure local CA: %w", err)
	}
	return caCrt, caKey, nil
}

// DefaultConfig returns a default certificate configuration suitable for mobile devices
func DefaultConfig() *CertificateConfig {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "opcua-client"
	}

	// Generate a unique application URI
	appURI := fmt.Sprintf("urn:%s:opcuababy", hostname)

	return &CertificateConfig{
		CommonName:         "OpcuaBaby",
		Organization:       "BigGiantBaby",
		OrganizationalUnit: "Daniel",
		Country:            "US",
		Province:           "CA",
		Locality:           "San Francisco",
		ApplicationURI:     appURI,
		Email:              "466719205@qq.com",
		ValidityDays:       3650, // 10 years
		KeySize:            2048,
		// Strict OPC UA profile: only URI SAN (ApplicationURI); no DNS/IP SAN
		DNSNames:           nil,
		IPAddresses:        nil,
	}
}

// MobileConfig returns a certificate configuration optimized for mobile devices
func MobileConfig() *CertificateConfig {
	config := DefaultConfig()
	
	// Mobile-specific adjustments
	if runtime.GOOS == "ios" || runtime.GOOS == "android" {
		config.CommonName = "OpcuaBaby"
		config.DNSNames = nil
		// Use smaller key size for better performance on mobile
		config.KeySize = 2048
	}
	
	return config
}

// GenerateCertificateFiles creates both .der certificate and .pem private key files
func GenerateCertificateFiles(config *CertificateConfig, certPath, keyPath string) error {
    // Disabled per user request: do not generate leaf certificate/key files.
    // Only CA generation via EnsureLocalCA/GenerateCertificateFilesSigned is supported.
    return nil
}

// GetMobileStoragePath returns the appropriate storage path for certificates on mobile devices
func GetMobileStoragePath() (string, error) {
	var baseDir string
	
	switch runtime.GOOS {
	case "ios":
		// On iOS, use the app's Documents directory
		if homeDir, err := os.UserHomeDir(); err == nil {
			baseDir = filepath.Join(homeDir, "Documents")
		} else {
			baseDir = "/tmp"
		}
	case "android":
		// On Android, use the app's internal storage
		if homeDir, err := os.UserHomeDir(); err == nil {
			baseDir = filepath.Join(homeDir, "files")
		} else {
			baseDir = "/data/data/com.giantbaby.opcuababy/files"
		}
	default:
		// Desktop platforms - use visible directory
		if homeDir, err := os.UserHomeDir(); err == nil {
			baseDir = filepath.Join(homeDir, "Documents", "opcuababy")
		} else {
			baseDir = "/tmp/opcuababy"
		}
	}
	
	certDir := filepath.Join(baseDir, "certificates")
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create certificate directory: %w", err)
	}
	
	return certDir, nil
}

// AutoGenerateCertificates automatically generates certificate files if they don't exist
func AutoGenerateCertificates() (certPath, keyPath string, err error) {
    storageDir, err := GetMobileStoragePath()
    if err != nil {
        return "", "", err
    }

    // Only ensure and return CA files (no leaf cert/key per user request)
    caCrt, caKey, err := EnsureLocalCA(storageDir)
    if err != nil {
        return "", "", fmt.Errorf("failed to ensure local CA: %w", err)
    }
    return caCrt, caKey, nil
}

// EnsureLocalCA creates a simple local CA (ca.crt/ca.key) under dir if missing.
func EnsureLocalCA(dir string) (caCrtPath, caKeyPath string, err error) {
    caCrtPath = filepath.Join(dir, "ca.crt")
    caKeyPath = filepath.Join(dir, "ca.key")
    // If both exist, return
    if _, err1 := os.Stat(caCrtPath); err1 == nil {
        if _, err2 := os.Stat(caKeyPath); err2 == nil {
            return caCrtPath, caKeyPath, nil
        }
    }

    // Generate CA key
    caKey, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        return "", "", fmt.Errorf("failed to generate CA key: %w", err)
    }
    now := time.Now().UTC().Add(-5 * time.Minute)
    tmpl := &x509.Certificate{
        SerialNumber: new(big.Int).SetBytes(func() []byte { b := make([]byte, 16); _, _ = rand.Read(b); return b }()),
        Subject: pkix.Name{
            CommonName:   "OpcuaBaby Local CA",
            Organization: []string{"BigGiantBaby"},
            Country:      []string{"US"},
            Province:     []string{"CA"},
            Locality:     []string{"San Francisco"},
        },
        NotBefore:             now,
        NotAfter:              now.Add(3650 * 24 * time.Hour),
        KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
        BasicConstraintsValid: true,
        IsCA:                  true,
        MaxPathLen:            0,
        SignatureAlgorithm:    x509.SHA256WithRSA,
    }
    if pubDER, err := x509.MarshalPKIXPublicKey(&caKey.PublicKey); err == nil {
        sum := sha1.Sum(pubDER)
        tmpl.SubjectKeyId = sum[:]
        tmpl.AuthorityKeyId = sum[:]
    }
    caDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
    if err != nil {
        return "", "", fmt.Errorf("failed to create CA certificate: %w", err)
    }
    // Write CA cert (PEM)
    if err := os.WriteFile(caCrtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0644); err != nil {
        return "", "", fmt.Errorf("failed to write ca.crt: %w", err)
    }
    // Write CA key (PEM PKCS#1)
    if err := os.WriteFile(caKeyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)}), 0600); err != nil {
        return "", "", fmt.Errorf("failed to write ca.key: %w", err)
    }
    return caCrtPath, caKeyPath, nil
}

// GenerateCertificateFilesSigned issues a client certificate signed by local CA.
// It also writes a PEM certificate (client.crt) and full chain (client_fullchain.crt).
func GenerateCertificateFilesSigned(config *CertificateConfig, certPath, keyPath, dir string) error {
    // Ensure CA exists; do not generate or write any leaf certificate or key
    if _, _, err := EnsureLocalCA(dir); err != nil {
        return err
    }
    return nil
}

// ValidateCertificateFiles checks if the certificate and key files are valid and compatible
func ValidateCertificateFiles(certPath, keyPath string) error {
    // Read certificate (PEM or DER)
    certData, err := os.ReadFile(certPath)
    if err != nil {
        return fmt.Errorf("failed to read certificate file: %w", err)
    }
    var certDER []byte
    if blk, _ := pem.Decode(certData); blk != nil && blk.Type == "CERTIFICATE" {
        certDER = blk.Bytes
    } else {
        certDER = certData
    }
    cert, err := x509.ParseCertificate(certDER)
    if err != nil {
        return fmt.Errorf("failed to parse certificate: %w", err)
    }
	
	// Check if certificate is expired
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid (valid from %v)", cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired (expired on %v)", cert.NotAfter)
	}
	
	// Read private key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key file: %w", err)
	}
	
	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block from private key")
	}
	
	var privateKey *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse PKCS8 private key: %w", err)
		}
		var ok bool
		privateKey, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("private key is not RSA")
		}
	default:
		return fmt.Errorf("unsupported private key type: %s", block.Type)
	}
	
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	
	// Verify that the private key matches the certificate
	certPublicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate does not contain RSA public key")
	}
	
	if privateKey.PublicKey.N.Cmp(certPublicKey.N) != 0 || privateKey.PublicKey.E != certPublicKey.E {
		return fmt.Errorf("private key does not match certificate public key")
	}
	
	return nil
}

// GetCertificateInfo returns human-readable information about a certificate
func GetCertificateInfo(certPath string) (string, error) {
    certData, err := os.ReadFile(certPath)
    if err != nil {
        return "", fmt.Errorf("failed to read certificate file: %w", err)
    }
    var certDER []byte
    if blk, _ := pem.Decode(certData); blk != nil && blk.Type == "CERTIFICATE" {
        certDER = blk.Bytes
    } else {
        certDER = certData
    }
    cert, err := x509.ParseCertificate(certDER)
    if err != nil {
        return "", fmt.Errorf("failed to parse certificate: %w", err)
    }
	
	var info strings.Builder
	info.WriteString(fmt.Sprintf("Subject: %s\n", cert.Subject.String()))
	info.WriteString(fmt.Sprintf("Valid from: %s\n", cert.NotBefore.Format("2006-01-02 15:04:05")))
	info.WriteString(fmt.Sprintf("Valid until: %s\n", cert.NotAfter.Format("2006-01-02 15:04:05")))
	info.WriteString(fmt.Sprintf("Serial Number: %s\n", cert.SerialNumber.String()))
	
	if len(cert.DNSNames) > 0 {
		info.WriteString(fmt.Sprintf("DNS Names: %s\n", strings.Join(cert.DNSNames, ", ")))
	}
	
	if len(cert.URIs) > 0 {
		var uris []string
		for _, uri := range cert.URIs {
			uris = append(uris, uri.String())
		}
		info.WriteString(fmt.Sprintf("URIs: %s\n", strings.Join(uris, ", ")))
	}
	
	return info.String(), nil
}

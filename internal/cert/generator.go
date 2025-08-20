package cert

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
	"net"
	"net/url"
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

// ---- Presets and CSR support (for compatibility with different servers) ----

// NewConfigPresetStrictUA returns a config focusing on strict OPC UA client profile:
// - SAN: only URI (ApplicationURI), no DNS/IP
// - RSA 2048, SHA256, ClientAuth EKU
// - Long validity (10 years by default)
func NewConfigPresetStrictUA(appURI, hostname string) *CertificateConfig {
    if hostname == "" { h, _ := os.Hostname(); if h != "" { hostname = h } }
    if strings.TrimSpace(appURI) == "" {
        if hostname == "" { hostname = "opcua-client" }
        appURI = fmt.Sprintf("urn:%s:opcuababy", hostname)
    }
    return &CertificateConfig{
        CommonName:         "OpcuaBaby",
        Organization:       "BigGiantBaby",
        OrganizationalUnit: "UAClient",
        Country:            "US",
        Province:           "CA",
        Locality:           "San Francisco",
        ApplicationURI:     appURI,
        Email:              "",
        ValidityDays:       3650,
        KeySize:            2048,
        DNSNames:           nil,
        IPAddresses:        nil,
    }
}

// NewConfigPresetWithDNS adds DNS/IP SANs in addition to the ApplicationURI.
// Useful for servers that validate DNS/IP SANs on client certs.
func NewConfigPresetWithDNS(appURI, hostname string, dns []string, ips []net.IP) *CertificateConfig {
    cfg := NewConfigPresetStrictUA(appURI, hostname)
    cfg.DNSNames = append(cfg.DNSNames, dns...)
    cfg.IPAddresses = append(cfg.IPAddresses, ips...)
    return cfg
}

// NewConfigPresetRsa3072 is identical to strict preset but uses a stronger 3072-bit RSA key.
func NewConfigPresetRsa3072(appURI, hostname string) *CertificateConfig {
    cfg := NewConfigPresetStrictUA(appURI, hostname)
    cfg.KeySize = 3072
    return cfg
}

// GenerateCSRFiles creates a CSR (PEM) and private key (PEM) according to the provided config.
// Use this when you need the certificate to be signed by an external CA (e.g., ABB CA).
// Returns the paths to the generated csr and key files.
func GenerateCSRFiles(config *CertificateConfig, csrPath, keyPath string) (string, string, error) {
    if config == nil { config = MobileConfig() }
    if csrPath == "" { csrPath = filepath.Join(os.TempDir(), "opcuababy", "client.csr") }
    if keyPath == "" { keyPath = filepath.Join(os.TempDir(), "opcuababy", "client.key") }

    if err := os.MkdirAll(filepath.Dir(csrPath), 0755); err != nil {
        return "", "", fmt.Errorf("create csr dir: %w", err)
    }
    if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
        return "", "", fmt.Errorf("create key dir: %w", err)
    }

    // Private key
    key, err := rsa.GenerateKey(rand.Reader, config.KeySize)
    if err != nil { return "", "", fmt.Errorf("generate key: %w", err) }

    // CSR template
    req := &x509.CertificateRequest{
        Subject: pkix.Name{
            CommonName:         config.CommonName,
            Organization:       []string{config.Organization},
            OrganizationalUnit: []string{config.OrganizationalUnit},
            Country:            []string{config.Country},
            Province:           []string{config.Province},
            Locality:           []string{config.Locality},
        },
        DNSNames:    append([]string{}, config.DNSNames...),
        IPAddresses: append([]net.IP{}, config.IPAddresses...),
    }
    if uri := strings.TrimSpace(config.ApplicationURI); uri != "" {
        if u, err := url.Parse(uri); err == nil { req.URIs = []*url.URL{u} }
    }

    csrDER, err := x509.CreateCertificateRequest(rand.Reader, req, key)
    if err != nil { return "", "", fmt.Errorf("create CSR: %w", err) }

    // Write CSR and key
    if err := os.WriteFile(csrPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), 0644); err != nil {
        return "", "", fmt.Errorf("write csr: %w", err)
    }
    if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
        return "", "", fmt.Errorf("write key: %w", err)
    }
    return csrPath, keyPath, nil
}

// GenerateSelfSignedCertificateFiles creates a self-signed client certificate and key
// and writes multiple formats under the storage directory. If certPath/keyPath are
// provided, it copies DER cert and PKCS#1 key to those destinations.
func GenerateSelfSignedCertificateFiles(config *CertificateConfig, certPath, keyPath string) error {
    storageDir := filepath.Dir(certPath)
    if certPath == "" || storageDir == "." || storageDir == "" {
        var err error
        storageDir, err = GetMobileStoragePath()
        if err != nil { return err }
    }
    if config == nil { config = MobileConfig() }
    out, err := generateSelfSignedClientCert(config, storageDir)
    if err != nil { return err }
    if certPath != "" {
        if err := copyFile(out.CertDERPath, certPath); err != nil { return err }
    }
    if keyPath != "" {
        if err := copyFile(out.KeyPKCS1Path, keyPath); err != nil { return err }
    }
    return nil
}

// generateSelfSignedClientCert generates a self-signed client cert/key and writes files.
func generateSelfSignedClientCert(config *CertificateConfig, dir string) (*genOutput, error) {
    if err := os.MkdirAll(dir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create dir: %w", err)
    }
    // Key
    key, err := rsa.GenerateKey(rand.Reader, config.KeySize)
    if err != nil { return nil, fmt.Errorf("generate key: %w", err) }

    // Template
    now := time.Now().UTC().Add(-5 * time.Minute)
    // For self-signed certs, set a friendly CN for server file display while keeping URI in SAN
    cn := "OpcUaBaby"
    tmpl := &x509.Certificate{
        SerialNumber: new(big.Int).SetBytes(func() []byte { b := make([]byte, 16); _, _ = rand.Read(b); return b }()),
        Subject: pkix.Name{
            CommonName:         cn,
            Organization:       []string{config.Organization},
            OrganizationalUnit: []string{config.OrganizationalUnit},
            Country:            []string{config.Country},
            Province:           []string{config.Province},
            Locality:           []string{config.Locality},
        },
        NotBefore:             now,
        NotAfter:              now.Add(time.Duration(config.ValidityDays) * 24 * time.Hour),
        KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
        SignatureAlgorithm:    x509.SHA256WithRSA,
        IsCA:                  false,
    }
    if uri := strings.TrimSpace(config.ApplicationURI); uri != "" {
        if u, err := url.Parse(uri); err == nil { tmpl.URIs = []*url.URL{u} }
    }
    if len(config.DNSNames) > 0 { tmpl.DNSNames = append(tmpl.DNSNames, config.DNSNames...) }
    if len(config.IPAddresses) > 0 { tmpl.IPAddresses = append(tmpl.IPAddresses, config.IPAddresses...) }
    if pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey); err == nil {
        sum := sha1.Sum(pubDER)
        tmpl.SubjectKeyId = sum[:]
        tmpl.AuthorityKeyId = sum[:]
    }

    // Self-sign
    certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
    if err != nil { return nil, fmt.Errorf("create self-signed cert: %w", err) }

    out := &genOutput{
        CertDERPath:  filepath.Join(dir, "selfsigned.der"),
        CertPEMPath:  filepath.Join(dir, "selfsigned.crt"),
        KeyPKCS1Path: filepath.Join(dir, "selfsigned.key"),
        KeyPKCS8Path: filepath.Join(dir, "selfsigned.pem"),
    }
    if err := os.WriteFile(out.CertDERPath, certDER, 0644); err != nil { return nil, fmt.Errorf("write selfsigned.der: %w", err) }
    if err := os.WriteFile(out.CertPEMPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0644); err != nil { return nil, fmt.Errorf("write selfsigned.crt: %w", err) }
    if err := os.WriteFile(out.KeyPKCS1Path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil { return nil, fmt.Errorf("write selfsigned.key: %w", err) }
    if pkcs8, err := x509.MarshalPKCS8PrivateKey(key); err == nil {
        if err := os.WriteFile(out.KeyPKCS8Path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0600); err != nil { return nil, fmt.Errorf("write selfsigned.pem: %w", err) }
    } else {
        return nil, fmt.Errorf("marshal PKCS#8: %w", err)
    }
    return out, nil
}

// ForceGenerateCertificates always generates new certificate and key files,
// overwriting any existing files at the standard storage location.
func ForceGenerateCertificates() (certPath, keyPath string, err error) {
	storageDir, err := GetMobileStoragePath()
	if err != nil {
		return "", "", err
	}

	// Ensure a local CA exists (self-created) for signing
	if _, _, err := EnsureLocalCA(storageDir); err != nil {
		return "", "", fmt.Errorf("failed to ensure local CA: %w", err)
	}

	// Use mobile-optimized defaults
	cfg := MobileConfig()

	// Generate a new client keypair and a certificate signed by our local CA
	out, err := generateClientCertSignedByLocalCA(cfg, storageDir)
	if err != nil {
		return "", "", err
	}
	// Also generate a self-signed client certificate alongside (for servers that prefer trusting leaf certs)
	if _, err := generateSelfSignedClientCert(cfg, storageDir); err != nil {
		return "", "", fmt.Errorf("failed to generate self-signed client certificate: %w", err)
	}

	// Return the DER cert and PKCS#1 key path as primary selections for our UI
	return out.CertDERPath, out.KeyPKCS1Path, nil
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
    storageDir := filepath.Dir(certPath)
    if storageDir == "." || storageDir == "" {
        var err error
        storageDir, err = GetMobileStoragePath()
        if err != nil { return err }
    }
    if _, _, err := EnsureLocalCA(storageDir); err != nil {
        return fmt.Errorf("failed to ensure local CA: %w", err)
    }
    if config == nil { config = MobileConfig() }
    out, err := generateClientCertSignedByLocalCA(config, storageDir)
    if err != nil { return err }
    // copy/rename to requested paths if provided
    if certPath != "" {
        if err := copyFile(out.CertDERPath, certPath); err != nil { return err }
    }
    if keyPath != "" {
        if err := copyFile(out.KeyPKCS1Path, keyPath); err != nil { return err }
    }
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
		// On Android, avoid relying on HOME which may be '/'. Prefer cache/config dirs.
		if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
			baseDir = cacheDir
		} else if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
			baseDir = cfgDir
		} else if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" && homeDir != "/" {
			baseDir = filepath.Join(homeDir, "files")
		} else {
			// Last resort: use process temp dir which is app-private on Android
			baseDir = filepath.Join(os.TempDir(), "opcuababy")
		}

		// Never use external storage (no permissions). If path points to /sdcard or /storage, fallback to temp.
		if strings.HasPrefix(baseDir, "/sdcard") || strings.HasPrefix(baseDir, "/storage") {
			baseDir = filepath.Join(os.TempDir(), "opcuababy")
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

    // Ensure CA and generate client materials if missing
    if _, _, err := EnsureLocalCA(storageDir); err != nil {
        return "", "", fmt.Errorf("failed to ensure local CA: %w", err)
    }
    cfg := MobileConfig()
    out, err := generateClientCertSignedByLocalCA(cfg, storageDir)
    if err != nil {
        return "", "", err
    }
    return out.CertDERPath, out.KeyPKCS1Path, nil
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
    if dir == "" { var err error; dir, err = GetMobileStoragePath(); if err != nil { return err } }
    if _, _, err := EnsureLocalCA(dir); err != nil { return err }
    if config == nil { config = MobileConfig() }
    out, err := generateClientCertSignedByLocalCA(config, dir)
    if err != nil { return err }
    // If specific destinations requested, copy
    if certPath != "" {
        if err := copyFile(out.CertDERPath, certPath); err != nil { return err }
    }
    if keyPath != "" {
        if err := copyFile(out.KeyPKCS1Path, keyPath); err != nil { return err }
    }
    return nil
}

// genOutput holds generated file paths for client materials
type genOutput struct {
    CertDERPath   string // client.der
    CertPEMPath   string // client.crt (PEM)
    KeyPKCS1Path  string // client.key (PKCS#1 PEM)
    KeyPKCS8Path  string // client.pem (PKCS#8 PEM)
}

// generateClientCertSignedByLocalCA generates a client RSA key and a certificate signed by our local CA.
// It writes multiple formats: client.key (PKCS#1 PEM), client.pem (PKCS#8 PEM), client.crt (PEM), client.der (DER).
func generateClientCertSignedByLocalCA(config *CertificateConfig, dir string) (*genOutput, error) {
    if err := os.MkdirAll(dir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create dir: %w", err)
    }

    caCrtPath := filepath.Join(dir, "ca.crt")
    caKeyPath := filepath.Join(dir, "ca.key")

    // Read and parse CA cert (PEM)
    caCrtPEM, err := os.ReadFile(caCrtPath)
    if err != nil { return nil, fmt.Errorf("read ca.crt: %w", err) }
    caBlk, _ := pem.Decode(caCrtPEM)
    if caBlk == nil || caBlk.Type != "CERTIFICATE" { return nil, fmt.Errorf("invalid ca.crt") }
    caCert, err := x509.ParseCertificate(caBlk.Bytes)
    if err != nil { return nil, fmt.Errorf("parse ca.crt: %w", err) }

    // Read and parse CA key (PEM PKCS#1)
    caKeyPEM, err := os.ReadFile(caKeyPath)
    if err != nil { return nil, fmt.Errorf("read ca.key: %w", err) }
    caKeyBlk, _ := pem.Decode(caKeyPEM)
    if caKeyBlk == nil || caKeyBlk.Type != "RSA PRIVATE KEY" { return nil, fmt.Errorf("invalid ca.key") }
    caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlk.Bytes)
    if err != nil { return nil, fmt.Errorf("parse ca.key: %w", err) }

    if config == nil { config = MobileConfig() }

    // Generate client RSA key
    key, err := rsa.GenerateKey(rand.Reader, config.KeySize)
    if err != nil { return nil, fmt.Errorf("generate client key: %w", err) }

    // Build certificate template
    now := time.Now().UTC().Add(-5 * time.Minute)
    // For CA-signed client certs, set a friendly CN for server display; keep ApplicationURI in SAN
    cn := "OpcUaBaby"
    tmpl := &x509.Certificate{
        SerialNumber: new(big.Int).SetBytes(func() []byte { b := make([]byte, 16); _, _ = rand.Read(b); return b }()),
        Subject: pkix.Name{
            CommonName:         cn,
            Organization:       []string{config.Organization},
            OrganizationalUnit: []string{config.OrganizationalUnit},
            Country:            []string{config.Country},
            Province:           []string{config.Province},
            Locality:           []string{config.Locality},
        },
        NotBefore:             now,
        NotAfter:              now.Add(time.Duration(config.ValidityDays) * 24 * time.Hour),
        KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
        SignatureAlgorithm:    x509.SHA256WithRSA,
    }
    // SANs: ApplicationURI only (strict OPC UA profile). Place in URIs.
    if uri := strings.TrimSpace(config.ApplicationURI); uri != "" {
        if u, err := url.Parse(uri); err == nil {
            tmpl.URIs = []*url.URL{u}
        }
    }

    // No DNS/IP SANs by default unless explicitly provided
    if len(config.DNSNames) > 0 { tmpl.DNSNames = append(tmpl.DNSNames, config.DNSNames...) }
    if len(config.IPAddresses) > 0 { tmpl.IPAddresses = append(tmpl.IPAddresses, config.IPAddresses...) }

    if pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey); err == nil {
        sum := sha1.Sum(pubDER)
        tmpl.SubjectKeyId = sum[:]
    }
    // Set AuthorityKeyId from CA for better chain building
    if len(caCert.SubjectKeyId) > 0 {
        tmpl.AuthorityKeyId = append([]byte(nil), caCert.SubjectKeyId...)
    } else if caPubDER, err := x509.MarshalPKIXPublicKey(caCert.PublicKey); err == nil {
        sum := sha1.Sum(caPubDER)
        tmpl.AuthorityKeyId = sum[:]
    }

    // Sign with CA
    certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
    if err != nil { return nil, fmt.Errorf("create client cert: %w", err) }

    // Write files
    out := &genOutput{
        CertDERPath:  filepath.Join(dir, "client.der"),
        CertPEMPath:  filepath.Join(dir, "client.crt"),
        KeyPKCS1Path: filepath.Join(dir, "client.key"),
        KeyPKCS8Path: filepath.Join(dir, "client.pem"),
    }

    // client.der (DER cert)
    if err := os.WriteFile(out.CertDERPath, certDER, 0644); err != nil { return nil, fmt.Errorf("write client.der: %w", err) }
    // client.crt (PEM cert)
    if err := os.WriteFile(out.CertPEMPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0644); err != nil { return nil, fmt.Errorf("write client.crt: %w", err) }
    // client.key (PKCS#1 PEM)
    if err := os.WriteFile(out.KeyPKCS1Path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil { return nil, fmt.Errorf("write client.key: %w", err) }
    // client.pem (PKCS#8 PEM)
    if pkcs8, err := x509.MarshalPKCS8PrivateKey(key); err == nil {
        if err := os.WriteFile(out.KeyPKCS8Path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0600); err != nil { return nil, fmt.Errorf("write client.pem: %w", err) }
    } else {
        return nil, fmt.Errorf("marshal PKCS#8: %w", err)
    }

    return out, nil
}

// copyFile copies a file from src to dst creating parent directories if needed
func copyFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil { return err }
    defer in.Close()
    if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil { return err }
    out, err := os.Create(dst)
    if err != nil { return err }
    defer func() {
        _ = out.Close()
    }()
    if _, err := io.Copy(out, in); err != nil { return err }
    return out.Close()
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

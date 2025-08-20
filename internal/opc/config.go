package opc

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"opcuababy/internal/cert"
	"os"
	"strings"
	"time"

	"github.com/gopcua/opcua"
)

// Config holds all the necessary connection parameters for an OPC UA client.
type Config struct {
	EndpointURL      string
	SecurityPolicy   string
	SecurityMode     string
	AuthMode         string // "Anonymous", "Username", "Certificate"
	Username         string
	Password         string
	// UserTokenPolicyID allows explicitly specifying the server's UserIdentityToken PolicyID
	// (e.g., "anonymous", "username"). Some servers require the exact PolicyID; if not
	// provided and no endpoint probing is performed, authentication may fail with
	// StatusBadIdentityTokenInvalid.
	UserTokenPolicyID string `json:"user_token_policy_id,omitempty"`
	CertFile         string
	KeyFile          string
	ApplicationURI   string `json:"application_uri,omitempty"`
	ProductURI       string `json:"product_uri,omitempty"`
	// SessionName is the OPC UA Session Name sent in CreateSession.
	// If empty, it will default to ApplicationURI.
	SessionName      string `json:"session_name,omitempty"`
	SessionTimeout   uint32 `json:"session_timeout,omitempty"` // in seconds
	ApiPort          string
	ApiEnabled       bool    // Enable/disable the API/web server
	DisableLog       bool    // When true, suppress UI/API logs
	AutoConnect      bool    // Automatically connect on startup
	ConnectTimeout   float64 `json:"connect_timeout,omitempty"`    // Connection timeout in seconds
	// RetryAttempts controls how many times to try establishing a connection.
	// 0 or 1 means single attempt (no retries). If omitted/zero, controller will default to 3.
	RetryAttempts    int     `json:"retry_attempts,omitempty"`
	// RetryDelaySeconds is the delay between attempts. If omitted/zero, controller will default to 1s.
	RetryDelaySeconds float64 `json:"retry_delay_seconds,omitempty"`
	Language         string  `json:"language,omitempty"`           // UI language code: "en", "zh"
	AutoGenerateCert bool    `json:"auto_generate_cert,omitempty"` // Automatically generate certificates if missing
}

// ToOpcuaOptions converts the Config struct into a slice of opcua.Option
// that can be used to initialize the opcua.Client.
func (c *Config) ToOpcuaOptions() ([]opcua.Option, error) {
	var opts []opcua.Option

	// Prepare Application URI but defer appending until after we possibly read it from the certificate
	appURI := c.ApplicationURI
	if appURI == "" {
		if hn, err := os.Hostname(); err == nil && hn != "" {
			appURI = fmt.Sprintf("urn:%s:opcuababy", hn)
		} else {
			appURI = "urn:opcuababy:client"
		}
	}

	// Set Product URI if provided
	if c.ProductURI != "" {
		opts = append(opts, opcua.ProductURI(c.ProductURI))
	}

	// Set Session Timeout if provided
	if c.SessionTimeout > 0 {
		opts = append(opts, opcua.SessionTimeout(time.Duration(c.SessionTimeout)*time.Second))
	}

	// Set connection timeout
	if c.ConnectTimeout > 0 {
		opts = append(opts, opcua.DialTimeout(time.Duration(c.ConnectTimeout*float64(time.Second))))
	}

	// Security Policy/Mode: config-driven
	modeLower := strings.ToLower(strings.TrimSpace(c.SecurityMode))
	pol := strings.TrimSpace(c.SecurityPolicy)
	// Normalize minor spacing variations sometimes seen in inputs
	pol = strings.ReplaceAll(pol, " ", "")
	// Normalize policy: map short names to official URIs to avoid server rejection
	switch strings.ToLower(pol) {
	case "", "auto":
		if modeLower == "" || modeLower == "auto" || modeLower == "none" {
			pol = "None"
		} else {
			return nil, fmt.Errorf("security policy required for mode %s", c.SecurityMode)
		}
	case "none":
		pol = "None"
	case "basic128rsa15":
		pol = "http://opcfoundation.org/UA/SecurityPolicy#Basic128Rsa15"
	case "basic256":
		pol = "http://opcfoundation.org/UA/SecurityPolicy#Basic256"
	case "basic256sha256":
		pol = "http://opcfoundation.org/UA/SecurityPolicy#Basic256Sha256"
	case "aes128_sha256_rsaoaep", "aes128sha256rsaoaep", "basic256sha256rsaoaep":
		pol = "http://opcfoundation.org/UA/SecurityPolicy#Aes128_Sha256_RsaOaep"
	case "aes256_sha256_rsapss", "aes256sha256rsapss":
		pol = "http://opcfoundation.org/UA/SecurityPolicy#Aes256_Sha256_RsaPss"
	default:
		// If it's already a URI or exact None, accept
		if !strings.HasPrefix(strings.ToLower(pol), "http") && !strings.EqualFold(pol, "None") {
			return nil, fmt.Errorf("unsupported security policy: %s", c.SecurityPolicy)
		}
	}
	var modeStr string
	switch modeLower {
	case "", "auto":
		modeStr = "None"
	case "none":
		modeStr = "None"
	case "sign":
		modeStr = "Sign"
	case "signandencrypt":
		modeStr = "SignAndEncrypt"
	default:
		return nil, fmt.Errorf("unsupported security mode: %s", c.SecurityMode)
	}
	opts = append(opts, opcua.SecurityPolicy(pol))
	opts = append(opts, opcua.SecurityModeString(modeStr))

	// For Sign/SignAndEncrypt: support both with and without client certificate.
	// If CertFile/KeyFile are provided, load them; otherwise proceed without, as some servers allow it.
	requiresSecureChannel := strings.EqualFold(c.SecurityMode, "Sign") || strings.EqualFold(c.SecurityMode, "SignAndEncrypt")
	if requiresSecureChannel {
		if c.CertFile != "" && c.KeyFile != "" {
			// Load and parse private key (PEM or DER). Accept PKCS#1 and PKCS#8. Reject encrypted keys.
			keyBytes, err := os.ReadFile(c.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read key file: %w", err)
			}
			var keyDER []byte
			if b, _ := pem.Decode(keyBytes); b != nil {
				if x509.IsEncryptedPEMBlock(b) || len(b.Headers) > 0 {
					return nil, fmt.Errorf("encrypted private key is not supported: %s", c.KeyFile)
				}
				keyDER = b.Bytes
			} else {
				// Not PEM; assume DER
				keyDER = keyBytes
			}
			var rsaKey *rsa.PrivateKey
			if k1, err := x509.ParsePKCS1PrivateKey(keyDER); err == nil {
				rsaKey = k1
			} else if kIf, err := x509.ParsePKCS8PrivateKey(keyDER); err == nil {
				rk, ok := kIf.(*rsa.PrivateKey)
				if !ok {
					return nil, fmt.Errorf("private key is not RSA: %T", kIf)
				}
				rsaKey = rk
			} else {
				return nil, fmt.Errorf("failed to parse private key as PKCS#1 or PKCS#8 (PEM/DER)")
			}

			// Load certificate(s) and pick one matching the private key's public key. Support PEM chain or single DER.
			certBytes, err := os.ReadFile(c.CertFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read certificate file: %w", err)
			}
			var certDER []byte
			var leafCert *x509.Certificate
			if strings.Contains(string(certBytes), "-----BEGIN") {
				rest := certBytes
				ders := make([][]byte, 0, 4)
				certs := make([]*x509.Certificate, 0, 4)
				for {
					var block *pem.Block
					block, rest = pem.Decode(rest)
					if block == nil {
						break
					}
					if block.Type == "CERTIFICATE" {
						crt, err := x509.ParseCertificate(block.Bytes)
						if err == nil {
							ders = append(ders, block.Bytes)
							certs = append(certs, crt)
						}
					}
				}
				if len(certs) == 0 {
					return nil, fmt.Errorf("no CERTIFICATE block(s) found in PEM: %s", c.CertFile)
				}
				// try to match by public key
				picked := -1
				for i, crt := range certs {
					if pk, ok := crt.PublicKey.(*rsa.PublicKey); ok {
						if pk.N.Cmp(rsaKey.N) == 0 && pk.E == rsaKey.E {
							picked = i
							break
						}
					}
				}
				if picked >= 0 {
					certDER = ders[picked]
					leafCert = certs[picked]
				} else {
					certDER = ders[0]
					leafCert = certs[0]
				}
			} else {
				// assume DER-encoded single certificate
				var crt *x509.Certificate
				crt, err = x509.ParseCertificate(certBytes)
				if err != nil {
					return nil, fmt.Errorf("failed to parse certificate as DER: %w", err)
				}
				certDER = certBytes
				leafCert = crt
			}

			opts = append(opts, opcua.PrivateKey(rsaKey))
			opts = append(opts, opcua.Certificate(certDER))
			// Adopt ApplicationURI from certificate to satisfy server checks
			if leafCert != nil {
				// Prefer SAN URI
				if len(leafCert.URIs) > 0 && leafCert.URIs[0] != nil {
					if u := leafCert.URIs[0].String(); u != "" {
						appURI = u
					}
				} else {
					// If no URI SAN, use CN when it looks like a URN (our generator can set CN=ApplicationURI)
					if cn := strings.TrimSpace(leafCert.Subject.CommonName); cn != "" && (strings.HasPrefix(strings.ToLower(cn), "urn:") || strings.HasPrefix(strings.ToLower(cn), "http:") || strings.HasPrefix(strings.ToLower(cn), "https:")) {
						appURI = cn
					}
				}
			}
		} // else: no cert/key provided; proceed without them
	}

	// Finally, set the Application URI (deterministic from config/hostname)
	opts = append(opts, opcua.ApplicationURI(appURI))

	// Set SessionName: default to ApplicationURI when not provided
	if sn := strings.TrimSpace(c.SessionName); sn != "" {
		opts = append(opts, opcua.SessionName(sn))
	} else {
		opts = append(opts, opcua.SessionName(appURI))
	}

	// Authentication mode: always apply as configured
	switch strings.ToLower(strings.TrimSpace(c.AuthMode)) {
	case "username":
		opts = append(opts, opcua.AuthUsername(c.Username, c.Password))
		if pid := strings.TrimSpace(c.UserTokenPolicyID); pid != "" {
			opts = append(opts, opcua.AuthPolicyID(pid))
		}
	case "certificate":
		// Strict: do not fall back. We currently do not implement user-certificate token auth.
		return nil, fmt.Errorf("unsupported authentication mode: certificate (user-certificate token is not implemented)")
	case "anonymous", "":
		opts = append(opts, opcua.AuthAnonymous())
		if pid := strings.TrimSpace(c.UserTokenPolicyID); pid != "" {
			opts = append(opts, opcua.AuthPolicyID(pid))
		}
	default:
		return nil, fmt.Errorf("unsupported authentication mode: %s", c.AuthMode)
	}

	return opts, nil
}

// EnsureCertificates validates certificate files if paths are set.
// It NEVER generates or mutates certificate paths. Generation is manual via UI.
func (c *Config) EnsureCertificates() error {
	// If no paths set, nothing to validate here.
	if c.CertFile == "" && c.KeyFile == "" {
		return nil
	}
	// If one of them is missing, treat as invalid configuration.
	if c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("both certificate and key paths must be set or both empty")
	}
	// Validate existing files; never generate here.
	if err := cert.ValidateCertificateFiles(c.CertFile, c.KeyFile); err != nil {
		return fmt.Errorf("invalid certificate files: %w", err)
	}
	return nil
}

// GetCertificateInfo returns information about the current certificate
func (c *Config) GetCertificateInfo() (string, error) {
	if c.CertFile == "" {
		return "No certificate file configured", nil
	}

	return cert.GetCertificateInfo(c.CertFile)
}

package opc

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/gopcua/opcua"
)

// Config holds all the necessary connection parameters for an OPC UA client.
type Config struct {
	EndpointURL     string
	SecurityPolicy  string
	SecurityMode    string
	AuthMode        string // "Anonymous", "Username", "Certificate"
	Username        string
	Password        string
	CertFile        string
	KeyFile         string
	ApplicationURI  string  `json:"application_uri,omitempty"`
	ProductURI      string  `json:"product_uri,omitempty"`
	SessionTimeout  uint32  `json:"session_timeout,omitempty"` // in seconds
	ApiPort         string
	ApiEnabled      bool // Enable/disable the API/web server
	DisableLog      bool // When true, suppress UI/API logs
	AutoConnect     bool // Automatically connect on startup
	ConnectTimeout  float64 `json:"connect_timeout,omitempty"` // Connection timeout in seconds
	Language        string  `json:"language,omitempty"` // UI language code: "en", "zh"
}

// ToOpcuaOptions converts the Config struct into a slice of opcua.Option
// that can be used to initialize the opcua.Client.
func (c *Config) ToOpcuaOptions() ([]opcua.Option, error) {
	var opts []opcua.Option

	// Set Application URI
	appURI := c.ApplicationURI
	if appURI == "" {
		if hn, err := os.Hostname(); err == nil && hn != "" {
			appURI = fmt.Sprintf("urn:%s:opcuababy", hn)
		} else {
			appURI = "urn:opcuababy:client"
		}
	}
	opts = append(opts, opcua.ApplicationURI(appURI))

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

	// Set Security Policy/Mode
	if (c.SecurityPolicy == "" || c.SecurityPolicy == "Auto") || (c.SecurityMode == "" || c.SecurityMode == "Auto") {
		// If user chose Auto and auth is Anonymous, default to None/None
		// which commonly matches servers that allow anonymous without secure channel
		if c.AuthMode == "" || c.AuthMode == "Anonymous" {
			opts = append(opts, opcua.SecurityPolicy("None"))
			opts = append(opts, opcua.SecurityModeString("None"))
		}
		// else: leave unspecified; explicit username/cert flows should set security explicitly in config
	} else {
		// Respect explicit settings
		opts = append(opts, opcua.SecurityPolicy(c.SecurityPolicy))
		opts = append(opts, opcua.SecurityModeString(c.SecurityMode))
	}

	// Set Authentication Mode
	switch c.AuthMode {
	case "Username":
		if c.Username == "" {
			return nil, fmt.Errorf("username cannot be empty for username authentication")
		}
		opts = append(opts, opcua.AuthUsername(c.Username, c.Password))
	case "Certificate":
		if c.CertFile == "" || c.KeyFile == "" {
			return nil, fmt.Errorf("certificate and key file paths are required for certificate authentication")
		}
		cert, err := os.ReadFile(c.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}

		keyBytes, err := os.ReadFile(c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}

		block, _ := pem.Decode(keyBytes)
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM block containing private key")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			// Try parsing as PKCS8
			keyInterface, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err2 != nil {
				return nil, fmt.Errorf("failed to parse private key: %v / %v", err, err2)
			}
			var ok bool
			key, ok = keyInterface.(*rsa.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("private key is not an RSA key")
			}
		}

		opts = append(opts, opcua.AuthCertificate(cert), opcua.PrivateKey(key))
	default:
		// Do not force SecurityPolicy/Mode to None. Leave security selection
		// to explicit config (SecurityPolicy/SecurityMode) or higher-level logic.
		opts = append(opts, opcua.AuthAnonymous())
	}

	return opts, nil
}
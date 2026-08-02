package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

type TLSFiles struct {
	Cert string
	Key  string
}

func EnsureSelfSigned(dataDir string, hosts []string) (TLSFiles, error) {
	files := TLSFiles{
		Cert: filepath.Join(dataDir, "tls.crt"),
		Key:  filepath.Join(dataDir, "tls.key"),
	}
	if tlsFilesExist(files.Cert, files.Key) {
		ok, err := certIsTrustAnchor(files.Cert)
		if err != nil {
			return TLSFiles{}, err
		}
		if ok {
			return files, nil
		}
		_ = os.Remove(files.Cert)
		_ = os.Remove(files.Key)
	}

	key, der, err := generateSelfSigned(hosts)
	if err != nil {
		return TLSFiles{}, err
	}
	return files, writeTLSFiles(files.Cert, files.Key, der, key)
}

func tlsFilesExist(certFile, keyFile string) bool {
	if _, err := os.Stat(certFile); err != nil {
		return false
	}
	_, err := os.Stat(keyFile)
	return err == nil
}

func generateSelfSigned(hosts []string) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, nil, err
	}

	tmpl := selfSignedTemplate(serial, hosts)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return key, der, nil
}

func selfSignedTemplate(serial *big.Int, hosts []string) *x509.Certificate {
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"kiosk-display"},
			CommonName:   "kiosk-display",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		DNSNames:              tlsDNSNames(hosts),
		IPAddresses:           tlsIPs(hosts),
	}
}

func tlsIPs(hosts []string) []net.IP {
	out := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	seen := map[string]bool{
		"127.0.0.1":               true,
		net.IPv6loopback.String(): true,
	}
	for _, host := range hosts {
		ip := net.ParseIP(host)
		if ip == nil {
			continue
		}
		s := ip.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, ip)
	}
	return out
}

func tlsDNSNames(hosts []string) []string {
	names := []string{"localhost", "kiosk-display"}
	seen := map[string]bool{"localhost": true, "kiosk-display": true}
	for _, host := range hosts {
		if host == "" || net.ParseIP(host) != nil {
			continue
		}
		if seen[host] {
			continue
		}
		seen[host] = true
		names = append(names, host)
	}
	return names
}

func writeTLSFiles(certFile, keyFile string, der []byte, key *ecdsa.PrivateKey) error {
	if err := writePEM(certFile, 0o644, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return writePEM(keyFile, 0o600, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
}

func writePEM(path string, mode os.FileMode, block *pem.Block) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, block)
}

func certIsTrustAnchor(certFile string) (bool, error) {
	cert, err := parseCertPEM(certFile)
	if err != nil {
		return false, err
	}
	return cert.BasicConstraintsValid && cert.IsCA, nil
}

func parseCertPEM(certFile string) (*x509.Certificate, error) {
	b, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("tls cert: missing certificate pem")
	}
	return x509.ParseCertificate(block.Bytes)
}

func CertDERBase64(certFile string) (string, error) {
	cert, err := parseCertPEM(certFile)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cert.Raw), nil
}

func ClientConfig(certFile string) (*tls.Config, error) {
	cert, err := parseCertPEM(certFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{RootCAs: pool}, nil
}

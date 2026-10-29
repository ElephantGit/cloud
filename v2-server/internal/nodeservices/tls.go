package nodeservices

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// TLSFiles are the private Node service identity and public trust root. Only CACert may
// be published to an Agent; the client key never belongs in a checkout or an event.
type TLSFiles struct {
	CACert     string `json:"ca_cert"`
	ClientCert string `json:"client_cert"`
	ClientKey  string `json:"client_key"`
}

func localTLS(root string) (TLSFiles, *tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return TLSFiles{}, nil, err
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "v2 local Node services"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return TLSFiles{}, nil, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	issue := func(client bool) ([]byte, []byte, error) {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		if client {
			cert.SerialNumber = big.NewInt(3)
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			cert.DNSNames = nil
			cert.IPAddresses = nil
		}
		der, err := x509.CreateCertificate(rand.Reader, cert, ca, &k.PublicKey, key)
		if err != nil {
			return nil, nil, err
		}
		private, err := x509.MarshalPKCS8PrivateKey(k)
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), err
	}
	serverCert, serverKey, err := issue(false)
	if err != nil {
		return TLSFiles{}, nil, err
	}
	clientCert, clientKey, err := issue(true)
	if err != nil {
		return TLSFiles{}, nil, err
	}
	pair, err := tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		return TLSFiles{}, nil, err
	}
	files := TLSFiles{filepath.Join(root, "ca.pem"), filepath.Join(root, "client.pem"), filepath.Join(root, "client-key.pem")}
	for path, data := range map[string][]byte{files.CACert: caPEM, files.ClientCert: clientCert, files.ClientKey: clientKey} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			return TLSFiles{}, nil, err
		}
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(caPEM)
	return files, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}, ClientCAs: roots, ClientAuth: tls.VerifyClientCertIfGiven}, nil
}

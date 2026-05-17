package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/wzshiming/socks5"
)

var (
	http3Addr                  = "127.0.0.1:8989"
	socks5Addr                 = "127.0.0.1:1080"
	readTimeout  time.Duration = 30 * time.Second
	writeTimeout time.Duration = 30 * time.Second
	flushTimeout time.Duration = 10 * time.Millisecond
	client                     = http.Client{
		Transport: &http3.Transport{
			Dial: proxyDialer("socks5://" + socks5Addr),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{http3.NextProtoH3},
			},
		},
	}
)

func main() {
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		panic(err)
	}
	server := http3.Server{
		Addr:      http3Addr,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
		QUICConfig: &quic.Config{
			MaxIdleTimeout:          30 * time.Second,
			KeepAlivePeriod:         10 * time.Second,
			MaxIncomingStreams:      1000,
			MaxIncomingUniStreams:   100,
			HandshakeIdleTimeout:    10 * time.Second,
			DisablePathMTUDiscovery: false,
		},
	}
	server.Handler = func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handleTunnel(w, r)
		}
	}()
	fmt.Printf("HTTP3 server listening on %s\n", http3Addr)
	go server.ListenAndServe()
	fmt.Printf("SOCKS5 server listening on %s\n", socks5Addr)
	socks5.NewServer().ListenAndServe("tcp", socks5Addr)
}

func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("GenerateKey error: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"http3 socks proxy"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("CreateCertificate error: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("X509KeyPair error: %w", err)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{http3.NextProtoH3},
	}, nil
}

func handleTunnel(w http.ResponseWriter, r *http.Request) {
	r.URL.Host = r.Host
	r.URL.Scheme = "https"
	req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	copyHeader(req.Header, r.Header)
	delConnectionHeaders(req.Header)
	delHopHeaders(req.Header)
	remoteAddr := r.Context().Value(http3.RemoteAddrContextKey).(net.Addr)
	if remoteAddr != nil {
		appendHostToXForwardHeader(req.Header, remoteAddr.String())
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	rc := http.NewResponseController(w)
	go func() {
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.Tick(flushTimeout):
				err := rc.Flush()
				if err != nil {
					return
				}
				err = rc.SetReadDeadline(time.Now().Add(readTimeout))
				if err != nil {
					return
				}
				err = rc.SetWriteDeadline(time.Now().Add(writeTimeout))
				if err != nil {
					return
				}
			}
		}
	}()
	delConnectionHeaders(resp.Header)
	delHopHeaders(resp.Header)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	n, err := io.Copy(w, resp.Body)
	fmt.Printf("Copied %d bytes\n", n)
	if err != nil && !errors.Is(err, &quic.IdleTimeoutError{}) {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func proxyDialer(proxyURL string) func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	dialer, err := socks5.NewDialer(proxyURL)
	if err != nil {
		panic(err)
	}

	return func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
		proxyConn, err := dialer.DialContext(ctx, "udp", addr)
		if err != nil {
			return nil, err
		}

		remoteAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return nil, err
		}

		earlyConn, err := quic.DialEarly(ctx, proxyConn.(net.PacketConn), remoteAddr, tlsCfg, cfg)
		if err != nil {
			return nil, err
		}

		return earlyConn, nil
	}
}

// Hop-by-hop headers
// https://datatracker.ietf.org/doc/html/rfc2616#section-13.5.1
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func delHopHeaders(header http.Header) {
	for _, h := range hopHeaders {
		header.Del(h)
	}
}

// delConnectionHeaders removes hop-by-hop headers listed in the "Connection" header
// https://datatracker.ietf.org/doc/html/rfc7230#section-6.1
func delConnectionHeaders(h http.Header) {
	for _, f := range h["Connection"] {
		for sf := range strings.SplitSeq(f, ",") {
			if sf = strings.TrimSpace(sf); sf != "" {
				h.Del(sf)
			}
		}
	}
}

func appendHostToXForwardHeader(header http.Header, host string) {
	if prior, ok := header["X-Forwarded-For"]; ok {
		host = strings.Join(prior, ", ") + ", " + host
	}
	header.Set("X-Forwarded-For", host)
}

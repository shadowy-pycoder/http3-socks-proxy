package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

var (
	proxy    string
	username string
	password string
	address  string
)

func init() {
	flag.StringVar(&proxy, "a", "127.0.0.1:8989", "proxy address")
	flag.StringVar(&username, "u", "", "proxy username")
	flag.StringVar(&password, "p", "", "proxy password")
	flag.Usage = func() {
		fmt.Println(
			`Usage: client [-a PROXY -u USERNAME -p PASSWORD] URL
OPTIONS:
  -h    show this help message and exit`,
		)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.Arg(0) == "" {
		address = "www.google.com"
	} else {
		address = flag.Arg(0)
	}
}

func main() {
	client := &http.Client{
		Transport: &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ClientSessionCache: tls.NewLRUClientSessionCache(100),
			},
			Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
				return quic.DialAddrEarly(ctx, proxy, tlsCfg, cfg)
			},
		},
	}
	req, err := http.NewRequest(http3.MethodGet0RTT, fmt.Sprintf("https://%s", address), nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	if username != "" && password != "" {
		encoded := base64.StdEncoding.EncodeToString(fmt.Appendf([]byte{}, "%s:%s", username, password))
		req.Header.Set("Proxy-Authorization", "Basic "+encoded)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	fmt.Println(resp.Status)
	fmt.Println(resp.Proto)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			fmt.Println(string(buf[:n]))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			break
		}
	}
}

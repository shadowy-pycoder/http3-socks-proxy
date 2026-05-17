package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func main() {
	args := os.Args[1:]
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: ./client <proxy> <url>\nExample: ./client 127.0.0.1:8989 google.com\n")
		os.Exit(1)
	}
	client := &http.Client{
		Transport: &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
				return quic.DialAddrEarly(ctx, args[0], tlsCfg, cfg)
			},
		},
	}

	resp, err := client.Get(fmt.Sprintf("https://%s", args[1]))
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

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	defer transport.Close()

	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", "https://localhost:8989", nil)
	req.Host = "google.com"

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(resp.Status)
	fmt.Println(string(body[:200]))
}

# Simple HTTP3 to SOCKS5 proxy example

## Flow

```
Client (Go)
        |
        | QUIC / UDP  (HTTP/3)
        | TLS encrypted
        v
127.0.0.1:8989  [HTTP/3 Proxy - quic-go]
        |
        | reads r.Host -> "google.com"
        | sets r.URL.Scheme = "https"
        | sets r.URL.Host = "google.com:443"
        |
        | TCP  (SOCKS5 handshake)
        v
127.0.0.1:1080  [SOCKS5 Server]
        |
        | UDP ASSOCIATE -> relay addr
        | wraps QUIC packets in SOCKS5 UDP header
        |
        | UDP (QUIC tunneled through SOCKS5)
        v
142.250.x.x:443  [google.com]
        |
        | HTTP/3 response
        v
127.0.0.1:1080  [SOCKS5 relay]
        |
        v
127.0.0.1:8989  [HTTP/3 Proxy]
        |
        | io.Copy response body -> client
        v
Client
```

## Usage

Start the server in one terminal:

```shell
go run ./cmd/proxy/main.go
```

Run the client in another terminal:

```shell
Usage: client [-a PROXY -u USERNAME -p PASSWORD] URL
OPTIONS:
  -h    show this help message and exit
  -a string
        proxy address (default "127.0.0.1:8989")
  -p string
        proxy password
  -u string
        proxy username
```

```shell
go run ./cmd/client/main.go
```

Or set options explicitly to connect to arbitrary HTTP3 proxy server:

```shell
go run ./cmd/client/main.go -a 127.0.0.1:8080 -u username -p password google.com
```

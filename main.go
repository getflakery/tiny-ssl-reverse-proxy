package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/sensiblecodeio/tiny-ssl-reverse-proxy/proxyprotocol"
)

// interface routers {
//     [key: string]: {
//         service: string
//     }
// }

// interface services {
//     [key: string]: {
//         servers: {
//             url: string
//         }[]
//     }
// }

// // log body
// return {
//     "http": {
//         "routers": {
//             ...routers
//         },
//         "services": {
//             ...services
//         }
//     }
// }

// create go structs for parsing the json

type Routers struct {
	Service string `json:"service"`
}

type Servers struct {
	URL string `json:"url"`
}

type Services struct {
	Servers []Servers `json:"servers"`
}

type Http struct {
	Routers  map[string]Routers  `json:"routers"`
	Services map[string]Services `json:"services"`
}

type Config struct {
	Http Http `json:"http"`
}

func parseConfig(config []byte) (Config, error) {
	var c Config
	err := json.Unmarshal(config, &c)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}

// Version number
const Version = "0.23.0"

var message = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>
Backend Unavailable
</title>
<style>
body {
	font-family: fantasy;
	text-align: center;
	padding-top: 20%;
	background-color: #f1f6f8;
}
</style>
</head>
<body>
<h1>503 Backend Unavailable</h1>
<p>Sorry, we&lsquo;re having a brief problem. You can retry.</p>
<p>If the problem persists, please get in touch.</p>
</body>
</html>`

type ConnectionErrorHandler struct{ http.RoundTripper }

func (c *ConnectionErrorHandler) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.RoundTripper.RoundTrip(req)
	if err != nil {
		log.Printf("Error: backend request failed for %v: %v",
			req.RemoteAddr, err)
	}
	if _, ok := err.(*net.OpError); ok {
		r := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       ioutil.NopCloser(bytes.NewBufferString(message)),
		}
		return r, nil
	}
	return resp, err
}

func main() {
	var (
		listen, cert, key, where           string
		useTLS, useLogging, behindTCPProxy bool
		flushInterval                      time.Duration
	)
	flag.StringVar(&listen, "listen", ":443", "Bind address to listen on")
	// certFile = "/var/lib/acme/flakery.xyz/cert.pem";
	// keyFile = "";
	flag.StringVar(&key, "key", "/var/lib/acme/flakery.xyz/key.pem", "Path to PEM key")
	flag.StringVar(&cert, "cert", "/var/lib/acme/flakery.xyz/cert.pem", "Path to PEM certificate")
	flag.StringVar(&where, "where", "http://10.0.4.20:3000", "Place to forward connections to")
	flag.BoolVar(&useTLS, "tls", true, "accept HTTPS connections")
	flag.BoolVar(&useLogging, "logging", true, "log requests")
	flag.BoolVar(&behindTCPProxy, "behind-tcp-proxy", false, "running behind TCP proxy (such as ELB or HAProxy)")
	flag.DurationVar(&flushInterval, "flush-interval", 0, "minimum duration between flushes to the client (default: off)")
	oldUsage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\n%v version %v\n\n", os.Args[0], Version)
		oldUsage()
	}
	flag.Parse()

	var handler http.Handler

	ttlCache := NewTTLCache(5 * time.Second)
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_version" {
			w.Header().Add("X-Tiny-SSL-Version", Version)
		}
		r.Header.Set("X-Forwarded-Proto", "https")
		// print request url
		c, err := ttlCache.Get()
		if err != nil {
			log.Printf("Error: %v", err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}
		// fmt.Fprintf(w, "Cache: %s\n", c)
		config, err := parseConfig(c)
		if err != nil {
			log.Printf("Error: %v", err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}
		service := config.Http.Routers[r.Host].Service
		servers := config.Http.Services[service].Servers
		// pick random server
		server := servers[0].URL
		parsed, err := url.Parse(server)
		if err != nil {
			log.Fatalln("Fatal parsing -where:", err)
		}
		h := httputil.NewSingleHostReverseProxy(parsed)
		h.Transport = &ConnectionErrorHandler{http.DefaultTransport}
		h.FlushInterval = flushInterval

		h.ServeHTTP(w, r)
	})

	if useLogging {
		handler = &LoggingMiddleware{handler}
	}

	server := &http.Server{Addr: listen, Handler: handler}

	var err error

	switch {
	case useTLS && behindTCPProxy:
		err = proxyprotocol.BehindTCPProxyListenAndServeTLS(server, cert, key)
	case behindTCPProxy:
		err = proxyprotocol.BehindTCPProxyListenAndServe(server)
	case useTLS:
		err = server.ListenAndServeTLS(cert, key)
	default:
		err = server.ListenAndServe()
	}

	log.Fatalln(err)
}

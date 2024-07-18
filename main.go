package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/samber/lo"
	"github.com/sensiblecodeio/tiny-ssl-reverse-proxy/proxyprotocol"
)

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

var unhealthyHosts = map[Servers]bool{}

type ConnectionErrorHandler struct {
	http.RoundTripper
	slog.Logger
	server Servers
}

func (c *ConnectionErrorHandler) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.RoundTripper.RoundTrip(req)
	if err != nil {
		c.Error("backend request failed", "err", err, "remoteAddr", req.RemoteAddr, "url", req.URL.String(), "host", req.Host)
		c.Info("marking server as unhealthy", "server", c.server)
		// mark server as unhealthy
		unhealthyHosts[c.server] = true
	}
	if _, ok := err.(*net.OpError); ok {
		c.Error("backend connection failed", "err", err, "remoteAddr", req.RemoteAddr, "url", req.URL.String(), "host", req.Host)
		r := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       ioutil.NopCloser(bytes.NewBufferString(message)),
		}
		return r, nil
	}
	return resp, err
}

func healthyServers(s []Servers, unhealthyHosts map[Servers]bool) []Servers {
	return lo.Filter(s, func(s Servers, _ int) bool {
		_, ok := unhealthyHosts[s]
		return !ok
	})
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("starting", "version", Version)

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
		if r.Host == "loadb.flakery.xyz" {
			// print 🌨️
			fmt.Fprintf(w, "🌨️\n")
			logger.Info("🌨️")
			return
		}
		if r.Host == "woodpecker-ci-19fcc5.flakery.xyz" {
			// check for host header X-Flakery-User-key
			logger.Info("checking for user key")
			userKey := r.Header.Get("X-Flakery-User-Key")
			if userKey == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			logger.Info("user key", "key", userKey)
		}
		r.Header.Set("X-Forwarded-Proto", "https")
		// print request url
		c, err := ttlCache.Get()
		if err != nil {
			logger.Error("error getting ttl cache", "err", err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}
		// fmt.Fprintf(w, "Cache: %s\n", c)
		config, err := parseConfig(c)
		if err != nil {
			logger.Error("error parsing config", "err", err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}

		fmt.Println("Host: ", r.Host)
		logger.Info("request", "host", r.Host, "url", r.URL.String())

		router, ok := config.Http.Routers[r.Host]
		if !ok {
			logger.Error("service not found", "service", r.Host)
			http.Error(w, "Service not found", http.StatusNotFound)
			return
		}
		servers := config.Http.Services[router.Service].Servers
		// filter unhealthy servers
		if len(servers) > 1 { // temporary hack to avoid empty server list
			servers = healthyServers(servers, unhealthyHosts)
		}
		// pick random server
		num := len(servers)
		if num == 0 {
			logger.Error("no servers found", "service", r.Host)
			http.Error(w, "No servers found", http.StatusServiceUnavailable)
			return
		}
		rnum, err := rand.Int(
			rand.Reader,
			big.NewInt(int64(num)),
		)
		if err != nil {
			logger.Error("error getting random number", "err", err)
			http.Error(w, "Error", http.StatusInternalServerError)
			return
		}
		serverIndex := rnum.Int64()
		serv := servers[serverIndex]
		server := servers[serverIndex].URL
		parsed, err := url.Parse(server)
		if err != nil {
			logger.Error("error parsing server", "err", err)
		}
		h := httputil.NewSingleHostReverseProxy(parsed)
		h.Transport = &ConnectionErrorHandler{
			http.DefaultTransport,
			*logger,
			serv,
		}
		h.FlushInterval = flushInterval

		h.ServeHTTP(w, r)
	})

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

	logger.Error("server error", "err", err)
}

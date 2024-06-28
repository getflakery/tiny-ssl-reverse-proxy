package main

import (
	"bytes"
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

	url, err := url.Parse(where)
	if err != nil {
		log.Fatalln("Fatal parsing -where:", err)
	}

	httpProxy := httputil.NewSingleHostReverseProxy(url)
	httpProxy.Transport = &ConnectionErrorHandler{http.DefaultTransport}
	httpProxy.FlushInterval = flushInterval

	var handler http.Handler

	handler = httpProxy

	otherUlr := "http://10.0.4.125:8080"
	otherProxy := httputil.NewSingleHostReverseProxy(url)
	otherProxy.Transport = &ConnectionErrorHandler{http.DefaultTransport}
	otherProxy.FlushInterval = flushInterval


	originalHandler := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_version" {
			w.Header().Add("X-Tiny-SSL-Version", Version)
		}
		r.Header.Set("X-Forwarded-Proto", "https")
		// print request url
		log.Printf("Request URL: %v", r.Host)
		if r.Host == "grafana.01cef0.flakery.xyz" {
			originalHandler.ServeHTTP(w, r)
		} else {
			otherProxy.ServeHTTP(w, r)
		}
	})

	if useLogging {
		handler = &LoggingMiddleware{handler}
	}

	server := &http.Server{Addr: listen, Handler: handler}

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

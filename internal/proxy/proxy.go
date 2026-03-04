package proxy

import (
	"io"
	"net"
	"net/http"
)

// Hop-by-hop headers that must not be forwarded by a proxy.
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// transport used for outgoing requests — explicitly no proxy to avoid loops.
var transport = &http.Transport{
	Proxy: nil,
}

// NewServer returns an HTTP forward proxy server.
func NewServer() *http.Server {
	return &http.Server{
		Handler: http.HandlerFunc(handleProxy),
	}
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		handleConnect(w, r)
		return
	}
	handleHTTP(w, r)
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	dest, err := net.DialTimeout("tcp", r.Host, 10*1e9)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		dest.Close()
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		dest.Close()
		return
	}

	go func() {
		defer dest.Close()
		defer client.Close()
		io.Copy(dest, client)
	}()
	go func() {
		defer dest.Close()
		defer client.Close()
		io.Copy(client, dest)
	}()
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	r.RequestURI = ""

	for _, h := range hopByHopHeaders {
		r.Header.Del(h)
	}

	resp, err := transport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for _, h := range hopByHopHeaders {
		resp.Header.Del(h)
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

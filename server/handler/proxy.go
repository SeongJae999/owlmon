package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// ProxyHandler는 Prometheus API를 인증된 요청에 한해 프록시합니다.
type ProxyHandler struct {
	proxy *httputil.ReverseProxy
}

func NewProxyHandler(prometheusURL string) (*ProxyHandler, error) {
	target, err := url.Parse(prometheusURL)
	if err != nil {
		return nil, err
	}
	return &ProxyHandler{proxy: httputil.NewSingleHostReverseProxy(target)}, nil
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS 불필요 — 프론트는 nginx(운영)/vite proxy(개발)로 같은 origin에서 호출
	h.proxy.ServeHTTP(w, r)
}

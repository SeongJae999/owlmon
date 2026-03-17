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
	// CORS 헤더 (프론트엔드에서 호출 가능하도록)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.proxy.ServeHTTP(w, r)
}

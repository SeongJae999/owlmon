package collector

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// 서버가 IP 대역으로 그룹을 자동 지정할 수 있게 장비 IP를 라벨로 보냄 (값은 늘 1)
func NewHostInfoCollector(ctx context.Context, meter metric.Meter, endpoint string) error {
	g, err := meter.Int64ObservableGauge("owlmon.agent.info", metric.WithDescription("에이전트 정보 (라벨: host.ip)"))
	if err != nil {
		return fmt.Errorf("agent info 게이지 생성 실패: %w", err)
	}
	// 수집 콜백은 순차 실행 — 여기서 DNS를 기다리면 같은 주기의 CPU·메모리 전송까지 밀리므로 따로 구해 둠
	var ip atomic.Value
	ip.Store(hostIP(endpoint))
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				ip.Store(hostIP(endpoint))
			}
		}
	}()
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if v := ip.Load().(string); v != "" {
			o.ObserveInt64(g, 1, metric.WithAttributes(attribute.String("host.ip", v)))
		}
		return nil
	}, g)
	return err
}

// 도커 브리지·VPN 등 여러 NIC 중 OWLmon으로 나가는 경로의 주소 — 운영자가 아는 관리 IP.
// 수집기가 같은 장비(localhost)면 기본 경로 주소로. UDP 연결이라 패킷은 나가지 않음
func hostIP(endpoint string) string {
	if v := os.Getenv("OWLMON_HOST_IP"); v != "" {
		return v
	}
	if ip := routeIP(endpoint); ip != "" {
		return ip
	}
	return routeIP("192.0.2.1:9") // 문서용 주소(TEST-NET-1) — 기본 경로만 고르게 함
}

func routeIP(addr string) string {
	c, err := net.DialTimeout("udp4", addr, 3*time.Second)
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() {
		return a.IP.String()
	}
	return ""
}

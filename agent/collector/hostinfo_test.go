package collector

import "testing"

func TestHostIP(t *testing.T) {
	// 수집기가 같은 장비여도 루프백이 아닌 주소(기본 경로)를 보냄
	if ip := hostIP("127.0.0.1:4317"); ip == "127.0.0.1" {
		t.Fatalf("루프백을 보냄: %s", ip)
	}
	t.Setenv("OWLMON_HOST_IP", "10.0.0.7")
	if ip := hostIP("127.0.0.1:4317"); ip != "10.0.0.7" {
		t.Fatalf("직접 지정 무시: %s", ip)
	}
}

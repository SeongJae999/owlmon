// Package prom은 Prometheus 공통 헬퍼입니다.
//
// 같은 패턴 코드가 server/handler/status.go, server/alert/checker.go,
// server/report/reporter.go 3곳에 중복되어 한 곳 fix하면 다른 곳 빼먹는
// 사고가 반복됨 (실제로 발생) → 공통 헬퍼로 추출.
package prom

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// ActiveHosts는 진짜 활성 호스트(최근 데이터를 보낸 호스트)만 반환합니다.
//
// 왜 labelValues API를 쓰지 않는가:
//   - /api/v1/label/host_name/values 는 retention 기간(기본 30일) 안에 한 번이라도
//     데이터를 보낸 모든 라벨을 반환.
//   - 죽은 에이전트, 옛 hostname, 일회성 노드가 다 포함됨 (stale).
//   - 이 stale 호스트가 오버뷰에 표시되고, down 알람 폭주를 일으키고, 월간 보고서에
//     "한 달 내내 다운"으로 나타나는 문제 발생.
//
// instant query 동작:
//   - PromQL `count(system_cpu_usage_percent) by (host_name)` 평가
//   - Prometheus lookback delta(기본 5분) 안에 데이터 있는 시리즈만 반환
//   - 진짜 살아있는 호스트만 잡힘
func ActiveHosts(prometheusURL string) ([]string, error) {
	q := url.QueryEscape("count(system_cpu_usage_percent) by (host_name)")
	resp, err := httpClient.Get(prometheusURL + "/api/v1/query?query=" + q)
	if err != nil {
		return nil, fmt.Errorf("prometheus query 실패: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("prometheus 응답 파싱 실패: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus query 실패 (status=%s)", result.Status)
	}

	hosts := make([]string, 0, len(result.Data.Result))
	for _, r := range result.Data.Result {
		if name := r.Metric["host_name"]; name != "" {
			hosts = append(hosts, name)
		}
	}
	return hosts, nil
}

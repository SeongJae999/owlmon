package prom

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// httpClient는 prom 패키지의 모든 호출이 공유한다.
// Prometheus가 행에 걸려도 호출자(30초 주기 알림 체커 등)가 무한 블로킹되지 않도록 타임아웃 필수.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Result는 instant query 결과의 시리즈 1개 (라벨 + 값).
type Result struct {
	Metric map[string]string
	Value  float64
}

// Query는 instant query를 실행해 시리즈 목록을 반환한다.
// (종전: checker.go / status.go / reporter.go에 같은 HTTP+파싱 코드가 3벌 중복)
func Query(prometheusURL, promql string) ([]Result, error) {
	return QueryAt(prometheusURL, promql, time.Time{})
}

// QueryAt은 특정 시각 기준 instant query (월간 보고서용). at이 zero면 현재 시각.
func QueryAt(prometheusURL, promql string, at time.Time) ([]Result, error) {
	params := url.Values{}
	params.Set("query", promql)
	if !at.IsZero() {
		params.Set("time", strconv.FormatInt(at.Unix(), 10))
	}

	resp, err := httpClient.Get(strings.TrimRight(prometheusURL, "/") + "/api/v1/query?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]interface{}    `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	out := make([]Result, 0, len(result.Data.Result))
	for _, r := range result.Data.Result {
		valStr, _ := r.Value[1].(string)
		val, _ := strconv.ParseFloat(valStr, 64)
		out = append(out, Result{Metric: r.Metric, Value: val})
	}
	return out, nil
}

// LabelValues는 라벨 값 목록을 반환한다.
// host_name은 stale 호스트(retention 내 한 번이라도 송신한 모든 라벨)가 섞이는
// labelValues API 대신 ActiveHosts(instant query)를 우선 사용한다.
func LabelValues(prometheusURL, label string) ([]string, error) {
	if label == "host_name" {
		if hosts, err := ActiveHosts(prometheusURL); err == nil {
			return hosts, nil
		}
		// instant query 실패 시 기본 labelValues로 폴백
	}

	resp, err := httpClient.Get(strings.TrimRight(prometheusURL, "/") + "/api/v1/label/" + label + "/values")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

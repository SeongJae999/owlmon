package handler

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/seongJae/owlmon/server/llm"
)

type routeCase struct {
	q, prev string
	want    []string
}

func rc(q string, want ...string) routeCase { return routeCase{q: q, want: want} }
func rf(prev, q string, want ...string) routeCase {
	return routeCase{q: q, prev: prev, want: want}
}

var routeDev = []routeCase{
	rc("지금 서버들 괜찮아?", routeOverview), rc("오늘 뭐부터 봐야 돼?", routeOverview), rc("서버 좀 봐줘", routeOverview), rc("제일 급한 게 뭐야?", routeOverview),
	rc("web-01 왜 이렇게 느려?", routeHost), rc("db-01 디스크 얼마나 남았어?", routeHost, routeDisk), rc("web-01 CPU 몇 퍼센트야?", routeHost), rc("print-01 상태 어때?", routeHost),
	rf("web-01 왜 이렇게 느려?", "메모리는 어때?", routeHost),
	rc("어제 밤에 무슨 일 있었어?", routeHistory), rc("지난 3일간 알림 몇 건 왔어?", routeHistory), rc("어제 알림 몇 건 왔어?", routeHistory), rc("지난주 장애 몇 번 있었어?", routeHistory),
	rc("인증서 언제 만료돼?", routeSSL), rc("디스크 언제 꽉 차?", routeDisk), rc("db-01 디스크 언제 차?", routeDisk, routeHost),
	rc("gpu-server에서 out of memory 로그 있었어?", routeInsights, routeHost), rc("백업 실패한 서버 있어?", routeInsights), rc("최근 로그에 이상한 거 있어?", routeInsights),
	rc("메모리 제일 많이 쓰는 서버는?", routeUsage), rc("CPU 70 넘는 서버 몇 대야?", routeUsage),
	rc("nginx 502 오류는 보통 왜 생겨?", routeNone), rc("SNMP가 뭐야?", routeNone), rc("SSH 무차별 대입 공격을 막으려면?", routeNone), rc("서버 응답이 느릴 때 뭘 먼저 확인해?", routeNone), rc("디스크가 가득 차면 어떻게 대처해?", routeNone),
	rc("db-01 재시작해줘", routeAction), rc("이 알림 꺼줘", routeAction), rc("web-01 로그 지워줘", routeAction),
	rc("사이트가 안 열려요", routeOverview), rc("인터넷이 이상해요", routeOverview),
}

var routeHoldout = []routeCase{
	rc("문제 있는 서버 있어?", routeOverview), rc("다 잘 돌아가고 있어?", routeOverview), rc("지금 꺼진 서버 있나요", routeOverview), rc("이상 없어?", routeOverview), rc("오늘 서버 상태 알려줘", routeOverview),
	rc("경고 뜬 거 뭐야?", routeOverview), rc("급하게 처리할 일 있어?", routeOverview), rc("서버들 어때", routeOverview), rc("전체 요약해줘", routeOverview), rc("현재 서버 상태가 궁금해요", routeOverview),
	rc("오늘 확인할 거 있어?", routeOverview), rc("별일 없지?", routeOverview), rc("서버 다운된 거 있어?", routeOverview), rc("위험한 서버 있어?", routeOverview), rc("주의할 서버 알려줘", routeOverview), rc("아침 점검 해줘", routeOverview), rc("전체 서버 현황 보여줘", routeOverview),
	rc("홈페이지가 안 열려요", routeOverview), rc("메일이 안 돼요", routeOverview),
	rc("DB-01 어때?", routeHost), rc("app-02 메모리 왜 이렇게 높아?", routeHost), rc("nas-01 살아있어?", routeHost), rc("mail-gw 상태", routeHost), rc("web-02는 괜찮아?", routeHost), rc("srv-99 디스크 몇 프로야", routeHost),
	rc("backup-01 디스크 얼마나 썼어?", routeHost), rc("gpu-server CPU 왜 높아?", routeHost), rc("APP-01 확인해줘", routeHost),
	rf("db-01 어때?", "그럼 얼마나 더 버텨?", routeDisk, routeHost),
	rc("오늘 알림 왔어?", routeHistory), rc("그저께 장애 있었어?", routeHistory), rc("이번 달 알림 몇 건이야?", routeHistory), rc("11월 3일에 무슨 일 있었어?", routeHistory), rc("최근 7일 경고 내역 보여줘", routeHistory), rc("저번주에 서버 죽은 적 있어?", routeHistory),
	rc("어제 장애 있었어?", routeHistory), rc("지난달 알림 통계 알려줘", routeHistory), rc("이번 주에 경고 몇 번 왔어?", routeHistory), rc("최근 3시간 동안 무슨 일 있었어?", routeHistory), rc("오늘 새벽에 알림 왔었어?", routeHistory),
	rc("SSL 인증서 괜찮아?", routeSSL), rc("홈페이지 인증서 언제까지야?", routeSSL), rc("인증서 만료 임박한 거 있어?", routeSSL), rc("도메인 인증서 상태 어때?", routeSSL),
	rc("용량 언제 다 차?", routeDisk), rc("디스크 부족해질 서버 있어?", routeDisk), rc("디스크 얼마 안 남은 서버 있어?", routeDisk, routeUsage), rc("며칠 뒤에 디스크 꽉 차?", routeDisk),
	rc("CPU 많이 쓰는 서버 알려줘", routeUsage), rc("메모리 90 이상인 서버 있어?", routeUsage), rc("디스크 사용률 순서대로 보여줘", routeUsage), rc("CPU 사용률 높은 순으로 알려줘", routeUsage), rc("메모리 부족한 서버 있어?", routeUsage),
	rc("해킹당한 거 아니야?", routeInsights), rc("에러 로그 많은 서버 어디야?", routeInsights), rc("백업 잘 됐어?", routeInsights), rc("보안 문제 있어?", routeInsights), rc("최근 보안 로그 요약해줘", routeInsights), rc("누가 서버 뚫으려고 한 흔적 있어?", routeInsights), rc("SSH 로그인 실패 많이 났어?", routeInsights),
	rc("CPU가 뭐야?", routeNone), rc("디스크 정리는 어떻게 해?", routeNone), rc("방화벽 설정 방법 알려줘", routeNone), rc("DNS가 무엇인가요?", routeNone), rc("메모리랑 디스크 차이가 뭐야?", routeNone), rc("서버 재부팅해도 돼?", routeNone), rc("안녕", routeNone),
	rc("OWLmon은 어떤 프로그램이야?", routeNone), rf("서버 상태 어때?", "DNS는 뭐야?", routeNone), rc("리눅스에서 디스크 용량 확인하는 명령어 알려줘", routeNone), rc("RAID가 뭔가요?", routeNone), rc("포트 포워딩이 무엇인지 설명해줘", routeNone), rc("메모리 누수가 뭐야?", routeNone), rc("502 에러 원인 알려줘", routeNone),
	rc("윈도우 서버 업데이트는 얼마나 자주 해야 돼?", routeNone), rc("고마워", routeNone), rc("로그 보관 기간은 어떻게 정해?", routeNone), rc("인증서가 뭐야?", routeNone), rc("SSL 인증서 갱신은 보통 어떻게 해?", routeNone), rc("백업은 왜 해야 돼?", routeNone),
	rc("서버 껐다 켜줘", routeAction), rc("디스크 정리해줘", routeAction), rc("알림 임계값 90으로 바꿔줘", routeAction), rc("nas-01 전원 꺼줘", routeAction), rc("이 사용자 차단해줘", routeAction), rc("nginx 재시작 부탁해", routeAction),
}

func TestChatRouterAccuracy(t *testing.T) {
	split := os.Getenv("OWLMON_CHAT_ROUTER_EVAL")
	if split == "" {
		t.Skip("실제 모델 측정은 OWLMON_CHAT_ROUTER_EVAL=dev|holdout 필요")
	}
	op, ok := llm.NewProvider().(*llm.OllamaProvider)
	if !ok {
		t.Fatal("OWLMON_LLM_PROVIDER=ollama 필요")
	}
	router := *op
	router.JSON = true
	cases := routeDev
	if split == "holdout" {
		cases = routeHoldout
	}
	okN := 0
	confusion := map[string]int{}
	var lat []time.Duration
	for _, c := range cases {
		start := time.Now()
		got, err := classifyRoute(context.Background(), &router, c.prev, c.q)
		lat = append(lat, time.Since(start))
		if err != nil {
			t.Logf("오류 %q: %v", c.q, err)
			continue
		}
		hit := false
		for _, w := range c.want {
			hit = hit || w == got
		}
		if hit {
			okN++
			continue
		}
		confusion[c.want[0]+" 정답, "+got+" 분류"]++
		t.Logf("X %q 정답=%s 분류=%s", c.q, strings.Join(c.want, "|"), got)
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	t.Logf("%s: %d/%d, 중앙값 %v, 최대 %v, 혼동 %v", split, okN, len(cases), lat[len(lat)/2].Round(100*time.Millisecond), lat[len(lat)-1].Round(100*time.Millisecond), confusion)
}

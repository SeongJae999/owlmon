# OWLmon 에이전트 설치 가이드

> 새 호스트(서버)에 OWLmon 에이전트를 배포하는 운영자/SI용 매뉴얼.
>
> 대상 OS: Ubuntu 18.04+, Debian 10+, RHEL/Rocky/AlmaLinux/Oracle/CentOS Stream 7+, OpenSUSE Leap 15+

---

## 사전 준비

| 항목 | 확인 |
|-----|------|
| 운영 OWLmon 서버 URL | 예: `http://192.168.0.30:8080` |
| OTLP 엔드포인트 | 예: `192.168.0.30:4317` |
| Agent Key | 운영 서버의 `.env`의 `OWLMON_AGENT_KEY` 값 (모든 에이전트 공통) |
| SSH 접근 | 대상 서버에 root 또는 sudo 가능 |
| 네트워크 | 대상 서버 → OWLmon 서버 8080/4317 도달 가능 |

---

## 설치 (3가지 방법)

### 방법 A — 사전 빌드 바이너리로 (Go 불필요, 권장)

운영 OWLmon 서버에서 binary 한 번 빌드해 두고, 각 호스트로 scp.

```bash
# 1) OWLmon 서버에서 빌드 (한 번만)
ssh owlmon@192.168.0.30
cd ~/owlmon/agent
docker run --rm -v $PWD:/src -w /src -e CGO_ENABLED=0 -e GOOS=linux \
  golang:1.26-alpine \
  go build -ldflags="-s -w" -o owlmon-agent .
exit

# 2) 본 머신으로 가져오기
scp owlmon@192.168.0.30:~/owlmon/agent/owlmon-agent /tmp/

# 3) 대상 호스트로 전송
scp /tmp/owlmon-agent root@<TARGET_HOST>:/tmp/
scp ~/mango/owlmon/scripts/install-agent-linux.sh root@<TARGET_HOST>:/tmp/

# 4) 대상 호스트에서 설치
ssh root@<TARGET_HOST>
bash /tmp/install-agent-linux.sh \
  --binary /tmp/owlmon-agent \
  --server http://192.168.0.30:8080 \
  --otlp   192.168.0.30:4317 \
  --agent-key <YOUR_AGENT_KEY> \
  --collect-interval 15s \
  --journald
```

### 방법 B — Go 소스에서 빌드

대상 호스트에 Go 1.26+ 설치돼 있는 경우.

```bash
# 1) 본 머신에서 repo clone 후 scp
git clone https://github.com/SeongJae999/owlmon.git
scp -r owlmon/ root@<TARGET_HOST>:/tmp/

# 2) 대상 호스트에서 설치 (script가 자동 빌드)
ssh root@<TARGET_HOST>
cd /tmp/owlmon
sudo bash scripts/install-agent-linux.sh \
  --build \
  --server http://192.168.0.30:8080 \
  --otlp   192.168.0.30:4317 \
  --agent-key <YOUR_AGENT_KEY> \
  --journald
```

### 방법 C — 수동 설치 (스크립트 안 쓸 때)

[install-agent-linux.sh](../scripts/install-agent-linux.sh) 소스 참고.

---

## 옵션

| 옵션 | 기본값 | 설명 |
|-----|------|------|
| `--server` | `http://localhost:8080` | OWLmon 서버 HTTP URL (로그 전송) |
| `--otlp` | `localhost:4317` | OTLP gRPC 엔드포인트 (메트릭 전송) |
| `--agent-key` | (빈 값, 401 됨) | 로그 인증 키. 운영 서버 `.env` 와 일치 |
| `--collect-interval` | `15s` | 메트릭 수집 주기 (5s ~ 1m) |
| `--journald` | (꺼짐) | systemd journal 로그 수집 (Debian 12 / RHEL 7+ 권장) |
| `--binary <path>` | (빌드 모드) | 사전 빌드 바이너리 경로 |
| `--build` | (기본) | 현재 디렉토리의 agent/ 소스로 Go 빌드 |
| `--install-dir` | `/opt/owlmon` | 설치 경로 |

---

## 검증 (설치 직후 1분 안에)

대상 호스트에서:

```bash
# 1. 서비스 active?
systemctl is-active owlmon-agent
# → active

# 2. 로그에 "호스트 스펙 전송 완료" 보이는지
journalctl -u owlmon-agent -n 20 --no-pager | grep "호스트 스펙"
# → 2026/05/19 ... 호스트 스펙 전송 완료: CPU=... RAM=... 디스크=...
```

OWLmon 서버에서:

```bash
ssh owlmon@192.168.0.30
docker exec owlmon-postgres psql -U owlmon -d owlmon -c \
  "SELECT host_name, cpu_cores, virtualization FROM agent_specs WHERE host_name='<TARGET_HOSTNAME>';"
```

→ 새 호스트 row 1개. 메트릭은 30~60초 안에 prometheus에도 등장.

---

## OS별 주의사항

### Debian 12 / Ubuntu 24.04
- `/var/log/syslog` **없음** (rsyslog 기본 미설치). `--journald` 옵션 필수.

### CentOS Stream 8 / RHEL 9 / Rocky / Alma / Oracle
- `/var/log/messages` 권한 `0600 root:root` → 일반 사용자 읽기 X. `--journald` 권장.
- SELinux enforcing 환경에서 systemd 거부 발생 시: `setenforce 0`로 임시 비활성화 후 정책 추가.

### Ubuntu 18.04 / Debian 10
- glibc 버전 낮음. 사전 빌드 바이너리(`--binary`) 사용 시 `CGO_ENABLED=0`으로 빌드된 정적 바이너리 권장 (위 방법 A 그대로).

### macOS / Windows
- 이 스크립트는 Linux 전용. macOS는 `make dev`로 개발 환경, Windows는 `scripts/install-agent-windows.ps1` 사용.

---

## 트러블슈팅

### 401 Unauthorized

```
로그 전송 실패 (N건): 서버 응답 오류: 401
```
→ `--agent-key` 값이 운영 서버 `.env`의 `OWLMON_AGENT_KEY` 와 다름.
   `/opt/owlmon/config.yaml`의 `agent_key:` 줄 수정 후 `systemctl restart owlmon-agent`.

### connection refused

```
스펙 전송 실패: dial tcp ...: connect: connection refused
```
→ 대상 호스트 → OWLmon 서버 8080/4317 도달 불가. 방화벽 / 망분리 확인.

### journald 로그 안 들어옴

→ `--journald` 옵션을 줬는지 확인. `systemd-journal` 그룹에 `owlmon-agent` 추가됐는지:
```bash
groups owlmon-agent
# owlmon-agent : owlmon-agent systemd-journal  ← 이렇게 보여야 함
```

### `/var/log/messages: permission denied`

→ journald 수집으로 전환하면 됨 (config의 `logs.tails` 비우고 `journald.enabled: true`).

---

## 다중 호스트 일괄 배포

호스트 5대 이상이면 간단한 for 루프:

```bash
KEY="<YOUR_AGENT_KEY>"
for H in host-a host-b host-c; do
  scp /tmp/owlmon-agent          root@$H:/tmp/
  scp scripts/install-agent-linux.sh root@$H:/tmp/
  ssh root@$H "bash /tmp/install-agent-linux.sh \
    --binary /tmp/owlmon-agent \
    --server http://192.168.0.30:8080 \
    --otlp   192.168.0.30:4317 \
    --agent-key '$KEY' \
    --journald"
done
```

50대 이상: Ansible playbook 도입 검토 (별도 로드맵).

---

## 제거

```bash
sudo bash uninstall-agent-linux.sh
```

또는 수동:
```bash
sudo systemctl stop owlmon-agent
sudo systemctl disable owlmon-agent
sudo rm -rf /opt/owlmon /var/lib/owlmon /etc/systemd/system/owlmon-agent.service
sudo userdel owlmon-agent
sudo systemctl daemon-reload
```

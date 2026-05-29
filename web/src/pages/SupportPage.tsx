import { useState } from 'react'
import {
  LifeBuoy, Terminal, ServerCog, HelpCircle, Wrench, Mail, Code,
  ChevronDown, Copy, CheckCircle2, AlertCircle, AlertTriangle,
} from 'lucide-react'
import { cn } from '../utils/cn'
import PageToolbar from '../components/PageToolbar'

/**
 * 지원 페이지 — 설치 가이드 + FAQ + 트러블슈팅 + 문의
 *
 * 학교/공공기관 IT 담당자가 자가 해결할 수 있는 자료 중심.
 * 망분리 환경 / 소규모 인력 환경에 맞춤.
 */

const SECTIONS = [
  { id: 'quickstart', icon: Terminal,   label: '빠른 시작' },
  { id: 'requirements', icon: ServerCog, label: '시스템 요구사항' },
  { id: 'faq',        icon: HelpCircle, label: '자주 묻는 질문' },
  { id: 'troubleshoot', icon: Wrench,   label: '문제 해결' },
  { id: 'contact',    icon: Mail,       label: '문의' },
] as const

export default function SupportPage() {
  const [active, setActive] = useState<string>('quickstart')

  return (
    <div className="space-y-6">
      <PageToolbar
        icon={LifeBuoy}
        title="지원"
        description="설치 가이드 · FAQ · 문제 해결 · 문의"
      />

      <div className="flex flex-col lg:flex-row gap-6">
        {/* Side Nav */}
        <nav className="lg:w-56 shrink-0">
          <ul className="space-y-1 sticky top-4">
            {SECTIONS.map(s => {
              const Icon = s.icon
              return (
                <li key={s.id}>
                  <button
                    onClick={() => {
                      setActive(s.id)
                      document.getElementById(s.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
                    }}
                    className={cn(
                      "w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm font-semibold transition-colors",
                      active === s.id
                        ? "bg-indigo-500/15 text-indigo-300"
                        : "text-slate-400 hover:bg-slate-800 hover:text-slate-200"
                    )}
                  >
                    <Icon size={14} />
                    {s.label}
                  </button>
                </li>
              )
            })}
          </ul>
        </nav>

        {/* Content */}
        <div className="flex-1 min-w-0 space-y-8">
          <Section id="quickstart" title="빠른 시작" icon={Terminal}>
            <p className="text-sm text-slate-400 mb-4 leading-relaxed">
              에이전트는 모니터링 대상 호스트에 설치합니다. 설치 후 자동으로 OWLmon 서버로 메트릭을 전송합니다.
            </p>

            <SubSection title="Linux 에이전트 설치">
              <CodeBlock
                code={`curl -sSL http://<OWLmon 서버 IP>/install-agent.sh | sudo bash`}
                note="3줄로 끝. 시스템 서비스로 등록되어 부팅 시 자동 시작합니다."
              />
              <p className="text-xs text-slate-500 mt-2">
                망분리 환경: <code className="text-slate-400 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">scripts/install-agent-linux.sh</code> 를 수동 복사 후 실행
              </p>
            </SubSection>

            <SubSection title="Windows 에이전트 설치">
              <CodeBlock
                code={`# PowerShell 관리자 권한으로 실행
iwr -useb http://<OWLmon 서버 IP>/install-agent.ps1 | iex`}
                note="Windows 서비스로 등록됩니다."
              />
            </SubSection>

            <SubSection title="설치 확인">
              <ol className="list-decimal list-inside text-sm text-slate-300 space-y-1.5 ml-1">
                <li>30초 이내 OWLmon 대시보드에 호스트 카드 자동 생성</li>
                <li>CPU / 메모리 / 디스크 사용률 실시간 표시</li>
                <li>안 보이면 → '문제 해결' 섹션 참고</li>
              </ol>
            </SubSection>
          </Section>

          <Section id="requirements" title="시스템 요구사항" icon={ServerCog}>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <ReqCard title="에이전트 (피감시 호스트)">
                <ReqRow label="OS" value="Linux (kernel 3.10+) · Windows 10/Server 2016+" />
                <ReqRow label="CPU" value="1 core 이상 (사용률 1% 미만)" />
                <ReqRow label="메모리" value="50MB 이상 (실제 사용 10~20MB)" />
                <ReqRow label="디스크" value="설치 50MB" />
                <ReqRow label="네트워크" value="OWLmon 서버로 outbound 4317/8080" />
              </ReqCard>

              <ReqCard title="서버 (OWLmon 호스팅)">
                <ReqRow label="OS" value="Linux (Ubuntu 20.04+ 권장)" />
                <ReqRow label="CPU" value="2 core 이상" />
                <ReqRow label="메모리" value="4GB 이상 (호스트 100대 기준 8GB 권장)" />
                <ReqRow label="디스크" value="20GB + (호스트당 1GB/월 추정)" />
                <ReqRow label="Docker" value="Docker Compose v2 필요" />
              </ReqCard>
            </div>

            <SubSection title="네트워크 포트">
              <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
                <table className="w-full text-sm">
                  <thead className="bg-slate-800/50">
                    <tr className="text-left text-[11px] font-bold text-slate-400 uppercase tracking-wide">
                      <th className="px-4 py-2.5">포트</th>
                      <th className="px-4 py-2.5">용도</th>
                      <th className="px-4 py-2.5">방향</th>
                    </tr>
                  </thead>
                  <tbody className="text-slate-300 [&_tr]:border-t [&_tr]:border-slate-800">
                    <tr><td className="px-4 py-2.5 font-mono">4317</td><td className="px-4 py-2.5">OTLP gRPC (메트릭 수집)</td><td className="px-4 py-2.5 text-slate-500">에이전트 → 서버</td></tr>
                    <tr><td className="px-4 py-2.5 font-mono">8080</td><td className="px-4 py-2.5">서버 API (로그 / 알림)</td><td className="px-4 py-2.5 text-slate-500">에이전트 → 서버</td></tr>
                    <tr><td className="px-4 py-2.5 font-mono">80 / 443</td><td className="px-4 py-2.5">웹 대시보드</td><td className="px-4 py-2.5 text-slate-500">사용자 → 서버</td></tr>
                    <tr><td className="px-4 py-2.5 font-mono">25 / 587</td><td className="px-4 py-2.5">SMTP (이메일 알림)</td><td className="px-4 py-2.5 text-slate-500">서버 → 메일</td></tr>
                  </tbody>
                </table>
              </div>
            </SubSection>

            <SubSection title="이메일 알림 (SMTP) 설정">
              <p className="text-sm text-slate-400 mb-2 leading-relaxed">
                알림 이메일 발송을 활성화하려면 서버 관리자가 <code className="bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">.env</code> 파일에 SMTP 정보를 설정해야 합니다.
              </p>
              <CodeBlock
                code={`# ~/owlmon/.env 파일에 추가
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-account@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=owlmon@school.go.kr

# 서버 재시작
docker compose up -d --force-recreate owlmon-server`}
                note="Gmail 앞 → 앱 비밀번호 발급 필요. 학교 자체 SMTP가 있으면 해당 값 입력."
              />
              <p className="text-xs text-slate-500 mt-2">
                설정 후 알림 수신자는 <strong>알림 설정</strong> 페이지에서 추가합니다.
              </p>
            </SubSection>

            <SubSection title="자동 암호화 백업 (KISA AES-256)">
              <p className="text-sm text-slate-400 mb-2 leading-relaxed">
                PostgreSQL DB(알림 히스토리, 룰, 도메인, 자산 등 모든 영속 데이터)는 매일 03:00 KST에 <strong>AES-256-CBC + PBKDF2(100000 iter)</strong>로 암호화되어 백업됩니다. 행안부 백업 의무 + KISA 암호화 알고리즘 권장 동시 충족.
              </p>
              <ul className="text-sm text-slate-300 space-y-1.5 ml-1 list-disc list-inside">
                <li>저장 경로: <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">~/owlmon/data/backups/</code></li>
                <li>파일명: <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">owlmon-YYYYMMDD-HHMMSS.sql.gz.enc</code></li>
                <li>보관 기간: 기본 30일 (<code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">BACKUP_RETENTION_DAYS</code> 환경변수)</li>
                <li>패스프레이즈: 첫 실행 시 자동 생성 → <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">/backups/.passphrase</code> (chmod 600)</li>
              </ul>

              <div className="mt-3 flex items-start gap-2 px-3 py-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-xs text-rose-300">
                <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                <span><strong>패스프레이즈 분실 시 백업 복원 불가</strong>. <code className="bg-slate-800 px-1 rounded">/backups/.passphrase</code> 파일을 USB 등 별도 안전한 곳에 1회 백업하세요.</span>
              </div>

              <h3 className="text-sm font-bold text-slate-300 mt-4 mb-1.5">수동 백업 (즉시)</h3>
              <CodeBlock
                code={`ssh owlmon@<서버IP> 'docker exec owlmon-backup sh -c "PASS=\\$(cat /backups/.passphrase); pg_dump --no-owner --no-acl | gzip -9 | openssl enc -aes-256-cbc -pbkdf2 -iter 100000 -salt -pass pass:\\$PASS -out /backups/owlmon-manual-\\$(date +%Y%m%d-%H%M%S).sql.gz.enc"'`}
              />

              <h3 className="text-sm font-bold text-slate-300 mt-4 mb-1.5">복원 (장애 시)</h3>
              <CodeBlock
                code={`# 1. 서버 컨테이너 정지
docker compose stop owlmon-server

# 2. 패스프레이즈 확인
PASS=$(sudo cat ~/owlmon/data/backups/.passphrase)

# 3. 복호화 + 압축 해제 + DB 적용
openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 -salt \\
  -pass "pass:$PASS" \\
  -in ~/owlmon/data/backups/owlmon-YYYYMMDD-HHMMSS.sql.gz.enc \\
  | gunzip \\
  | docker exec -i owlmon-postgres psql -U owlmon -d owlmon

# 4. 서버 컨테이너 재시작
docker compose start owlmon-server`}
                note="⚠️ 복원 전 현재 DB 별도 백업 권장. 패스프레이즈 분실 시 복원 불가."
              />

              <h3 className="text-sm font-bold text-slate-300 mt-4 mb-1.5">패스프레이즈 직접 지정</h3>
              <p className="text-xs text-slate-500 mb-1.5">
                기관 정책에 따라 직접 패스프레이즈 관리가 필요하면 <code className="bg-slate-800 px-1 rounded">.env</code>에 추가:
              </p>
              <CodeBlock
                code={`# ~/owlmon/.env에 추가
OWLMON_BACKUP_PASSPHRASE=학교에서_결정한_충분히_긴_문자열`}
              />
            </SubSection>
          </Section>

          <Section id="faq" title="자주 묻는 질문" icon={HelpCircle}>
            <FAQItem q="에이전트 설치 후 호스트가 대시보드에 안 보입니다.">
              <p className="mb-2">다음 순서로 확인하세요:</p>
              <ol className="list-decimal list-inside space-y-1 ml-1">
                <li>에이전트 서비스 상태: <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">systemctl status owlmon-agent</code></li>
                <li>방화벽 — 에이전트에서 OWLmon 서버 4317 포트 도달 가능한지: <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">nc -zv {`<서버IP>`} 4317</code></li>
                <li>에이전트 로그: <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">journalctl -u owlmon-agent -n 50</code></li>
                <li>30초 대기 후 새로고침 (메트릭은 30초 주기 전송)</li>
              </ol>
            </FAQItem>

            <FAQItem q="SSL 인증서 카드가 '확인 중'으로 멈춰 있습니다.">
              <p>서버 재시작 직후엔 SSL 캐시가 비어있어 즉시 표시되지 않습니다. 페이지 자동으로 체크가 트리거되며 보통 5~10초 내 결과가 표시됩니다. '즉시 체크' 버튼으로 수동 실행할 수도 있습니다.</p>
            </FAQItem>

            <FAQItem q="알림이 안 옵니다.">
              <p className="mb-2">다음을 확인하세요:</p>
              <ol className="list-decimal list-inside space-y-1 ml-1">
                <li>알림 설정 페이지에서 SMTP 정상 동작 (테스트 버튼)</li>
                <li>임계치 설정 — CPU/메모리/디스크 기본값(90/95/90%) 조정 필요할 수 있음</li>
                <li>점검 모드 활성화된 호스트는 알림 미발송</li>
                <li>3회 연속 임계치 초과 후 발송 (디바운싱)</li>
              </ol>
            </FAQItem>

            <FAQItem q="디스크 예측이 '9999일 후 부족'처럼 비현실적으로 나옵니다.">
              <p>디스크 사용량 증가율이 매우 낮을 때 선형 회귀가 무의미한 큰 값을 산출할 수 있습니다. OWLmon은 90일 초과 예측은 숨기고 30일/7일 단계로 경고를 표시합니다.</p>
            </FAQItem>

            <FAQItem q="망분리 환경에서 사용 가능한가요?">
              <p>네. 에이전트와 서버가 사내 네트워크 안에서만 통신하면 됩니다. 외부 인터넷 의존성 없음 (선택적으로 SMTP 메일 서버만). 에이전트 업데이트도 OWLmon 서버 경유 자가 갱신 지원.</p>
            </FAQItem>

            <FAQItem q="SNMP 장비 (스위치/라우터)도 모니터링 가능한가요?">
              <p>가능합니다. SNMP v2c 지원 장비를 등록하면 트래픽/CPU/메모리 폴링합니다. 망분리 환경에선 에이전트가 SNMP 프록시 역할도 합니다.</p>
            </FAQItem>
          </Section>

          <Section id="troubleshoot" title="문제 해결" icon={Wrench}>
            <TroubleCard
              issue="에이전트 CPU 사용률이 비정상적으로 높음 (10%+)"
              cause="대부분 다른 프로세스(ML/배치 작업 등)와 시간이 겹친 우연. 에이전트 자체는 1% 미만."
              fix={
                <>
                  <p><code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">top</code>으로 실제 CPU 점유 프로세스 확인. owlmon-agent가 아니라면 해당 프로세스 조사.</p>
                </>
              }
            />

            <TroubleCard
              issue="SSL 인증서 만료 임박했는데 알림이 안 옴"
              cause="알림 발송 주기는 도메인당 1회 (디바운싱)."
              fix={
                <>
                  <p>알림 히스토리 페이지에서 해당 도메인 발송 여부 확인. 30일/7일 두 번 발송되며, 이미 보낸 단계는 재발송하지 않음.</p>
                </>
              }
            />

            <TroubleCard
              issue="로그 수집이 멈춤"
              cause="에이전트와 서버 간 HTTP 8080 차단 또는 디스크 가득참."
              fix={
                <>
                  <p>1. 서버 디스크 용량 확인 (logs 테이블 GB 단위 증가 가능)</p>
                  <p>2. 에이전트 → 서버 8080 포트 도달성 (<code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">curl -v http://서버IP:8080/health</code>)</p>
                  <p>3. WAL 파일 디스크 확인 (<code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">/opt/owlmon/logs-wal.json</code>)</p>
                </>
              }
            />

            <TroubleCard
              issue="대시보드가 로딩만 되고 데이터 안 보임"
              cause="JWT 토큰 만료 또는 백엔드 API 오류."
              fix={
                <>
                  <p>1. 로그아웃 후 재로그인</p>
                  <p>2. 브라우저 개발자도구 (F12) → Network 탭에서 401/500 응답 확인</p>
                  <p>3. 서버 컨테이너 로그: <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">docker logs owlmon-server --tail 100</code></p>
                </>
              }
            />

            <TroubleCard
              issue="DB 카드에 '쿼리 분석 기능 OFF'가 표시됨"
              cause="대상 PostgreSQL에 pg_stat_statements 확장이 설치되지 않음. 기본 메트릭(연결 수/캐시/DB 크기)은 정상 동작하지만 슬로우 쿼리 TOP N은 수집 불가."
              fix={
                <>
                  <p>대상 PostgreSQL 서버에서 다음 작업 (관리자 권한):</p>
                  <p>1. <code className="text-slate-300 bg-slate-800 px-1.5 py-0.5 rounded text-[11px]">postgresql.conf</code>에 다음 줄 추가/수정:</p>
                  <pre className="bg-slate-950 border border-slate-800 rounded-lg p-2.5 text-[11px] text-slate-300 overflow-x-auto font-mono ml-3">shared_preload_libraries = 'pg_stat_statements'</pre>
                  <p>2. PostgreSQL 재시작</p>
                  <p>3. 대상 DB에 접속 후:</p>
                  <pre className="bg-slate-950 border border-slate-800 rounded-lg p-2.5 text-[11px] text-slate-300 overflow-x-auto font-mono ml-3">CREATE EXTENSION pg_stat_statements;</pre>
                  <p className="text-slate-500 text-[10px] mt-1 flex items-start gap-1"><AlertTriangle size={11} className="shrink-0 mt-0.5" /> AWS RDS / GCP Cloud SQL은 파라미터 그룹에서 켜야 합니다. 자체 운영 DB만 위 절차 사용.</p>
                </>
              }
            />
          </Section>

          <Section id="contact" title="문의" icon={Mail}>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <a
                href="mailto:skanehfud279@willkomo.com"
                className="flex items-center gap-4 p-5 rounded-2xl bg-slate-900 border border-slate-800 hover:border-indigo-500/50 hover:bg-slate-800/50 transition-colors group"
              >
                <div className="p-3 rounded-xl bg-indigo-500/15 text-indigo-300 group-hover:bg-indigo-500/25 transition-colors">
                  <Mail size={20} />
                </div>
                <div className="min-w-0">
                  <div className="text-sm font-bold text-slate-200">이메일 문의</div>
                  <div className="text-xs text-slate-400 mt-0.5 truncate">skanehfud279@willkomo.com</div>
                </div>
              </a>

              <a
                href="https://github.com/SeongJae999/owlmon/issues"
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-4 p-5 rounded-2xl bg-slate-900 border border-slate-800 hover:border-indigo-500/50 hover:bg-slate-800/50 transition-colors group"
              >
                <div className="p-3 rounded-xl bg-slate-800 text-slate-300 group-hover:bg-slate-700 transition-colors">
                  <Code size={20} />
                </div>
                <div className="min-w-0">
                  <div className="text-sm font-bold text-slate-200">GitHub Issues</div>
                  <div className="text-xs text-slate-400 mt-0.5">버그 신고 / 기능 제안</div>
                </div>
              </a>
            </div>

            <p className="text-xs text-slate-500 mt-4">
              영업/도입 문의는 이메일을 권장합니다. 영업일 기준 1~2일 내 회신.
            </p>
          </Section>
        </div>
      </div>
    </div>
  )
}

// ─── 하위 컴포넌트들 ─────────────────────────────

function Section({ id, title, icon: Icon, children }: { id: string; title: string; icon: any; children: React.ReactNode }) {
  return (
    <section id={id} className="bg-slate-900 rounded-2xl border border-slate-800 p-5 sm:p-6 space-y-4 scroll-mt-6">
      <div className="flex items-center gap-3 pb-3 border-b border-slate-800">
        <div className="p-2 rounded-lg bg-indigo-500/15 text-indigo-300">
          <Icon size={18} />
        </div>
        <h2 className="text-base font-bold text-slate-100">{title}</h2>
      </div>
      {children}
    </section>
  )
}

function SubSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mt-4">
      <h3 className="text-sm font-bold text-slate-300 mb-2">{title}</h3>
      {children}
    </div>
  )
}

function CodeBlock({ code, note }: { code: string; note?: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="relative">
      <pre className="bg-slate-950 border border-slate-800 rounded-xl p-4 text-xs text-slate-300 overflow-x-auto whitespace-pre-wrap break-all font-mono">
        {code}
      </pre>
      <button
        onClick={() => {
          navigator.clipboard.writeText(code).then(() => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
          })
        }}
        className="absolute top-2 right-2 p-1.5 rounded-md bg-slate-800 text-slate-400 hover:text-slate-200 hover:bg-slate-700 transition-colors"
        title="복사"
      >
        {copied ? <CheckCircle2 size={12} className="text-emerald-400" /> : <Copy size={12} />}
      </button>
      {note && <p className="text-[11px] text-slate-500 mt-1.5 ml-1">{note}</p>}
    </div>
  )
}

function ReqCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-4">
      <h4 className="text-sm font-bold text-slate-200 mb-3 pb-2 border-b border-slate-800">{title}</h4>
      <div className="space-y-2">{children}</div>
    </div>
  )
}

function ReqRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3 text-xs">
      <span className="text-slate-500 w-16 shrink-0">{label}</span>
      <span className="text-slate-300 font-medium">{value}</span>
    </div>
  )
}

function FAQItem({ q, children }: { q: string; children: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="border-b border-slate-800 last:border-b-0">
      <button
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between gap-3 py-3 text-left hover:text-indigo-300 transition-colors"
      >
        <span className="text-sm font-semibold text-slate-200">{q}</span>
        <ChevronDown size={16} className={cn("shrink-0 text-slate-500 transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <div className="pb-3 text-xs text-slate-400 leading-relaxed space-y-1.5">
          {children}
        </div>
      )}
    </div>
  )
}

function TroubleCard({ issue, cause, fix }: { issue: string; cause: string; fix: React.ReactNode }) {
  return (
    <div className="bg-slate-950/50 border border-slate-800 rounded-xl p-4 space-y-2">
      <div className="flex items-start gap-2">
        <AlertCircle size={14} className="shrink-0 text-amber-400 mt-0.5" />
        <span className="text-sm font-bold text-slate-200">{issue}</span>
      </div>
      <div className="ml-6 space-y-1.5 text-xs">
        <div>
          <span className="text-slate-500 font-bold">원인: </span>
          <span className="text-slate-400">{cause}</span>
        </div>
        <div className="text-slate-300 space-y-1">
          <span className="text-slate-500 font-bold">해결: </span>
          <div className="ml-1">{fix}</div>
        </div>
      </div>
    </div>
  )
}

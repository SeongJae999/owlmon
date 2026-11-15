import { Suspense, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useLocation, Outlet } from 'react-router-dom'
import { logout, getMe, heartbeat, FORBIDDEN_EVENT } from '../api/auth'
import { useAlertStatus } from '../hooks/useMonitoring'
import {
  LayoutGrid,
  FileText,
  Settings,
  FileBarChart,
  LogOut,
  KeyRound,
  Bell,
  Sun,
  Moon,
  MonitorPlay,
  ArrowUpRight,
  Boxes,
  Network,
  Globe,
  Database,
  Rows3,
  ChevronDown,
} from 'lucide-react'
import { cn } from '../utils/cn'
import EmailStatusBanner from '../components/EmailStatusBanner'
import ChatWidget from '../components/ChatWidget'
import PasswordModal from '../components/PasswordModal'

// 메뉴는 최소로 — 장비·알림 화면은 지금 상태의 행·카드·종 아이콘에서, 관리 화면은 설정 첫 줄 바로가기에서 들어감. 경로(path)는 그대로 (북마크 호환)
type NavItem = { id: string; path: string; icon: any; label: string; short?: string; match?: string[]; leaves?: boolean; children?: NavItem[] }
const NAV_SECTIONS: { adminOnly?: boolean; items: NavItem[] }[] = [
  { items: [
    // 타고 들어간 화면에서도 지금 상태가 켜져 있어야 한 번에 돌아옴
    { id: 'overview', path: '/', icon: LayoutGrid, label: '지금 상태', match: ['/detail', '/trends', '/history'] },
    // 사이드바 밖 전체 화면으로 나가므로 표시 — 갑자기 메뉴가 사라져 당황하지 않게
    { id: 'wall', path: '/wall', icon: MonitorPlay, label: '상황판', leaves: true },
    // 장비 종류별 화면은 EMS의 기본 — 메뉴 한 칸에 펼쳐 넣어 폭은 아끼고 한 번에 고를 수 있게 (서버는 지금 상태의 장비 표)
    { id: 'devices', path: '', icon: Boxes, label: '장비', children: [
      { id: 'snmp', path: '/snmp', icon: Network, label: '네트워크 장비' },
      { id: 'website', path: '/website', icon: Globe, label: '웹사이트' },
      { id: 'dpm', path: '/dpm', icon: Database, label: '데이터베이스' },
      { id: 'racks', path: '/racks', icon: Rows3, label: '랙 실장도' },
    ] },
    { id: 'logs', path: '/logs', icon: FileText, label: '로그', match: ['/insights'] },
    { id: 'report', path: '/report', icon: FileBarChart, label: '월간 보고서', short: '보고서' },
  ] },
  { adminOnly: true, items: [
    { id: 'settings', path: '/settings', icon: Settings, label: '설정', match: ['/rules', '/rules/stats', '/assets', '/topology', '/users', '/audit'] },
  ] },
]

export default function DashboardLayout() {
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [theme, setTheme] = useState(() => document.documentElement.dataset.theme === 'light' ? 'light' : 'dark')
  const toggleTheme = () => {
    const next = theme === 'light' ? 'dark' : 'light'
    document.documentElement.dataset.theme = next
    try { localStorage.setItem('owlmon-theme', next) } catch { /* 저장 불가 환경이면 이번 세션만 적용 */ }
    setTheme(next)
  }
  const [passwordOpen, setPasswordOpen] = useState(false)
  const [forbidden, setForbidden] = useState<string | null>(null)
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 5 * 60_000 })
  const isViewer = me?.role === 'viewer'

  // 무조작 판단은 서버가 함 (탭이 여러 개여도 계정 세션 기준). 끊기면 401 처리에서 로그인 화면으로 이동
  useEffect(() => {
    let active = false
    const touch = () => { active = true }
    const events = ['mousemove', 'keydown', 'click', 'scroll', 'touchstart'] as const
    events.forEach(e => window.addEventListener(e, touch, { passive: true }))
    const timer = setInterval(() => {
      heartbeat(active).catch(() => {}) // 네트워크·서버 일시 오류로는 로그아웃하지 않음
      active = false
    }, 60_000)
    return () => {
      events.forEach(e => window.removeEventListener(e, touch))
      clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    const onForbidden = (e: Event) => setForbidden((e as CustomEvent<string>).detail)
    window.addEventListener(FORBIDDEN_EVENT, onForbidden)
    return () => window.removeEventListener(FORBIDDEN_EVENT, onForbidden)
  }, [])
  useEffect(() => {
    if (!forbidden) return
    const t = setTimeout(() => setForbidden(null), 4000)
    return () => clearTimeout(t)
  }, [forbidden])
  const location = useLocation()
  const { data: activeAlerts = [] } = useAlertStatus()

  const unackedAlertCount = activeAlerts.filter(a => !a.acked && !a.in_maintenance).length
  // 다른 탭·창을 보고 있어도 새 문제를 알아채도록 탭 제목에 미확인 건수 표시
  useEffect(() => {
    document.title = unackedAlertCount > 0 ? `(${unackedAlertCount}) OWLmon` : 'OWLmon'
    return () => { document.title = 'OWLmon' } // 상황판 등 레이아웃 밖으로 나가면 건수 제거
  }, [unackedAlertCount])

  const onLogout = () => {
    logout()
  }

  const isActive = (it: NavItem): boolean => location.pathname === it.path || !!it.match?.includes(location.pathname) || !!it.children?.some(isActive)
  // 펼침 메뉴는 메뉴 줄(가로 스크롤 영역) 밖에 고정 위치로 띄움 — 안에 두면 스크롤 영역에 잘려 안 보임
  // 연 화면 주소를 같이 기억 — 키보드·뒤로 가기로 다른 화면에 가면 저절로 안 보이게
  const [menuState, setOpenMenu] = useState<{ id: string; left: number; top: number; path: string } | null>(null)
  const openMenu = menuState?.path === location.pathname ? menuState : null
  const menuRef = useRef<HTMLDivElement>(null)
  const menuButton = useRef<HTMLButtonElement>(null)
  // 열면 첫 항목으로 포커스 — 키보드로 연 사람이 메뉴 뒤 탭들을 거치지 않고 바로 고르게
  useEffect(() => { if (openMenu) menuRef.current?.querySelector('a')?.focus() }, [openMenu?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!openMenu) return
    const onDown = (e: MouseEvent) => { if (!menuRef.current?.contains(e.target as Node) && !(e.target as Element).closest?.('[data-menu-button]')) setOpenMenu(null) }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') { setOpenMenu(null); menuButton.current?.focus() } }
    const close = () => setOpenMenu(null)
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    window.addEventListener('resize', close)
    return () => { window.removeEventListener('mousedown', onDown); window.removeEventListener('keydown', onKey); window.removeEventListener('resize', close) }
  }, [openMenu])
  const items = NAV_SECTIONS.filter(s => !(s.adminOnly && isViewer))

  return (
    <div className="flex flex-col h-screen bg-base-950 font-sans text-base-100 overflow-hidden">
      {/* 메뉴가 5개뿐이라 왼쪽 사이드바 대신 위쪽 한 줄에 — 본문(장비 표·구성도)이 화면 폭을 다 씀 */}
      {/* 좁은 화면(lg 미만)에선 메뉴를 로고 아래 둘째 줄로 내려 5개가 한눈에 보이게 */}
      <header className="min-h-14 bg-base-900 border-b border-base-800 flex flex-wrap items-center gap-x-3 sm:gap-x-6 px-3 sm:px-6 shrink-0 z-40">
        <Link to="/" className="h-14 flex items-center gap-2 shrink-0" aria-label="OWLmon 지금 상태">
          <img src="/favicon.svg" alt="" className="w-8 h-8 rounded-lg" />
          <span className="hidden md:block font-bold text-base-50">OWLmon</span>
        </Link>

        {/* 태블릿 세로(768~1023px)에선 한 줄에 다 안 들어가 '설정'이 가려지므로 lg 미만은 둘째 줄로 */}
        <nav aria-label="주 메뉴" onScroll={() => setOpenMenu(null)} className="order-last w-[calc(100%+1.5rem)] h-11 -mx-3 px-1 sm:w-full sm:mx-0 sm:px-0 lg:order-none lg:w-auto lg:h-14 lg:flex-1 min-w-0 flex items-stretch gap-1 overflow-x-auto scrollbar-hide border-t border-base-800 lg:border-t-0">
          {items.map((section, si) => (
            <div key={section.adminOnly ? 'admin' : 'main'} className={cn('flex items-stretch gap-1', si > 0 && 'ml-1 pl-1 sm:ml-2 sm:pl-3 border-l border-base-800 my-3')}>
              {section.items.map(item => {
                const active = isActive(item)
                const tabCls = cn(
                  // 메뉴 줄이 가로 스크롤 영역이라 바깥 포커스 테두리는 잘림 — 안쪽에 그림
                  'relative flex items-center gap-1 sm:gap-2 px-1.5 sm:px-3 text-sm font-semibold whitespace-nowrap border-b-2 transition-colors focus-visible:outline-offset-[-2px]',
                  si > 0 && '-my-3',
                  active ? 'border-brand-500 text-base-50' : 'border-transparent text-base-400 hover:text-base-100',
                )
                if (item.children) {
                  const open = openMenu?.id === item.id
                  return (
                    <div key={item.id} className="flex items-stretch">
                      <button
                        ref={menuButton} type="button" data-menu-button aria-expanded={open} aria-controls={`menu-${item.id}`}
                        onClick={e => {
                          const r = e.currentTarget.getBoundingClientRect()
                          setOpenMenu(open ? null : { id: item.id, left: Math.max(8, Math.min(r.left, window.innerWidth - 200)), top: r.bottom, path: location.pathname })
                        }}
                        className={tabCls}
                      >
                        <item.icon size={17} className={cn('hidden sm:block shrink-0', active ? 'text-brand-400' : 'text-base-500')} />
                        <span>{item.label}</span>
                        <ChevronDown size={14} className={cn('-ml-0.5 text-base-500 transition-transform', open && 'rotate-180')} />
                      </button>
                      {/* 버튼 바로 다음에 둬 Tab 순서·읽는 순서가 이어지게. 화면 위치는 고정 좌표라 가로 스크롤 영역에 잘리지 않음 */}
                      {open && (
                        <div ref={menuRef} id={`menu-${item.id}`} style={{ left: openMenu.left, top: openMenu.top + 4 }}
                          className="fixed z-50 w-48 py-1 rounded-xl border border-base-700 bg-base-900 shadow-xl">
                          {item.children.map(c => {
                            const on = isActive(c)
                            return (
                              <Link key={c.id} to={c.path} aria-current={on ? 'page' : undefined} onClick={() => setOpenMenu(null)}
                                className={cn('flex items-center gap-2.5 px-3 py-2 text-sm focus-visible:outline-offset-[-2px]', on ? 'text-base-50 font-semibold bg-base-800' : 'text-base-300 hover:bg-base-800 hover:text-base-100')}>
                                <c.icon size={15} className={on ? 'text-brand-400' : 'text-base-500'} /> {c.label}
                              </Link>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  )
                }
                return (
                  <Link
                    key={item.id}
                    to={item.path}
                    aria-current={active ? 'page' : undefined}
                    className={tabCls}
                  >
                    <item.icon size={17} className={cn('hidden sm:block shrink-0', active ? 'text-brand-400' : 'text-base-500')} />
                    {/* 휴대폰 폭에선 짧은 이름 — 메뉴 6개가 한 줄에 들어가게 */}
                    <span className={cn(item.short && 'hidden sm:inline')}>{item.label}</span>
                    {item.short && <span className="sm:hidden">{item.short}</span>}
                    {item.leaves && <ArrowUpRight size={13} aria-label="전체 화면으로 열림" className="-ml-1 text-base-500" />}
                    {item.id === 'overview' && unackedAlertCount > 0 && (
                      <span aria-label={`처리할 문제 ${unackedAlertCount}건`} className="flex h-5 min-w-[20px] px-1 items-center justify-center rounded-full bg-rose-500 text-white text-xs font-bold tabular-nums">
                        {unackedAlertCount}
                      </span>
                    )}
                  </Link>
                )
              })}
            </div>
          ))}
        </nav>

          <div className="ml-auto lg:ml-0 flex items-center gap-3 shrink-0">
            <div className="flex items-center gap-1 p-1 bg-base-800 rounded-lg border border-base-700">
              <button
                type="button"
                onClick={toggleTheme}
                className="p-2 text-base-400 hover:text-brand-400 hover:bg-base-700 rounded-md transition-colors"
                title={theme === 'light' ? '어두운 화면으로' : '밝은 화면으로'}
                aria-label={theme === 'light' ? '어두운 화면으로 전환' : '밝은 화면으로 전환'}
              >
                {theme === 'light' ? <Moon size={18} /> : <Sun size={18} />}
              </button>
              {/* 벨 클릭 → 알림 이력 (장식 버튼이라 데모 중 무반응으로 보이던 것 수정) */}
              <Link to="/history" className="p-2 text-base-400 hover:text-brand-400 hover:bg-base-700 rounded-md transition-colors relative block" title="지난 알림" aria-label="지난 알림 보기">
                <Bell size={18} />
                {unackedAlertCount > 0 && (
                  <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-rose-500 rounded-full border border-base-900" />
                )}
              </Link>
              <div className="w-px h-5 bg-base-700 mx-0.5" />
              <div className="relative">
                <button
                  type="button"
                  onClick={() => setUserMenuOpen(o => !o)}
                  aria-expanded={userMenuOpen}
                  aria-label="내 계정 메뉴"
                  className="flex items-center gap-2 pl-1.5 pr-2 py-1 hover:bg-base-700 rounded-md transition-colors group"
                >
                  <div className="w-7 h-7 bg-base-700 rounded-md flex items-center justify-center text-base-300 text-xs font-semibold group-hover:bg-brand-500/20 group-hover:text-brand-400 transition-colors">
                    {me?.username?.[0]?.toUpperCase() ?? '·'}
                  </div>
                  <div className="hidden lg:block text-left">
                    <p className="text-xs font-semibold text-base-200 leading-none">{me?.username ?? ''}</p>
                    <p className="text-xs font-medium text-base-500 leading-none mt-0.5">{isViewer ? '조회 전용' : '관리자'}</p>
                  </div>
                </button>
                {userMenuOpen && (
                  <div className="absolute right-0 mt-2 w-48 rounded-xl border border-base-700 bg-base-900 shadow-xl py-1 z-50" onMouseLeave={() => setUserMenuOpen(false)}>
                    <p className="px-3 py-2 text-xs text-base-500 lg:hidden">{me?.username} · {isViewer ? '조회 전용' : '관리자'}</p>
                    {me?.multi_user && (
                      <button type="button" onClick={() => { setUserMenuOpen(false); setPasswordOpen(true) }} className="w-full flex items-center gap-2 px-3 py-2 text-sm text-base-200 hover:bg-base-800">
                        <KeyRound size={15} /> 비밀번호 변경
                      </button>
                    )}
                    <button type="button" onClick={onLogout} className="w-full flex items-center gap-2 px-3 py-2 text-sm text-base-200 hover:bg-base-800">
                      <LogOut size={15} /> 로그아웃
                    </button>
                  </div>
                )}
              </div>
            </div>
          </div>
        </header>

        {/* Scrollable Area */}
        <main className="flex-1 overflow-y-auto p-3 pb-20 sm:p-6 lg:p-8 scroll-smooth">
          <div className="w-full">
            <EmailStatusBanner />
            <Suspense fallback={<div className="py-12 text-center text-sm text-base-500">불러오는 중...</div>}>
              <Outlet />
            </Suspense>
          </div>

          {/* Global Footer in Main Area */}
          <footer className="w-full mt-12 pt-6 border-t border-base-800 flex flex-col sm:flex-row items-center justify-between gap-3 pb-8">
            <div className="flex items-center gap-2 text-base-500">
              <img src="/favicon.svg" alt="OWLmon" className="w-5 h-5 rounded-md" />
              <span className="text-xs font-medium">OWLmon v1.2</span>
            </div>
            <div className="flex gap-6">
              <Link to="/support" className="text-xs font-medium text-base-500 hover:text-brand-400 transition-colors">지원</Link>
              <a href="https://github.com/SeongJae999/owlmon" target="_blank" rel="noopener noreferrer" className="text-xs font-medium text-base-500 hover:text-brand-400 transition-colors">GitHub</a>
            </div>
          </footer>
        </main>
      <ChatWidget />
      {passwordOpen && <PasswordModal onClose={() => setPasswordOpen(false)} />}
      {forbidden && (
        <div role="alert" className="fixed z-[70] top-28 lg:top-20 left-1/2 -translate-x-1/2 px-4 py-2.5 rounded-xl border border-amber-500/40 bg-base-900 text-amber-300 text-sm shadow-xl">
          {forbidden}
        </div>
      )}
    </div>
  )
}

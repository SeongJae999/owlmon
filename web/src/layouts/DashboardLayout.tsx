import { Suspense, useState } from 'react'
import { Link, useLocation, Outlet } from 'react-router-dom'
import { logout } from '../api/auth'
import { useAlertStatus } from '../hooks/useMonitoring'
import {
  LayoutGrid,
  Network,
  ShieldCheck,
  Globe,
  Database,
  FileText,
  History,
  Settings,
  FileBarChart,
  Server,
  LogOut,
  ChevronLeft,
  ChevronRight,
  Menu,
  Bell,
  ListChecks,
  TrendingUp,
  Sparkles,
} from 'lucide-react'
import { cn } from '../utils/cn'
import EmailStatusBanner from '../components/EmailStatusBanner'
import ChatWidget from '../components/ChatWidget'

// 기능 나열 대신 "문제 확인 → 장비 → 분석 → 설정" 흐름 순서. 경로(path)는 기존 그대로 (북마크 호환)
const NAV_SECTIONS: { title?: string; collapsible?: boolean; items: { id: string; path: string; icon: any; label: string; match?: string[] }[] }[] = [
  { items: [
    { id: 'overview', path: '/', icon: LayoutGrid, label: '지금 상태', match: ['/detail'] },
  ] },
  { title: '문제·알림', items: [
    { id: 'alert-history', path: '/history', icon: History, label: '지난 알림' },
    { id: 'insights', path: '/insights', icon: Sparkles, label: 'AI 분석 결과' },
  ] },
  { title: '장비별 보기', items: [
    { id: 'snmp', path: '/snmp', icon: Network, label: '네트워크 장비' },
    { id: 'website', path: '/website', icon: Globe, label: '웹사이트' },
    { id: 'dpm', path: '/dpm', icon: Database, label: '데이터베이스' },
  ] },
  { title: '분석 도구', items: [
    { id: 'trends', path: '/trends', icon: TrendingUp, label: '사용량 추이' },
    { id: 'logs', path: '/logs', icon: FileText, label: '로그 검색' },
    { id: 'report', path: '/report', icon: FileBarChart, label: '월간 보고서' },
  ] },
  { title: '설정', collapsible: true, items: [
    { id: 'alert-settings', path: '/settings', icon: Settings, label: '알림 받는 사람·기준' },
    { id: 'rules', path: '/rules', icon: ListChecks, label: '로그 감지 규칙', match: ['/rules/stats'] },
    { id: 'assets', path: '/assets', icon: Server, label: '장비 목록' },
    { id: 'audit', path: '/audit', icon: ShieldCheck, label: '사용 기록' },
  ] },
]

export default function DashboardLayout() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const location = useLocation()
  const { data: activeAlerts = [] } = useAlertStatus()

  const unackedAlertCount = activeAlerts.filter(a => !a.acked && !a.in_maintenance).length

  const onLogout = () => {
    logout()
  }

  return (
    <div className="flex h-screen bg-slate-950 font-sans text-slate-100 overflow-hidden">
      {/* Mobile Overlay */}
      {mobileMenuOpen && (
        <div 
          className="fixed inset-0 bg-slate-900/40 backdrop-blur-sm z-40 lg:hidden transition-all duration-300" 
          onClick={() => setMobileMenuOpen(false)} 
        />
      )}

      {/* Sidebar */}
      <aside 
        className={cn(
          "fixed inset-y-0 left-0 z-50 bg-slate-900 text-slate-300 transition-all duration-300 ease-in-out transform lg:translate-x-0 lg:static flex flex-col shadow-2xl",
          sidebarCollapsed ? "w-20" : "w-72",
          mobileMenuOpen ? "translate-x-0" : "-translate-x-full"
        )}
      >
        {/* Brand */}
        <div className="h-16 flex items-center px-5 gap-3 border-b border-slate-800/50">
          <img src="/favicon.svg" alt="OWLmon" title="OWLmon" className="w-9 h-9 shrink-0 rounded-xl" />
          {!sidebarCollapsed && (
            <div className="overflow-hidden whitespace-nowrap">
              <p className="font-bold text-white text-lg">OWLmon</p>
              <p className="text-xs text-slate-500 font-medium">통합 모니터링</p>
            </div>
          )}
        </div>

        {/* Nav */}
        <nav className="flex-1 overflow-y-auto py-6 space-y-6 scrollbar-hide">
          {NAV_SECTIONS.map(section => {
            const isActive = (it: { path: string; match?: string[] }) =>
              location.pathname === it.path || !!it.match?.includes(location.pathname)
            // 설정은 평소 접어 둠 — 설정 화면에 있거나 사이드바가 아이콘 모드면 펼침
            const open = !section.collapsible || settingsOpen || sidebarCollapsed || section.items.some(isActive)
            return (
              <div key={section.title ?? 'home'} className="px-3">
                {section.title && !sidebarCollapsed && (
                  section.collapsible ? (
                    <button
                      type="button"
                      onClick={() => setSettingsOpen(o => !o)}
                      aria-expanded={open}
                      className="w-full flex items-center justify-between text-xs font-semibold text-slate-500 hover:text-slate-300 px-3 mb-3"
                    >
                      {section.title}
                      <ChevronRight size={14} className={cn('transition-transform', open && 'rotate-90')} />
                    </button>
                  ) : (
                    <p className="text-xs font-semibold text-slate-500 px-3 mb-3">{section.title}</p>
                  )
                )}
                {open && (
                  <div className="space-y-1.5">
                    {section.items.map(item => {
                      const active = isActive(item)
                      return (
                        <Link
                          key={item.id}
                          to={item.path}
                          onClick={() => setMobileMenuOpen(false)}
                          title={sidebarCollapsed ? item.label : undefined}
                          className={cn(
                            "flex items-center gap-3.5 px-4 py-3 rounded-xl transition-all duration-200 group relative font-semibold",
                            active
                              ? "bg-indigo-600 text-white shadow-lg shadow-indigo-500/20"
                              : "hover:bg-slate-800/50 hover:text-white"
                          )}
                        >
                          <item.icon size={20} className={cn("shrink-0 transition-transform duration-200 group-hover:scale-110", active ? "text-white" : "text-slate-500 group-hover:text-slate-300")} />
                          {!sidebarCollapsed && <span className="text-[13px]">{item.label}</span>}
                          {item.id === 'overview' && unackedAlertCount > 0 && (
                            <span
                              aria-label={`처리할 문제 ${unackedAlertCount}건`}
                              className={cn(
                                "absolute right-3 top-1/2 -translate-y-1/2 flex h-5 min-w-[20px] px-1 items-center justify-center rounded-full text-[10px] font-bold shadow-sm",
                                active ? "bg-white text-indigo-600" : "bg-rose-500 text-white"
                              )}
                            >
                              {unackedAlertCount}
                            </span>
                          )}
                        </Link>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}
        </nav>

        {/* Footer */}
        <div className="p-4 border-t border-slate-800/50 bg-slate-900/50 space-y-1">
          <button 
            onClick={onLogout}
            className="w-full flex items-center gap-3.5 px-4 py-3 rounded-xl text-slate-500 hover:bg-rose-500/10 hover:text-rose-400 transition-all duration-200 font-semibold"
          >
            <LogOut size={20} className="shrink-0" />
            {!sidebarCollapsed && <span className="text-[13px]">시스템 로그아웃</span>}
          </button>
          <button 
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            className="hidden lg:flex w-full items-center gap-3.5 px-4 py-3 rounded-xl text-slate-500 hover:bg-slate-800 hover:text-slate-300 transition-all duration-200 font-semibold"
          >
            {sidebarCollapsed ? <ChevronRight size={20} /> : <ChevronLeft size={20} />}
            {!sidebarCollapsed && <span className="text-[13px]">사이드바 축소</span>}
          </button>
        </div>
      </aside>

      {/* Main Content */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden relative">
        {/* Header */}
        <header className="h-16 bg-slate-900 border-b border-slate-800 flex items-center justify-between px-3 sm:px-6 shrink-0 z-40 sticky top-0">
          <div className="flex items-center gap-4">
            <button
              className="lg:hidden p-2 text-slate-400 hover:bg-slate-800 rounded-lg transition-colors"
              onClick={() => setMobileMenuOpen(true)}
              aria-label="메뉴 열기"
            >
              <Menu size={20} />
            </button>
            {/* 전역 검색 — 추후 구현 예정 (UI placeholder 제거됨) */}
          </div>

          <div className="flex items-center gap-3">
            <div className="flex items-center gap-1 p-1 bg-slate-800 rounded-lg border border-slate-700">
              {/* 벨 클릭 → 알림 이력 (장식 버튼이라 데모 중 무반응으로 보이던 것 수정) */}
              <Link to="/history" className="p-2 text-slate-400 hover:text-indigo-400 hover:bg-slate-700 rounded-md transition-colors relative block" title="알림" aria-label="알림 이력 보기">
                <Bell size={18} />
                {unackedAlertCount > 0 && (
                  <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-rose-500 rounded-full border border-slate-900" />
                )}
              </Link>
              <div className="w-px h-5 bg-slate-700 mx-0.5" />
              <button className="flex items-center gap-2 pl-1.5 pr-2 py-1 hover:bg-slate-700 rounded-md transition-colors group" aria-label="관리자 메뉴">
                <div className="w-7 h-7 bg-slate-700 rounded-md flex items-center justify-center text-slate-300 text-xs font-semibold group-hover:bg-indigo-500/20 group-hover:text-indigo-400 transition-colors">관</div>
                <div className="hidden lg:block text-left">
                  <p className="text-xs font-semibold text-slate-200 leading-none">관리자</p>
                  <p className="text-xs font-medium text-slate-500 leading-none mt-0.5">슈퍼유저</p>
                </div>
              </button>
            </div>
          </div>
        </header>

        {/* Scrollable Area */}
        <main className="flex-1 overflow-y-auto p-3 pb-20 sm:p-6 lg:p-8 scroll-smooth">
          <div className="w-full">
            <EmailStatusBanner />
            <Suspense fallback={<div className="py-12 text-center text-sm text-slate-500">불러오는 중...</div>}>
              <Outlet />
            </Suspense>
          </div>

          {/* Global Footer in Main Area */}
          <footer className="w-full mt-12 pt-6 border-t border-slate-800 flex flex-col sm:flex-row items-center justify-between gap-3 pb-8">
            <div className="flex items-center gap-2 text-slate-500">
              <img src="/favicon.svg" alt="OWLmon" className="w-5 h-5 rounded-md" />
              <span className="text-xs font-medium">OWLmon v1.2</span>
            </div>
            <div className="flex gap-6">
              <Link to="/support" className="text-xs font-medium text-slate-500 hover:text-indigo-400 transition-colors">지원</Link>
              <a href="https://github.com/SeongJae999/owlmon" target="_blank" rel="noopener noreferrer" className="text-xs font-medium text-slate-500 hover:text-indigo-400 transition-colors">GitHub</a>
            </div>
          </footer>
        </main>
      </div>
      <ChatWidget />
    </div>
  )
}

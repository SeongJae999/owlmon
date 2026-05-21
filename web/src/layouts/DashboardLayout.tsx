import React, { useState } from 'react'
import { Link, useLocation, Outlet } from 'react-router-dom'
import { logout } from '../api/auth'
import { useAlertStatus } from '../hooks/useMonitoring'
import { 
  LayoutGrid, 
  Activity, 
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
  Search,
  ListChecks,
} from 'lucide-react'
import { cn } from '../utils/cn'
import EmailStatusBanner from '../components/EmailStatusBanner'

const NAV_ITEMS = [
  { id: 'overview', path: '/', icon: LayoutGrid, label: '전체 현황', section: 'monitor' },
  // 호스트 상세는 Overview 카드 클릭으로만 진입 — 사이드바에서 제거
  { id: 'snmp', path: '/snmp', icon: Network, label: '네트워크 장비', section: 'monitor' },
  { id: 'ssl', path: '/ssl', icon: ShieldCheck, label: 'SSL 인증서', section: 'monitor' },
  { id: 'synthetic', path: '/synthetic', icon: Globe, label: '사이트 점검', section: 'monitor' },
  { id: 'dpm', path: '/dpm', icon: Database, label: 'DB 성능', section: 'monitor' },
  { id: 'logs', path: '/logs', icon: FileText, label: '로그 뷰어', section: 'monitor' },
  { id: 'rules', path: '/rules', icon: ListChecks, label: '로그 룰', section: 'manage' },
  { id: 'alert-history', path: '/history', icon: History, label: '알림 히스토리', section: 'manage' },
  { id: 'alert-settings', path: '/settings', icon: Settings, label: '알림 설정', section: 'manage' },
  { id: 'report', path: '/report', icon: FileBarChart, label: '월간 보고서', section: 'manage' },
  { id: 'assets', path: '/assets', icon: Server, label: '자산 관리', section: 'manage' },
]

export default function DashboardLayout() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
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
          <div className="w-9 h-9 bg-indigo-600 rounded-lg flex items-center justify-center text-white font-bold text-lg shrink-0">O</div>
          {!sidebarCollapsed && (
            <div className="overflow-hidden whitespace-nowrap">
              <p className="font-bold text-white text-lg">OWLmon</p>
              <p className="text-xs text-slate-500 font-medium">통합 모니터링</p>
            </div>
          )}
        </div>

        {/* Nav */}
        <nav className="flex-1 overflow-y-auto py-6 space-y-6 scrollbar-hide">
          <div className="px-3">
            {!sidebarCollapsed && <p className="text-xs font-semibold text-slate-500 px-3 mb-3">모니터링</p>}
            <div className="space-y-1.5">
              {NAV_ITEMS.filter(n => n.section === 'monitor').map(item => {
                const isActive = location.pathname === item.path
                return (
                  <Link
                    key={item.id}
                    to={item.path}
                    onClick={() => setMobileMenuOpen(false)}
                    className={cn(
                      "flex items-center gap-3.5 px-4 py-3 rounded-xl transition-all duration-200 group relative font-semibold",
                      isActive 
                        ? "bg-indigo-600 text-white shadow-lg shadow-indigo-500/20" 
                        : "hover:bg-slate-800/50 hover:text-white"
                    )}
                  >
                    <item.icon size={20} className={cn("shrink-0 transition-transform duration-200 group-hover:scale-110", isActive ? "text-white" : "text-slate-500 group-hover:text-slate-300")} />
                    {!sidebarCollapsed && <span className="text-[13px]">{item.label}</span>}
                    {item.id === 'overview' && unackedAlertCount > 0 && (
                      <span className={cn(
                        "absolute right-3 top-1/2 -translate-y-1/2 flex h-5 min-w-[20px] px-1 items-center justify-center rounded-full text-[10px] font-bold shadow-sm",
                        isActive ? "bg-white text-indigo-600" : "bg-rose-500 text-white"
                      )}>
                        {unackedAlertCount}
                      </span>
                    )}
                  </Link>
                )
              })}
            </div>
          </div>

          <div className="px-3">
            {!sidebarCollapsed && <p className="text-xs font-semibold text-slate-500 px-3 mb-3">관리</p>}
            <div className="space-y-1.5">
              {NAV_ITEMS.filter(n => n.section === 'manage').map(item => {
                const isActive = location.pathname === item.path
                return (
                  <Link
                    key={item.id}
                    to={item.path}
                    onClick={() => setMobileMenuOpen(false)}
                    className={cn(
                      "flex items-center gap-3.5 px-4 py-3 rounded-xl transition-all duration-200 group font-semibold",
                      isActive 
                        ? "bg-indigo-600 text-white shadow-lg shadow-indigo-500/20" 
                        : "hover:bg-slate-800/50 hover:text-white"
                    )}
                  >
                    <item.icon size={20} className={cn("shrink-0 transition-transform duration-200 group-hover:scale-110", isActive ? "text-white" : "text-slate-500 group-hover:text-slate-300")} />
                    {!sidebarCollapsed && <span className="text-[13px]">{item.label}</span>}
                  </Link>
                )
              })}
            </div>
          </div>
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
        <header className="h-16 bg-slate-900 border-b border-slate-800 flex items-center justify-between px-6 shrink-0 z-40 sticky top-0">
          <div className="flex items-center gap-4">
            <button
              className="lg:hidden p-2 text-slate-400 hover:bg-slate-800 rounded-lg transition-colors"
              onClick={() => setMobileMenuOpen(true)}
            >
              <Menu size={20} />
            </button>
            <div className="hidden md:flex items-center relative group">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" size={16} />
              <input
                type="text"
                placeholder="장비, 로그, 서비스 검색..."
                className="bg-slate-800 border border-slate-700 focus:border-indigo-500/40 focus:ring-2 focus:ring-indigo-500/10 rounded-lg pl-9 pr-3 py-2 text-sm w-72 transition-all outline-none text-slate-200 placeholder:text-slate-500"
              />
            </div>
          </div>

          <div className="flex items-center gap-3">
            <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 bg-emerald-500/10 rounded-lg text-xs font-semibold text-emerald-400 border border-emerald-500/20">
              <span className="w-1.5 h-1.5 bg-emerald-400 rounded-full" />
              연결 정상
            </div>

            <div className="flex items-center gap-1 p-1 bg-slate-800 rounded-lg border border-slate-700">
              <button className="p-2 text-slate-400 hover:text-indigo-400 hover:bg-slate-700 rounded-md transition-colors relative" title="알림">
                <Bell size={18} />
                {unackedAlertCount > 0 && (
                  <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-rose-500 rounded-full border border-slate-900" />
                )}
              </button>
              <div className="w-px h-5 bg-slate-700 mx-0.5" />
              <button className="flex items-center gap-2 pl-1.5 pr-2 py-1 hover:bg-slate-700 rounded-md transition-colors group">
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
        <main className="flex-1 overflow-y-auto p-6 lg:p-8 scroll-smooth">
          <div className="w-full">
            <EmailStatusBanner />
            <Outlet />
          </div>

          {/* Global Footer in Main Area */}
          <footer className="w-full mt-12 pt-6 border-t border-slate-800 flex flex-col sm:flex-row items-center justify-between gap-3 pb-8">
            <div className="flex items-center gap-2 text-slate-500">
              <div className="w-5 h-5 bg-slate-800 rounded flex items-center justify-center text-xs font-bold">O</div>
              <span className="text-xs font-medium">OWLmon v1.2</span>
            </div>
            <div className="flex gap-6">
              <a href="#" className="text-xs font-medium text-slate-500 hover:text-indigo-400 transition-colors">문서</a>
              <a href="#" className="text-xs font-medium text-slate-500 hover:text-indigo-400 transition-colors">지원</a>
              <a href="#" className="text-xs font-medium text-slate-500 hover:text-indigo-400 transition-colors">GitHub</a>
            </div>
          </footer>
        </main>
      </div>
    </div>
  )
}

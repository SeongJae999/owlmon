import { lazy } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { isLoggedIn } from './api/auth'
import LoginPage from './components/LoginPage'
import DashboardLayout from './layouts/DashboardLayout'

// 페이지별 코드 분할 — 첫 로딩 번들 축소 (Recharts 등은 쓰는 페이지에서만 로드)
const OverviewPage = lazy(() => import('./pages/Overview'))
const HostDetailPage = lazy(() => import('./pages/HostDetail'))
const AlertSettings = lazy(() => import('./components/AlertSettings'))
const AlertHistory = lazy(() => import('./components/AlertHistory'))
const SNMPDashboard = lazy(() => import('./components/SNMPDashboard'))
const AssetManagement = lazy(() => import('./components/AssetManagement'))
const WebsiteMonitoring = lazy(() => import('./components/WebsiteMonitoring'))
const DPMDashboard = lazy(() => import('./components/DPMDashboard'))
const LogViewer = lazy(() => import('./components/LogViewer'))
const InsightDashboard = lazy(() => import('./pages/InsightDashboard'))
const MonthlyReportModal = lazy(() => import('./components/MonthlyReport'))
const RulesPage = lazy(() => import('./pages/RulesPage'))
const RuleStatsPage = lazy(() => import('./pages/RuleStatsPage'))
const TrendsPage = lazy(() => import('./pages/TrendsPage'))
const SupportPage = lazy(() => import('./pages/SupportPage'))
const AuditLogPage = lazy(() => import('./pages/AuditLogPage'))
const UsersPage = lazy(() => import('./pages/UsersPage'))

export default function App() {
  const authenticated = isLoggedIn()

  if (!authenticated) {
    return <LoginPage onLogin={() => window.location.href = '/'} />
  }

  return (
    <Routes>
      <Route element={<DashboardLayout />}>
        <Route path="/" element={<OverviewPage />} />
        <Route path="/trends" element={<TrendsPage />} />
        <Route path="/detail" element={<HostDetailPage />} />
        <Route path="/snmp" element={<SNMPDashboard />} />
        <Route path="/website" element={<WebsiteMonitoring />} />
        {/* Legacy 경로 — 기능이 /website로 통합됨 (북마크 호환용 리다이렉트만 유지) */}
        <Route path="/ssl" element={<Navigate to="/website" replace />} />
        <Route path="/synthetic" element={<Navigate to="/website" replace />} />
        <Route path="/dpm" element={<DPMDashboard />} />
        <Route path="/logs" element={<LogViewer />} />
        <Route path="/insights" element={<InsightDashboard />} />
        {/* OWL AI는 모든 화면 우측 하단 버튼으로 이동 (ChatWidget) */}
        <Route path="/chat" element={<Navigate to="/" replace />} />
        <Route path="/rules" element={<RulesPage />} />
        <Route path="/rules/stats" element={<RuleStatsPage />} />
        <Route path="/history" element={<AlertHistory />} />
        <Route path="/settings" element={<AlertSettings />} />
        <Route path="/report" element={<MonthlyReportModal />} />
        <Route path="/assets" element={<AssetManagement />} />
        <Route path="/support" element={<SupportPage />} />
        <Route path="/audit" element={<AuditLogPage />} />
        <Route path="/users" element={<UsersPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

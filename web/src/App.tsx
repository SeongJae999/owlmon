import { Routes, Route, Navigate } from 'react-router-dom'
import { isLoggedIn } from './api/auth'
import LoginPage from './components/LoginPage'
import DashboardLayout from './layouts/DashboardLayout'

// Pages
import OverviewPage from './pages/Overview'
import HostDetailPage from './pages/HostDetail'
import AlertSettings from './components/AlertSettings'
import AlertHistory from './components/AlertHistory'
import SNMPDashboard from './components/SNMPDashboard'
import AssetManagement from './components/AssetManagement'
import WebsiteMonitoring from './components/WebsiteMonitoring'
import DPMDashboard from './components/DPMDashboard'
import LogViewer from './components/LogViewer'
import MonthlyReportModal from './components/MonthlyReport'
import RulesPage from './pages/RulesPage'
import RuleStatsPage from './pages/RuleStatsPage'
import TrendsPage from './pages/TrendsPage'
import SupportPage from './pages/SupportPage'
import AuditLogPage from './pages/AuditLogPage'

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
        <Route path="/rules" element={<RulesPage />} />
        <Route path="/rules/stats" element={<RuleStatsPage />} />
        <Route path="/history" element={<AlertHistory />} />
        <Route path="/settings" element={<AlertSettings />} />
        <Route path="/report" element={<MonthlyReportModal />} />
        <Route path="/assets" element={<AssetManagement />} />
        <Route path="/support" element={<SupportPage />} />
        <Route path="/audit" element={<AuditLogPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

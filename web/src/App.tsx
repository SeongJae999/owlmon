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
import SSLDashboard from './components/SSLDashboard'
import SyntheticDashboard from './components/SyntheticDashboard'
import DPMDashboard from './components/DPMDashboard'
import LogViewer from './components/LogViewer'
import MonthlyReportModal from './components/MonthlyReport'
import RulesPage from './pages/RulesPage'
import RuleStatsPage from './pages/RuleStatsPage'
import TrendsPage from './pages/TrendsPage'

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
        <Route path="/ssl" element={<SSLDashboard />} />
        <Route path="/synthetic" element={<SyntheticDashboard />} />
        <Route path="/dpm" element={<DPMDashboard />} />
        <Route path="/logs" element={<LogViewer />} />
        <Route path="/rules" element={<RulesPage />} />
        <Route path="/rules/stats" element={<RuleStatsPage />} />
        <Route path="/history" element={<AlertHistory />} />
        <Route path="/settings" element={<AlertSettings />} />
        <Route path="/report" element={<MonthlyReportModal />} />
        <Route path="/assets" element={<AssetManagement />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

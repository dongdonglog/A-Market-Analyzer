import { App as AntApp } from 'antd'
import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { clearAuth, getStoredToken } from './lib/auth'
import { AdminPage } from './pages/AdminPage'
import { BillingPage } from './pages/BillingPage'
import { DashboardPage } from './pages/DashboardPage'
import { LoginPage } from './pages/LoginPage'

function ProtectedRoute() {
  if (!getStoredToken()) {
    clearAuth()
    return <Navigate to="/login" replace />
  }

  return <Outlet />
}

function App() {
  return (
    <AntApp>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<ProtectedRoute />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/billing" element={<BillingPage />} />
          <Route path="/admin" element={<AdminPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AntApp>
  )
}

export default App

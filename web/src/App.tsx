import { BrowserRouter, Routes, Route, Navigate, Outlet, useNavigate, useLocation } from 'react-router-dom'
import { useState } from 'react'
import { AuthProvider, useAuth } from './auth'
import SideNavBar from './components/SideNavBar'
import TopAppBar from './components/TopAppBar'
import Dashboard from './pages/Dashboard'
import SessionList from './pages/SessionList'
import SessionChat from './pages/SessionChat'
import LogViewer from './pages/LogViewer'
import WechatBind from './pages/WechatBind'
import Settings from './pages/Settings'
import Login from './pages/Login'

function Shell() {
  const navigate = useNavigate()
  const location = useLocation()
  const [newSessionSignal, setNewSessionSignal] = useState(0)

  const handleNewSession = () => {
    if (location.pathname !== '/sessions') {
      navigate('/sessions', { state: { newSession: true } })
    } else {
      setNewSessionSignal(prev => prev + 1)
    }
  }

  return (
    <div className="h-screen bg-background text-on-surface overflow-hidden">
      <SideNavBar onNewSession={handleNewSession} />
      <div className="ml-sidebar h-full flex flex-col overflow-hidden">
        <TopAppBar />
        <main className="flex-1 overflow-y-auto">
          <Outlet context={{ newSessionSignal, setNewSessionSignal }} />
        </main>
      </div>
    </div>
  )
}

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()

  if (loading) {
    return (
      <div className="h-screen flex items-center justify-center bg-background text-on-surface-variant">
        <span className="material-symbols-outlined animate-spin text-[32px]">progress_activity</span>
      </div>
    )
  }

  if (!user) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            path="/"
            element={
              <RequireAuth>
                <Shell />
              </RequireAuth>
            }
          >
            <Route index element={<Dashboard />} />
            <Route path="sessions" element={<SessionList />} />
            <Route path="sessions/:id" element={<SessionChat />} />
            <Route path="log" element={<LogViewer />} />
            <Route path="wechat" element={<WechatBind />} />
            <Route path="settings" element={<Settings />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}

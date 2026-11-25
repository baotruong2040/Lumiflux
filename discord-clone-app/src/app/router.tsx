import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ProtectedRoute, PublicOnlyRoute } from '../features/auth/components/RouteGuards'
import { LoginPage } from '../features/auth/pages/LoginPage'
import { RegisterPage } from '../features/auth/pages/RegisterPage'
import { WorkspacePage } from '../features/workspace/pages/WorkspacePage'

export function AppRouter() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<PublicOnlyRoute />}>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
        </Route>

        <Route element={<ProtectedRoute />}>
          <Route path="/app" element={<WorkspacePage />} />
          <Route path="/app/guilds/:guildId" element={<WorkspacePage />} />
          <Route path="/app/guilds/:guildId/channels/:channelId" element={<WorkspacePage />} />
        </Route>

        <Route path="/" element={<Navigate to="/app" replace />} />
        <Route path="*" element={<Navigate to="/app" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

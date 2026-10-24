import { Routes, Route } from 'react-router-dom'
import AppLayout from '../components/layout/AppLayout'
import AuthLayout from '../components/layout/AuthLayout'
import ProtectedRoute from './ProtectedRoute'

import Login from '../pages/auth/Login'
import Dashboard from '../pages/dashboard/Dashboard'
import Users from '../pages/users/Users'
import Menus from '../pages/menus/Menus'
import Roles from '../pages/roles/Roles'
import RolePermissions from '../pages/roles/RolePermissions'
import Settings from '../pages/settings/Settings'
import ActivityLogs from '../pages/activity_logs/ActivityLogs'
import FileManager from '../pages/file_manager/FileManager'
import Terminal from '../pages/terminal/Terminal'
import Websites from '../pages/websites/Websites'
import Databases from '../pages/databases/Databases'
import Docker from '../pages/docker/Docker'
import Cron from '../pages/cron/Cron'
import Firewall from '../pages/firewall/Firewall'
import Security from '../pages/security/Security'
import Forbidden from '../pages/Forbidden'
import NotFound from '../pages/NotFound'
import Maintenance from '../pages/Maintenance'

function AppRouter() {
  return (
    <Routes>
      <Route element={<AuthLayout />}>
        <Route path="/login" element={<Login />} />
      </Route>

      <Route path="/maintenance" element={<Maintenance />} />
      <Route path="/403" element={<Forbidden />} />
      <Route path="/404" element={<NotFound />} />

      <Route
        element={
          <ProtectedRoute>
            <AppLayout />
          </ProtectedRoute>
        }
      >
        <Route path="/" element={<Dashboard />} />
        <Route
          path="/users"
          element={
            <ProtectedRoute menuKey="users">
              <Users />
            </ProtectedRoute>
          }
        />
        <Route
          path="/menus"
          element={
            <ProtectedRoute menuKey="menus">
              <Menus />
            </ProtectedRoute>
          }
        />
        <Route
          path="/roles"
          element={
            <ProtectedRoute menuKey="roles">
              <Roles />
            </ProtectedRoute>
          }
        />
        <Route
          path="/roles/:uuid/permissions"
          element={
            <ProtectedRoute menuKey="roles">
              <RolePermissions />
            </ProtectedRoute>
          }
        />
        <Route
          path="/settings"
          element={
            <ProtectedRoute menuKey="settings">
              <Settings />
            </ProtectedRoute>
          }
        />
        <Route
          path="/activity-logs"
          element={
            <ProtectedRoute menuKey="activity-logs">
              <ActivityLogs />
            </ProtectedRoute>
          }
        />
        <Route
          path="/file-manager"
          element={
            <ProtectedRoute menuKey="file-manager">
              <FileManager />
            </ProtectedRoute>
          }
        />
        <Route
          path="/terminal"
          element={
            <ProtectedRoute menuKey="terminal">
              <Terminal />
            </ProtectedRoute>
          }
        />
        <Route
          path="/websites"
          element={
            <ProtectedRoute menuKey="websites">
              <Websites />
            </ProtectedRoute>
          }
        />
        <Route
          path="/databases"
          element={
            <ProtectedRoute menuKey="databases">
              <Databases />
            </ProtectedRoute>
          }
        />
        <Route
          path="/docker"
          element={
            <ProtectedRoute menuKey="docker">
              <Docker />
            </ProtectedRoute>
          }
        />
        <Route
          path="/cron"
          element={
            <ProtectedRoute menuKey="cron">
              <Cron />
            </ProtectedRoute>
          }
        />
        <Route
          path="/firewall"
          element={
            <ProtectedRoute menuKey="firewall">
              <Firewall />
            </ProtectedRoute>
          }
        />
        <Route path="/security" element={<Security />} />
      </Route>

      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}

export default AppRouter

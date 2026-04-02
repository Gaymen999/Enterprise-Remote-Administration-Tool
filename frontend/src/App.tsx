import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ProtectedRoute } from './components/ProtectedRoute';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { Terminal } from './pages/Terminal';
import { Login } from './pages/Login';
import { AgentsPage } from './pages/AgentsPage';

function Settings() {
  return (
    <div className="p-6">
      <h2 className="text-2xl font-bold text-white mb-4">Settings</h2>
      <p className="text-gray-400">Settings page coming soon...</p>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          path="/dashboard"
          element={
            <ProtectedRoute>
              <Layout>
                <Dashboard />
              </Layout>
            </ProtectedRoute>
          }
        />
        <Route
          path="/agents"
          element={
            <ProtectedRoute>
              <Layout>
                <AgentsPage />
              </Layout>
            </ProtectedRoute>
          }
        />
        <Route
          path="/terminal/:id"
          element={
            <ProtectedRoute>
              <Layout>
                <Terminal />
              </Layout>
            </ProtectedRoute>
          }
        />
        <Route
          path="/terminal"
          element={
            <ProtectedRoute>
              <Layout>
                <div className="p-6">
                  <h2 className="text-2xl font-bold text-white mb-4">Terminal</h2>
                  <p className="text-gray-400">Select an agent from the Dashboard to connect.</p>
                </div>
              </Layout>
            </ProtectedRoute>
          }
        />
        <Route
          path="/settings"
          element={
            <ProtectedRoute>
              <Layout>
                <Settings />
              </Layout>
            </ProtectedRoute>
          }
        />
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;

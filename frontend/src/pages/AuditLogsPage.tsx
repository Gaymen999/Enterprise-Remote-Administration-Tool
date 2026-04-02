import { useState, useEffect } from 'react';
import { 
  Search, Filter, Download, ChevronLeft, ChevronRight,
  Clock, User, Activity, AlertCircle, CheckCircle, XCircle
} from 'lucide-react';
import { api } from '../services/api';

interface AuditLog {
  id: string;
  user_id: string;
  username: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: {
    method?: string;
    path?: string;
    status_code?: number;
    duration_ms?: number;
    query?: string;
  };
  ip_address: string;
  user_agent?: string;
  created_at: string;
}

interface AuditLogsResponse {
  logs: AuditLog[];
  count: number;
  limit: number;
  offset: number;
}

const ACTION_COLORS: Record<string, string> = {
  read: 'bg-blue-500/20 text-blue-400 border-blue-500/50',
  create: 'bg-green-500/20 text-green-400 border-green-500/50',
  update: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/50',
  delete: 'bg-red-500/20 text-red-400 border-red-500/50',
  login: 'bg-purple-500/20 text-purple-400 border-purple-500/50',
  logout: 'bg-gray-500/20 text-gray-400 border-gray-500/50',
};

const ACTION_ICONS: Record<string, React.ReactNode> = {
  read: <Search className="w-4 h-4" />,
  create: <CheckCircle className="w-4 h-4" />,
  update: <Activity className="w-4 h-4" />,
  delete: <XCircle className="w-4 h-4" />,
};

export function AuditLogsPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [totalCount, setTotalCount] = useState(0);
  const [page, setPage] = useState(0);
  const [filters, setFilters] = useState({
    action: '',
    resource_type: '',
    search: '',
  });
  const limit = 50;

  const fetchLogs = async () => {
    setLoading(true);
    setError(null);

    try {
      const params = new URLSearchParams({
        limit: limit.toString(),
        offset: (page * limit).toString(),
      });

      if (filters.action) params.append('action', filters.action);
      if (filters.resource_type) params.append('resource_type', filters.resource_type);

      const response = await api.get(`/audit/logs?${params.toString()}`);
      const data: AuditLogsResponse = response.data;

      setLogs(data.logs || []);
      setTotalCount(data.count || 0);
    } catch (err) {
      setError('Failed to fetch audit logs');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchLogs();
  }, [page, filters.action, filters.resource_type]);

  const formatTimestamp = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleString();
  };

  const formatDuration = (ms?: number) => {
    if (!ms) return '-';
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  const getStatusBadge = (statusCode?: number) => {
    if (!statusCode) return null;

    if (statusCode >= 200 && statusCode < 300) {
      return <span className="px-2 py-0.5 text-xs rounded bg-green-500/20 text-green-400">OK</span>;
    }
    if (statusCode >= 400 && statusCode < 500) {
      return <span className="px-2 py-0.5 text-xs rounded bg-yellow-500/20 text-yellow-400">{statusCode}</span>;
    }
    if (statusCode >= 500) {
      return <span className="px-2 py-0.5 text-xs rounded bg-red-500/20 text-red-400">{statusCode}</span>;
    }
    return <span className="px-2 py-0.5 text-xs rounded bg-gray-500/20 text-gray-400">{statusCode}</span>;
  };

  const exportLogs = () => {
    const csv = [
      ['Timestamp', 'User', 'Action', 'Resource Type', 'Resource ID', 'IP Address', 'Status', 'Duration'].join(','),
      ...logs.map(log => [
        log.created_at,
        log.username,
        log.action,
        log.resource_type,
        log.resource_id,
        log.ip_address,
        log.details.status_code || '',
        log.details.duration_ms || '',
      ].map(v => `"${v}"`).join(',')),
    ].join('\n');

    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `audit-logs-${new Date().toISOString().split('T')[0]}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const totalPages = Math.ceil(totalCount / limit);

  return (
    <div className="flex flex-col h-full">
      <div className="p-4 bg-gray-800 border-b border-gray-700">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-xl font-bold text-white flex items-center gap-2">
            <Activity className="w-6 h-6" />
            Audit Logs
          </h1>
          <button
            onClick={exportLogs}
            className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded text-sm"
          >
            <Download className="w-4 h-4" />
            Export CSV
          </button>
        </div>

        <div className="flex items-center gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              type="text"
              placeholder="Search logs..."
              value={filters.search}
              onChange={(e) => setFilters({ ...filters, search: e.target.value })}
              className="w-full pl-10 pr-4 py-2 bg-gray-700 border border-gray-600 rounded text-white text-sm placeholder-gray-400"
            />
          </div>

          <select
            value={filters.action}
            onChange={(e) => setFilters({ ...filters, action: e.target.value })}
            className="px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white text-sm"
          >
            <option value="">All Actions</option>
            <option value="read">Read</option>
            <option value="create">Create</option>
            <option value="update">Update</option>
            <option value="delete">Delete</option>
            <option value="login">Login</option>
            <option value="logout">Logout</option>
          </select>

          <select
            value={filters.resource_type}
            onChange={(e) => setFilters({ ...filters, resource_type: e.target.value })}
            className="px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white text-sm"
          >
            <option value="">All Resources</option>
            <option value="agent">Agents</option>
            <option value="command">Commands</option>
            <option value="user">Users</option>
            <option value="file">Files</option>
          </select>

          <button
            onClick={fetchLogs}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded text-white text-sm"
          >
            Apply
          </button>
        </div>
      </div>

      {error && (
        <div className="p-4 bg-red-500/20 border-b border-red-500/50 text-red-400 flex items-center gap-2">
          <AlertCircle className="w-4 h-4" />
          {error}
        </div>
      )}

      <div className="flex-1 overflow-auto">
        {loading ? (
          <div className="flex items-center justify-center h-full text-gray-400">
            Loading audit logs...
          </div>
        ) : logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-gray-400">
            No audit logs found
          </div>
        ) : (
          <table className="w-full">
            <thead className="bg-gray-800 sticky top-0">
              <tr className="text-left text-xs text-gray-400 uppercase">
                <th className="px-4 py-3">Timestamp</th>
                <th className="px-4 py-3">User</th>
                <th className="px-4 py-3">Action</th>
                <th className="px-4 py-3">Resource</th>
                <th className="px-4 py-3">Details</th>
                <th className="px-4 py-3">IP Address</th>
                <th className="px-4 py-3">Duration</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id} className="border-b border-gray-800 hover:bg-gray-800/50">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2 text-sm">
                      <Clock className="w-4 h-4 text-gray-500" />
                      <span className="text-gray-300">{formatTimestamp(log.created_at)}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <User className="w-4 h-4 text-gray-500" />
                      <span className="text-white text-sm">{log.username || 'system'}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded border text-xs ${ACTION_COLORS[log.action] || 'bg-gray-500/20 text-gray-400 border-gray-500/50'}`}>
                      {ACTION_ICONS[log.action] || <Activity className="w-4 h-4" />}
                      {log.action}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <div className="text-sm">
                      <span className="text-gray-400">{log.resource_type}</span>
                      {log.resource_id && (
                        <span className="text-gray-500 ml-1">/{log.resource_id.slice(0, 8)}...</span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      {getStatusBadge(log.details.status_code)}
                      {log.details.path && (
                        <span className="text-xs text-gray-500 truncate max-w-[200px]" title={log.details.path}>
                          {log.details.path}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className="text-gray-400 text-sm font-mono">{log.ip_address}</span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="text-gray-400 text-sm">{formatDuration(log.details.duration_ms)}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="p-4 bg-gray-800 border-t border-gray-700 flex items-center justify-between">
        <span className="text-sm text-gray-400">
          Showing {page * limit + 1} - {Math.min((page + 1) * limit, totalCount)} of {totalCount}
        </span>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setPage(Math.max(0, page - 1))}
            disabled={page === 0}
            className="p-2 hover:bg-gray-700 rounded disabled:opacity-50"
          >
            <ChevronLeft className="w-4 h-4 text-gray-400" />
          </button>

          <span className="text-sm text-gray-400">
            Page {page + 1} of {totalPages || 1}
          </span>

          <button
            onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
            disabled={page >= totalPages - 1}
            className="p-2 hover:bg-gray-700 rounded disabled:opacity-50"
          >
            <ChevronRight className="w-4 h-4 text-gray-400" />
          </button>
        </div>
      </div>
    </div>
  );
}

import { useEffect, useState, useCallback } from 'react';
import { Terminal, RefreshCw, Loader2 } from 'lucide-react';
import { Link } from 'react-router-dom';
import { AgentStatusBadge } from '../components/AgentStatusBadge';
import { agentService } from '../services/api';
import type { Agent } from '../types/agent';

export function Dashboard() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastRefresh, setLastRefresh] = useState<Date>(new Date());

  const fetchAgents = useCallback(async () => {
    try {
      const data = await agentService.getAgents();
      setAgents(data || []);
      setError(null);
      setLastRefresh(new Date());
    } catch (err) {
      console.error('Failed to load agents:', err);
      setError('Failed to load agents - Is the backend running?');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();

    const interval = setInterval(() => {
      fetchAgents();
    }, 5000);

    return () => clearInterval(interval);
  }, [fetchAgents]);

  if (loading && agents.length === 0) {
    return (
      <div className="p-6 flex items-center justify-center h-full">
        <Loader2 className="animate-spin text-blue-500 mr-2" size={24} />
        <span className="text-gray-400">Loading agents...</span>
      </div>
    );
  }

  return (
    <div className="p-6 h-full flex flex-col">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold text-white mb-1">Agent Fleet</h2>
          <p className="text-gray-400 text-sm">
            {agents.length} endpoint{agents.length !== 1 ? 's' : ''} registered
            <span className="ml-2 text-gray-500">• Last updated: {lastRefresh.toLocaleTimeString()}</span>
          </p>
        </div>
        <button
          onClick={fetchAgents}
          className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-lg transition-colors"
        >
          <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {error && (
        <div className="mb-4 p-4 bg-red-500/20 border border-red-500/50 rounded-lg text-red-400">
          {error}
        </div>
      )}

      <div className="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden flex-1">
        <table className="w-full">
          <thead className="bg-gray-700/50 sticky top-0">
            <tr>
              <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Hostname</th>
              <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">OS</th>
              <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">IP Address</th>
              <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Status</th>
              <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Last Seen</th>
              <th className="px-4 py-3 text-right text-sm font-semibold text-gray-300">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-700">
            {agents.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">
                  No agents connected yet. Start an agent to see it here.
                </td>
              </tr>
            ) : (
              agents.map((agent) => (
                <tr key={agent.id} className="hover:bg-gray-700/30 transition-colors">
                  <td className="px-4 py-3 text-white font-medium">{agent.hostname || 'Unknown'}</td>
                  <td className="px-4 py-3 text-gray-400">
                    {agent.os_family || 'Unknown'} {agent.os_version || ''}
                  </td>
                  <td className="px-4 py-3 text-gray-400 font-mono text-sm">{agent.ip_address || 'N/A'}</td>
                  <td className="px-4 py-3">
                    <AgentStatusBadge status={agent.status || 'offline'} lastSeen={agent.last_seen} />
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-sm">
                    {agent.last_seen ? new Date(agent.last_seen).toLocaleString() : 'Never'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Link
                      to={`/terminal/${agent.id}`}
                      className="inline-flex items-center gap-2 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded-md transition-colors"
                    >
                      <Terminal size={16} />
                      Connect
                    </Link>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

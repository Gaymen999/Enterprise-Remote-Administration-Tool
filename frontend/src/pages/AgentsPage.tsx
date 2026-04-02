import { useEffect, useState } from 'react';
import { Terminal } from 'lucide-react';
import { Link } from 'react-router-dom';
import { AgentStatusBadge } from '../components/AgentStatusBadge';
import { agentService } from '../services/api';
import type { Agent } from '../types/agent';

export function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchAgents = async () => {
      try {
        const data = await agentService.getAgents();
        setAgents(data || []);
      } catch (err) {
        console.error('Failed to load agents:', err);
        setError('Failed to load agents - Is the backend running?');
      } finally {
        setLoading(false);
      }
    };
    fetchAgents();
  }, []);

  if (loading) {
    return (
      <div className="p-6 flex items-center justify-center h-full">
        <div className="text-gray-400">Loading agents...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-red-500/20 border border-red-500/50 rounded-lg p-4 text-red-400">
          {error}
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 h-full flex flex-col">
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-white mb-2">Agent Management</h2>
        <p className="text-gray-400">{agents.length} endpoint{agents.length !== 1 ? 's' : ''} registered</p>
      </div>

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
                  No agents connected yet.
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

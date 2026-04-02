import React from 'react';

interface AgentStatusBadgeProps {
  status: 'online' | 'offline' | 'error';
  lastSeen?: string;
}

export const AgentStatusBadge: React.FC<AgentStatusBadgeProps> = ({ status, lastSeen }) => {
  const statusColors = {
    online: 'bg-green-100 text-green-800 border-green-200',
    offline: 'bg-gray-100 text-gray-800 border-gray-200',
    error: 'bg-red-100 text-red-800 border-red-200'
  };

  return (
    <div className="flex flex-col items-start gap-1">
      <span className={`px-2.5 py-0.5 rounded-full text-xs font-medium border ${statusColors[status]}`}>
        {status.toUpperCase()}
      </span>
      {lastSeen && (
        <span className="text-xs text-gray-500">
          Last: {new Date(lastSeen).toLocaleTimeString()}
        </span>
      )}
    </div>
  );
};

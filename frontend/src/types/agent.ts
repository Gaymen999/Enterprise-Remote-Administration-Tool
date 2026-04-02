export interface Agent {
  id: string;
  hostname: string;
  ip_address: string;
  os_family: string;
  os_version: string;
  agent_version: string;
  status: 'online' | 'offline' | 'error';
  last_seen: string;
  created_at: string;
  updated_at: string;
}

export interface Host {
  id: string;
  hostname: string;
  ip_address?: string;
  agent_version: string;
  last_seen: string;
}

export interface CheckResult {
  connector: string;
  display_name: string;
  category: string;
  values: Record<string, string>;
  update_available: boolean;
  changelog: string;
  error?: string;
  checked_at: string;
  ignored?: boolean;
  ignored_items?: string[];
}

export interface HostDetail {
  host: Host;
  results: CheckResult[];
  commands: Command[];
}

export interface HostStatus {
  host: Host;
  results: CheckResult[];
}

export interface Command {
  id: string;
  host_id: string;
  connector: string;
  status: 'pending' | 'running' | 'done' | 'failed';
  output: string;
  created_at: string;
  updated_at: string;
}

export interface ConnectorSpec {
  name: string;
  vars: Record<string, string>;
}

export interface Provision {
  token: string;
  name: string;
  host_type: string;
  connectors: ConnectorSpec[];
  created_at: string;
  claimed_by?: string;
  server_url?: string;
}

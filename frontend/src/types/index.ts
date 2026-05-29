export interface Host {
  id: string;
  hostname: string;
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
}

export interface HostStatus {
  host: Host;
  results: CheckResult[];
}

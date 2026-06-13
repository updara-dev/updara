import type { HostStatus, HostDetail, Command } from '../types';

const BASE = import.meta.env.VITE_SERVER_URL ?? '';

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const token = localStorage.getItem('updara_token') ?? '';
  const res = await fetch(`${BASE}${path}`, {
    ...opts,
    headers: {
      ...opts?.headers,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
  if (res.status === 401) {
    localStorage.removeItem('updara_token');
    window.location.reload();
    throw new Error('Unauthorized');
  }
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const fetchHosts = () =>
  request<HostStatus[]>('/api/v1/hosts');

export interface ConnectorMeta {
  name: string;
  display_name: string;
  category: string;
  vars: { name: string; description: string; required: boolean; default: string }[];
  has_update: boolean;
  hint?: string;
}

export const fetchConnectors = () =>
  request<ConnectorMeta[]>('/api/v1/connectors');

export interface Provision {
  token: string;
  name: string;
  host_type: string;
  connectors: { name: string; vars: Record<string, string> }[];
  created_at: string;
  claimed_by?: string;
}

export const fetchProvisions = () =>
  request<Provision[]>('/api/v1/provisions');

export const createProvision = (body: {
  name: string;
  host_type: string;
  connectors: { name: string; vars: Record<string, string> }[];
}) =>
  request<{ token: string; install_cmd: string }>('/api/v1/provisions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });

export const deleteProvision = (token: string) =>
  request<void>(`/api/v1/provisions/${token}`, { method: 'DELETE' });

export const fetchConnectorYAML = async (name: string): Promise<string> => {
  const res = await fetch(`${BASE}/api/v1/connectors/${encodeURIComponent(name)}/yaml`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.text();
};

export const saveConnectorYAML = (name: string, yaml: string) =>
  request<void>(`/api/v1/connectors/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'text/plain' },
    body: yaml,
  });

export const deleteConnector = (name: string) =>
  request<void>(`/api/v1/connectors/${encodeURIComponent(name)}`, { method: 'DELETE' });

export const triggerUpdate = (hostname: string, connector: string) =>
  request<Command>(`/api/v1/hosts/${encodeURIComponent(hostname)}/update/${connector}`, {
    method: 'POST',
  });

export const fetchCommands = (hostname: string) =>
  request<Command[]>(`/api/v1/hosts/${encodeURIComponent(hostname)}/commands`);

export const fetchHostDetail = (hostname: string) =>
  request<HostDetail>(`/api/v1/hosts/${encodeURIComponent(hostname)}`);

export const ignoreConnector = (hostname: string, connector: string, item?: string) =>
  request<void>(`/api/v1/hosts/${encodeURIComponent(hostname)}/ignore/${encodeURIComponent(connector)}${item != null ? `?item=${encodeURIComponent(item)}` : ''}`, {
    method: 'POST',
  });

export const unignoreConnector = (hostname: string, connector: string, item?: string) =>
  request<void>(`/api/v1/hosts/${encodeURIComponent(hostname)}/ignore/${encodeURIComponent(connector)}${item != null ? `?item=${encodeURIComponent(item)}` : ''}`, {
    method: 'DELETE',
  });

export const removeHostConnector = (hostname: string, connector: string) =>
  request<void>(`/api/v1/hosts/${encodeURIComponent(hostname)}/connectors/${encodeURIComponent(connector)}`, {
    method: 'DELETE',
  });

export const deleteHost = (hostname: string) =>
  request<void>(`/api/v1/hosts/${encodeURIComponent(hostname)}`, { method: 'DELETE' });

export const renameHost = (hostname: string, displayName: string) =>
  request<void>(`/api/v1/hosts/${encodeURIComponent(hostname)}/rename`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ display_name: displayName }),
  });

export const fetchHostProvision = (hostname: string) =>
  request<import('../types').Provision>(`/api/v1/hosts/${encodeURIComponent(hostname)}/provision`);

export const updateProvision = (token: string, connectors: { name: string; vars: Record<string, string> }[]) =>
  request<void>(`/api/v1/provisions/${encodeURIComponent(token)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ connectors }),
  });

export const recheckConnector = (hostname: string, connector: string) =>
  request<void>(`/api/v1/hosts/${encodeURIComponent(hostname)}/recheck/${encodeURIComponent(connector)}`, {
    method: 'POST',
  });

export const syncAgent = (hostname: string) =>
  request<Command>(`/api/v1/hosts/${encodeURIComponent(hostname)}/sync`, {
    method: 'POST',
  });

export const installConnector = (hostname: string, connector: string, vars?: Record<string, string>) =>
  request<Command>(`/api/v1/hosts/${encodeURIComponent(hostname)}/connectors/${encodeURIComponent(connector)}/install`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ vars: vars ?? {} }),
  });

export interface NotificationSettings {
  ntfy_url: string;
  ntfy_topic: string;
  ntfy_enabled: boolean;
  telegram_token: string;
  telegram_chat_id: string;
  telegram_enabled: boolean;
  email_enabled: boolean;
  email_host: string;
  email_port: string;
  email_username: string;
  email_password: string;
  email_from: string;
  email_to: string;
  email_tls: string;
  digest_enabled: boolean;
  digest_frequency: string;
  digest_weekday: number;
  digest_day: number;
  digest_time: string;
  cooldown_days: number;
  min_count: number;
  batch_schedule: string;
  batch_time1: string;
  batch_time2: string;
  show_lts_upgrades: boolean;
}

export const fetchNotificationSettings = () =>
  request<NotificationSettings>('/api/v1/settings/notifications');

export const saveNotificationSettings = (s: NotificationSettings) =>
  request<void>('/api/v1/settings/notifications', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(s),
  });

export const testNotification = () =>
  request<void>('/api/v1/settings/notifications/test', { method: 'POST' });

export const testDigest = () =>
  request<void>('/api/v1/settings/notifications/test-digest', { method: 'POST' });

export interface HostStatSummary {
  hostname: string;
  ip_address: string;
  last_update: string;
  total_done: number;
  done_30days: number;
  top_connector: string;
}

export interface UpdateRecord {
  connector: string;
  display_name: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export const fetchGlobalStats = () =>
  request<HostStatSummary[]>('/api/v1/stats');

export const fetchHostStats = (hostname: string) =>
  request<UpdateRecord[]>(`/api/v1/hosts/${encodeURIComponent(hostname)}/stats`);

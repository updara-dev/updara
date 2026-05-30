import type { HostStatus, Command } from '../types';

const BASE = import.meta.env.VITE_SERVER_URL ?? '';

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, opts);
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

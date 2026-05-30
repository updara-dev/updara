import type { HostStatus } from '../types';
import { CheckRow } from './CheckRow';

interface Props {
  status: HostStatus;
}

function cardColor(status: HostStatus): string {
  const results = status.results ?? [];
  if (results.some(r => r.error)) return 'var(--color-critical)';
  if (results.some(r => r.update_available)) return 'var(--color-warning)';
  return 'var(--color-ok)';
}

export function HostCard({ status }: Props) {
  const { host, results } = status;
  const color = cardColor(status);
  const lastSeen = new Date(host.last_seen).toLocaleString();

  const byCategory = (results ?? []).reduce<Record<string, typeof results>>((acc, r) => {
    const cat = r.category || 'other';
    (acc[cat] ??= []).push(r);
    return acc;
  }, {});

  return (
    <div className="host-card" style={{ borderTop: `3px solid ${color}` }}>
      <div className="host-card__header">
        <div className="host-card__title">
          <span className="host-card__dot" style={{ background: color }} />
          <h2>{host.hostname}</h2>
        </div>
        <span className="host-card__meta">
          {host.ip_address && <span className="host-ip">{host.ip_address} · </span>}
          agent v{host.agent_version} · {lastSeen}
        </span>
      </div>

      {Object.entries(byCategory).map(([cat, items]) => (
        <div key={cat} className="host-card__category">
          <h3>{cat}</h3>
          {items.map(r => <CheckRow key={r.connector} result={r} hostname={host.hostname} />)}
        </div>
      ))}

      {(results ?? []).length === 0 && (
        <p className="host-card__empty">No checks reported yet.</p>
      )}
    </div>
  );
}

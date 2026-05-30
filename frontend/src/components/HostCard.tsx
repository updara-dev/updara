import type { HostStatus } from '../types';
import { CheckRow } from './CheckRow';
import { useT } from '../i18n';

interface Props {
  status: HostStatus;
  selected: Set<string>;
  onToggleSelect: (hostname: string, connector: string) => void;
  onClick?: () => void;
}

function cardColor(status: HostStatus): string {
  const results = status.results ?? [];
  if (results.some(r => r.error)) return 'var(--color-critical)';
  if (results.some(r => r.update_available)) return 'var(--color-warning)';
  return 'var(--color-ok)';
}

export function HostCard({ status, selected, onToggleSelect, onClick }: Props) {
  const { t } = useT();
  const { host, results } = status;
  const color = cardColor(status);
  const lastSeen = new Date(host.last_seen).toLocaleString();

  const byCategory = (results ?? []).reduce<Record<string, typeof results>>((acc, r) => {
    const cat = r.category || 'other';
    (acc[cat] ??= []).push(r);
    return acc;
  }, {});

  return (
    <div
      className={`host-card${onClick ? ' host-card--clickable' : ''}`}
      style={{ borderTop: `3px solid ${color}` }}
      onClick={(e) => {
        if ((e.target as HTMLElement).closest('button, a, input')) return;
        onClick?.();
      }}
    >
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
          {items.map(r => (
            <CheckRow
              key={r.connector}
              result={r}
              hostname={host.hostname}
              isSelected={selected.has(`${host.hostname}::${r.connector}`)}
              onToggleSelect={() => onToggleSelect(host.hostname, r.connector)}
            />
          ))}
        </div>
      ))}

      {(results ?? []).length === 0 && (
        <p className="host-card__empty">{t.dashboard.noChecks}</p>
      )}
    </div>
  );
}

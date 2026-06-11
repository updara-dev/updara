import { useEffect, useState } from 'react';
import { fetchGlobalStats } from '../api/client';
import type { HostStatSummary } from '../api/client';
import { useT } from '../i18n';

function timeAgo(iso: string): string {
  if (!iso) return '';
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

interface Props {
  onSelectHost: (hostname: string) => void;
}

export function StatsPage({ onSelectHost }: Props) {
  const { t } = useT();
  const [stats, setStats] = useState<HostStatSummary[] | null>(null);
  const [filter, setFilter] = useState('');

  useEffect(() => {
    fetchGlobalStats().then(setStats).catch(() => setStats([]));
  }, []);

  const filtered = stats?.filter(r =>
    !filter || r.hostname.toLowerCase().includes(filter.toLowerCase()) || r.ip_address.includes(filter)
  ) ?? [];

  const totalDone = stats?.reduce((s, h) => s + h.total_done, 0) ?? 0;
  const done30 = stats?.reduce((s, h) => s + h.done_30days, 0) ?? 0;

  return (
    <div className="stats-page">
      <div className="stats-page__header">
        <h2 className="stats-page__title">{t.stats.title}</h2>
        <p className="stats-page__subtitle">{t.stats.subtitle}</p>
      </div>

      {stats === null ? (
        <p className="stats-page__loading">{t.stats.loading}</p>
      ) : (
        <>
          <div className="stats-summary">
            <div className="stats-summary__card">
              <span className="stats-summary__value">{stats.length}</span>
              <span className="stats-summary__label">Hosts</span>
            </div>
            <div className="stats-summary__card">
              <span className="stats-summary__value">{done30}</span>
              <span className="stats-summary__label">{t.stats.updates30d}</span>
            </div>
            <div className="stats-summary__card">
              <span className="stats-summary__value">{totalDone}</span>
              <span className="stats-summary__label">{t.stats.updatesTotal}</span>
            </div>
          </div>

          <div className="stats-toolbar">
            <input
              className="stats-filter"
              type="search"
              placeholder={t.stats.filterPlaceholder}
              value={filter}
              onChange={e => setFilter(e.target.value)}
            />
          </div>

          {filtered.length === 0 ? (
            <p className="stats-page__empty">{filter ? t.stats.noMatch : t.stats.noData}</p>
          ) : (
            <table className="stats-table">
              <thead>
                <tr>
                  <th>{t.stats.host}</th>
                  <th>{t.stats.lastUpdate}</th>
                  <th className="stats-table__num">{t.stats.updates30d}</th>
                  <th className="stats-table__num">{t.stats.updatesTotal}</th>
                  <th>{t.stats.topConnector}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(row => (
                  <tr key={row.hostname} className="stats-table__row" onClick={() => onSelectHost(row.hostname)}>
                    <td>
                      <div className="stats-table__host">
                        <span className="stats-table__hostname">{row.hostname}</span>
                        {row.ip_address && <span className="stats-table__ip">{row.ip_address}</span>}
                      </div>
                    </td>
                    <td className="stats-table__date">
                      {row.last_update ? timeAgo(row.last_update) : <span className="stats-table__never">{t.stats.never}</span>}
                    </td>
                    <td className="stats-table__num">{row.done_30days > 0 ? row.done_30days : '—'}</td>
                    <td className="stats-table__num">{row.total_done > 0 ? row.total_done : '—'}</td>
                    <td className="stats-table__top">{row.top_connector || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  );
}

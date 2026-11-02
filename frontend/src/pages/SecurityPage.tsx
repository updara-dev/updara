import { useEffect, useState } from 'react';
import { fetchSecurity, runSecurityCheck } from '../api/client';
import type { SecurityFinding, SecurityOverview } from '../api/client';
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

const runtimeNames: Record<string, string> = { node: 'Node.js', python3: 'Python', php: 'PHP' };

interface Props {
  onSelectHost: (hostname: string) => void;
}

export function SecurityPage({ onSelectHost }: Props) {
  const { t } = useT();
  const [data, setData] = useState<SecurityOverview | null>(null);
  const [checking, setChecking] = useState(false);
  const [showDistro, setShowDistro] = useState(false);

  useEffect(() => {
    fetchSecurity().then(setData).catch(() => {});
  }, []);

  const handleCheck = async () => {
    setChecking(true);
    try {
      setData(await runSecurityCheck());
    } finally {
      setChecking(false);
    }
  };

  const severity = (f: SecurityFinding) => {
    if (f.kev && f.kind === 'kev') return { cls: 'kev', label: t.security.kindPossible };
    if (f.kev) return { cls: 'kev', label: t.security.kindKev };
    if (f.kind === 'security') return { cls: 'security', label: t.security.kindSecurity };
    return { cls: 'eol', label: t.security.kindEol };
  };

  if (data === null) {
    return <div className="stats-page"><p className="stats-page__loading">{t.security.loading}</p></div>;
  }

  const relevant = data.findings.filter(f => !f.distro);
  const distroCount = data.findings.length - relevant.length;
  const shown = showDistro ? data.findings : relevant;

  return (
    <div className="stats-page">
      <div className="stats-page__header security-page__header">
        <div>
          <h2 className="stats-page__title">{t.security.title}</h2>
          <p className="stats-page__subtitle">{t.security.subtitle}</p>
        </div>
        <button className="btn-secondary" onClick={handleCheck} disabled={checking}>
          {checking ? t.security.checking : t.security.checkNow}
        </button>
      </div>

      {!data.enabled && <p className="security-page__notice">{t.security.disabled}</p>}

      <div className="stats-summary">
        <div className="stats-summary__card">
          <span className="stats-summary__value">{relevant.length}</span>
          <span className="stats-summary__label">{t.security.open}</span>
        </div>
        <div className="stats-summary__card">
          <span className={`stats-summary__value ${relevant.some(f => f.kev) ? 'security-page__value--critical' : ''}`}>
            {relevant.filter(f => f.kev).length}
          </span>
          <span className="stats-summary__label">{t.security.exploited}</span>
        </div>
        <div className="stats-summary__card">
          <span className="stats-summary__value">{relevant.filter(f => f.kind === 'eol').length}</span>
          <span className="stats-summary__label">{t.security.eol}</span>
        </div>
        <div className="stats-summary__card">
          <span className="stats-summary__value">{data.hosts}</span>
          <span className="stats-summary__label">{t.security.hostsScanned}</span>
        </div>
      </div>

      <div className="security-feeds">
        <span className="security-feeds__label">{t.security.feedsLabel}:</span>
        {data.feeds.map(feed => (
          <span key={feed.name} className={`security-feeds__item ${feed.error ? 'security-feeds__item--error' : ''}`}
            title={feed.error ?? ''}>
            {feed.error ? '✖' : '✔'} {feed.name}
            {!feed.error && feed.fetched_at && !feed.fetched_at.startsWith('0001') &&
              ` · ${t.security.feedEntries(feed.entries)} · ${timeAgo(feed.fetched_at)}`}
          </span>
        ))}
        {data.last_run && <span className="security-feeds__item">{t.security.lastRun(timeAgo(data.last_run))}</span>}
      </div>

      {data.hosts === 0 ? (
        <p className="stats-page__empty">{t.security.noInventory}</p>
      ) : (
        <>
          {distroCount > 0 && (
            <div className="stats-toolbar">
              <label className="settings-toggle">
                <input type="checkbox" checked={showDistro} onChange={e => setShowDistro(e.target.checked)} />
                <span>{t.security.showDistro(distroCount)}</span>
              </label>
              {showDistro && <p className="settings-section__hint">{t.security.distroHint}</p>}
            </div>
          )}
          {shown.length === 0 ? (
            <p className="stats-page__empty">{t.security.noFindings}</p>
          ) : (
            <table className="stats-table">
              <thead>
                <tr>
                  <th>{t.security.colSeverity}</th>
                  <th>{t.security.colHost}</th>
                  <th>{t.security.colRuntime}</th>
                  <th>{t.security.colFix}</th>
                  <th>{t.security.colDetail}</th>
                  <th>{t.security.colSince}</th>
                </tr>
              </thead>
              <tbody>
                {shown.map(f => {
                  const sev = severity(f);
                  return (
                    <tr key={`${f.host_id}|${f.runtime}@${f.location}|${f.kind}`}
                      className={`stats-table__row ${f.distro ? 'security-row--distro' : ''}`}
                      onClick={() => onSelectHost(f.host_id)}>
                      <td>
                        <span className={`security-badge security-badge--${sev.cls}`}>{sev.label}</span>
                      </td>
                      <td>
                        <div className="stats-table__host">
                          <span className="stats-table__hostname">{f.host_name}</span>
                          <span className="stats-table__ip">{f.location === 'host' ? t.security.onHost : f.location}</span>
                        </div>
                      </td>
                      <td>
                        {runtimeNames[f.runtime] ?? f.runtime} {f.installed}
                        {f.distro && <span className="security-row__distro"> · {t.security.distro}</span>}
                      </td>
                      <td>{f.fixed || '—'}</td>
                      <td className="security-row__detail">
                        {f.detail}
                        {(f.cves ?? []).map(cve => (
                          <a key={cve} className="security-row__cve" href={`https://nvd.nist.gov/vuln/detail/${cve}`}
                            target="_blank" rel="noopener noreferrer" onClick={e => e.stopPropagation()}>
                            {cve}
                          </a>
                        ))}
                      </td>
                      <td className="stats-table__date">{timeAgo(f.first_seen)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  );
}

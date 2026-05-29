import type { CheckResult } from '../types';

interface Props {
  result: CheckResult;
}

const STATUS_ICON: Record<string, string> = {
  error: '❌',
  update: '⚠️',
  ok: '✅',
};

export function CheckRow({ result }: Props) {
  const state = result.error ? 'error' : result.update_available ? 'update' : 'ok';
  const icon = STATUS_ICON[state];

  const valueLabel = Object.entries(result.values ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join(' · ');

  return (
    <div className={`check-row check-row--${state}`}>
      <span className="check-row__icon">{icon}</span>
      <div className="check-row__body">
        <span className="check-row__name">{result.display_name || result.connector}</span>
        {result.error && (
          <span className="check-row__detail check-row__detail--error">{result.error}</span>
        )}
        {!result.error && valueLabel && (
          <span className="check-row__detail">{valueLabel}</span>
        )}
      </div>
      {result.update_available && result.changelog && (
        <a
          className="check-row__changelog"
          href={result.changelog}
          target="_blank"
          rel="noopener noreferrer"
        >
          Changelog ↗
        </a>
      )}
    </div>
  );
}

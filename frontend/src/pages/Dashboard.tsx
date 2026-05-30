import type { HostStatus } from '../types';
import { HostCard } from '../components/HostCard';
import { useT } from '../i18n';

interface Props {
  hosts: HostStatus[];
  loading: boolean;
  error: string | null;
}

export function Dashboard({ hosts, loading, error }: Props) {
  const { t } = useT();

  if (loading && hosts.length === 0) {
    return <p className="status-msg">{t.dashboard.loading}</p>;
  }
  if (error) {
    return <p className="status-msg status-msg--error">{t.dashboard.error(error)}</p>;
  }
  if (hosts.length === 0) {
    return (
      <div className="status-msg">
        <p>{t.dashboard.noHosts}</p>
        <p>{t.dashboard.noHostsHint}</p>
      </div>
    );
  }
  return (
    <div className="host-grid">
      {hosts.map(h => <HostCard key={h.host.id} status={h} />)}
    </div>
  );
}

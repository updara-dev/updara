import type { HostStatus } from '../types';
import { HostCard } from '../components/HostCard';

interface Props {
  hosts: HostStatus[];
  loading: boolean;
  error: string | null;
}

export function Dashboard({ hosts, loading, error }: Props) {
  if (loading && hosts.length === 0) {
    return <p className="status-msg">Loading…</p>;
  }
  if (error) {
    return <p className="status-msg status-msg--error">Error: {error}</p>;
  }
  if (hosts.length === 0) {
    return (
      <div className="status-msg">
        <p>No hosts reporting yet.</p>
        <p>Deploy an agent and point it to this server to get started.</p>
      </div>
    );
  }
  return (
    <div className="host-grid">
      {hosts.map(h => <HostCard key={h.host.id} status={h} />)}
    </div>
  );
}

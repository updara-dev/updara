import { useState } from 'react';
import { useT } from '../i18n';

const BASE = import.meta.env.VITE_SERVER_URL ?? '';

export function LoginPage({ onLogin }: { onLogin: () => void }) {
  const { t } = useT();
  const [token, setToken] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      const res = await fetch(`${BASE}/api/v1/hosts`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        localStorage.setItem('updara_token', token);
        onLogin();
      } else {
        setError(t.login.invalidToken);
      }
    } catch {
      setError(t.login.connectionError);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <div className="login-card">
        <img src="/updara_logo.svg" alt="Updara" className="login-logo" />
        <form onSubmit={handleSubmit} className="login-form">
          <input
            type="password"
            value={token}
            onChange={e => setToken(e.target.value)}
            placeholder={t.login.tokenPlaceholder}
            className="login-input"
            autoFocus
          />
          {error && <p className="login-error">{error}</p>}
          <button type="submit" className="btn-primary login-btn" disabled={loading || !token}>
            {loading ? t.login.verifying : t.login.submit}
          </button>
        </form>
      </div>
    </div>
  );
}

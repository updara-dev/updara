import { useEffect, useState } from 'react';
import { fetchNotificationSettings, saveNotificationSettings, testNotification, testDigest } from '../api/client';
import type { NotificationSettings } from '../api/client';
import { useT } from '../i18n';

const empty: NotificationSettings = {
  ntfy_url: '', ntfy_topic: '', ntfy_enabled: false,
  telegram_token: '', telegram_chat_id: '', telegram_enabled: false,
  email_enabled: false, email_host: '', email_port: '587',
  email_username: '', email_password: '', email_from: '', email_to: '',
  email_tls: 'starttls',
  digest_enabled: false, digest_frequency: 'monthly', digest_weekday: 1,
  digest_day: 1, digest_time: '08:00',
  cooldown_days: 3, min_count: 0,
  batch_schedule: 'immediate', batch_time1: '07:00', batch_time2: '19:00',
  show_lts_upgrades: true,
};

export function SettingsPage() {
  const { t } = useT();
  const [cfg, setCfg] = useState<NotificationSettings>(empty);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [testStatus, setTestStatus] = useState<'idle' | 'testing' | 'ok' | 'error'>('idle');
  const [digestTestStatus, setDigestTestStatus] = useState<'idle' | 'testing' | 'ok' | 'error'>('idle');
  const [errMsg, setErrMsg] = useState('');

  useEffect(() => {
    fetchNotificationSettings().then(setCfg).catch(() => {});
  }, []);

  const set = (key: keyof NotificationSettings, value: string | boolean | number) =>
    setCfg(prev => ({ ...prev, [key]: value }));

  const handleSave = async () => {
    setSaveStatus('saving');
    try {
      await saveNotificationSettings(cfg);
      setSaveStatus('saved');
      setTimeout(() => setSaveStatus('idle'), 2500);
    } catch (e) {
      setErrMsg(String(e));
      setSaveStatus('error');
    }
  };

  const handleTest = async () => {
    setTestStatus('testing');
    try {
      await testNotification();
      setTestStatus('ok');
      setTimeout(() => setTestStatus('idle'), 2500);
    } catch (e) {
      setErrMsg(String(e));
      setTestStatus('error');
    }
  };

  const anyEnabled = cfg.ntfy_enabled || cfg.telegram_enabled || cfg.email_enabled;

  const handleTestDigest = async () => {
    setDigestTestStatus('testing');
    try {
      await testDigest();
      setDigestTestStatus('ok');
      setTimeout(() => setDigestTestStatus('idle'), 3000);
    } catch (e) {
      setErrMsg(String(e));
      setDigestTestStatus('error');
    }
  };

  return (
    <div className="settings-page">
      <h2 className="settings-page__title">{t.settings.title}</h2>

      <div className="settings-section">
        <div className="settings-section__header">
          <h3>{t.settings.ntfySection}</h3>
          <label className="settings-toggle">
            <input type="checkbox" checked={cfg.ntfy_enabled} onChange={e => set('ntfy_enabled', e.target.checked)} />
            <span>{t.settings.enabled}</span>
          </label>
        </div>
        <p className="settings-section__hint">{t.settings.ntfyHint}</p>
        <div className="settings-fields">
          <label className="settings-field">
            <span>{t.settings.ntfyUrl}</span>
            <input className="settings-input" value={cfg.ntfy_url} onChange={e => set('ntfy_url', e.target.value)} placeholder="http://10.0.0.247" />
          </label>
          <label className="settings-field">
            <span>{t.settings.ntfyTopic}</span>
            <input className="settings-input" value={cfg.ntfy_topic} onChange={e => set('ntfy_topic', e.target.value)} placeholder="updara" />
          </label>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section__header">
          <h3>{t.settings.telegramSection}</h3>
          <label className="settings-toggle">
            <input type="checkbox" checked={cfg.telegram_enabled} onChange={e => set('telegram_enabled', e.target.checked)} />
            <span>{t.settings.enabled}</span>
          </label>
        </div>
        <p className="settings-section__hint">{t.settings.telegramHint}</p>
        <div className="settings-fields">
          <label className="settings-field">
            <span>{t.settings.telegramToken}</span>
            <input className="settings-input" value={cfg.telegram_token} onChange={e => set('telegram_token', e.target.value)} placeholder="123456:ABC-DEF..." type="password" />
          </label>
          <label className="settings-field">
            <span>{t.settings.telegramChatId}</span>
            <input className="settings-input" value={cfg.telegram_chat_id} onChange={e => set('telegram_chat_id', e.target.value)} placeholder="123456789" />
          </label>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section__header">
          <h3>{t.settings.emailSection}</h3>
          <label className="settings-toggle">
            <input type="checkbox" checked={cfg.email_enabled} onChange={e => set('email_enabled', e.target.checked)} />
            <span>{t.settings.enabled}</span>
          </label>
        </div>
        <p className="settings-section__hint">{t.settings.emailHint}</p>
        <div className="settings-fields">
          <label className="settings-field">
            <span>{t.settings.emailHost}</span>
            <input className="settings-input" value={cfg.email_host} onChange={e => set('email_host', e.target.value)} placeholder="smtp.gmail.com" />
          </label>
          <label className="settings-field">
            <span>{t.settings.emailPort}</span>
            <input className="settings-input settings-input--short" value={cfg.email_port} onChange={e => set('email_port', e.target.value)} placeholder="587" />
          </label>
          <label className="settings-field">
            <span>{t.settings.emailTLS}</span>
            <select className="settings-select" value={cfg.email_tls} onChange={e => set('email_tls', e.target.value)}>
              <option value="starttls">{t.settings.emailTLSStarttls}</option>
              <option value="ssl">{t.settings.emailTLSSsl}</option>
              <option value="none">{t.settings.emailTLSNone}</option>
            </select>
          </label>
          <label className="settings-field">
            <span>{t.settings.emailUsername}</span>
            <input className="settings-input" value={cfg.email_username} onChange={e => set('email_username', e.target.value)} placeholder="you@gmail.com" />
          </label>
          <label className="settings-field">
            <span>{t.settings.emailPassword}</span>
            <input className="settings-input" type="password" value={cfg.email_password} onChange={e => set('email_password', e.target.value)} placeholder="App password" />
          </label>
          <label className="settings-field">
            <span>{t.settings.emailFrom}</span>
            <input className="settings-input" value={cfg.email_from} onChange={e => set('email_from', e.target.value)} placeholder="updara@yourdomain.com" />
          </label>
          <label className="settings-field">
            <span>{t.settings.emailTo}</span>
            <input className="settings-input" value={cfg.email_to} onChange={e => set('email_to', e.target.value)} placeholder="you@yourdomain.com" />
          </label>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section__header">
          <h3>{t.settings.digestSection}</h3>
          <label className="settings-toggle">
            <input type="checkbox" checked={cfg.digest_enabled} onChange={e => set('digest_enabled', e.target.checked)} />
            <span>{t.settings.enabled}</span>
          </label>
        </div>
        <p className="settings-section__hint">{t.settings.digestHint}</p>
        <div className="settings-num-fields">
          <div className="settings-num-row">
            <label className="settings-num-row__label">{t.settings.digestFrequency}</label>
            <select className="settings-select" value={cfg.digest_frequency} onChange={e => set('digest_frequency', e.target.value)}>
              <option value="daily">{t.settings.digestFreqDaily}</option>
              <option value="weekly">{t.settings.digestFreqWeekly}</option>
              <option value="monthly">{t.settings.digestFreqMonthly}</option>
            </select>
          </div>
          {cfg.digest_frequency === 'weekly' && (
            <div className="settings-num-row">
              <label className="settings-num-row__label">{t.settings.digestWeekday}</label>
              <select className="settings-select" value={cfg.digest_weekday} onChange={e => set('digest_weekday', parseInt(e.target.value))}>
                {t.settings.digestWeekdays.map((day, i) => (
                  <option key={i} value={i + 1}>{day}</option>
                ))}
              </select>
            </div>
          )}
          {cfg.digest_frequency === 'monthly' && (
            <div className="settings-num-row">
              <label className="settings-num-row__label">{t.settings.digestDay}</label>
              <input className="settings-num-row__input" type="number" min={1} max={28} value={cfg.digest_day}
                onChange={e => set('digest_day', parseInt(e.target.value) || 1)} />
              <span className="settings-num-row__hint">{t.settings.digestDayHint}</span>
            </div>
          )}
          <div className="settings-num-row">
            <label className="settings-num-row__label">{t.settings.digestTime}</label>
            <input className="settings-num-row__input settings-time-input" type="time" value={cfg.digest_time}
              onChange={e => set('digest_time', e.target.value)} />
          </div>
        </div>
        <div className="settings-digest-footer">
          {digestTestStatus === 'error' && <span className="settings-status settings-status--err">{t.settings.testDigestError(errMsg)}</span>}
          {digestTestStatus === 'ok' && <span className="settings-status settings-status--ok">{t.settings.testDigestOk}</span>}
          <button className="btn-secondary" onClick={handleTestDigest}
            disabled={!cfg.email_enabled || !cfg.digest_enabled || digestTestStatus === 'testing'}>
            {digestTestStatus === 'testing' ? t.settings.testDigestSending : t.settings.testDigestBtn}
          </button>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section__header">
          <h3>{t.settings.notificationsSection}</h3>
        </div>
        <div className="settings-num-fields">
          <div className="settings-num-row">
            <label className="settings-num-row__label">{t.settings.cooldownDays}</label>
            <input className="settings-num-row__input" type="number" min={1} max={365} value={cfg.cooldown_days}
              onChange={e => set('cooldown_days', parseInt(e.target.value) || 3)} />
            <span className="settings-num-row__hint">{t.settings.cooldownDaysHint}</span>
          </div>
          <div className="settings-num-row">
            <label className="settings-num-row__label">{t.settings.minCount}</label>
            <input className="settings-num-row__input" type="number" min={0} max={999} value={cfg.min_count}
              onChange={e => set('min_count', parseInt(e.target.value) || 0)} />
            <span className="settings-num-row__hint">{t.settings.minCountHint}</span>
          </div>
          <div className="settings-num-row">
            <label className="settings-num-row__label">{t.settings.showLtsUpgrades}</label>
            <input type="checkbox" checked={cfg.show_lts_upgrades}
              onChange={e => set('show_lts_upgrades', e.target.checked)} />
            <span className="settings-num-row__hint">{t.settings.showLtsUpgradesHint}</span>
          </div>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section__header">
          <h3>{t.settings.scheduleSection}</h3>
        </div>
        <p className="settings-section__hint">{t.settings.scheduleHint}</p>
        <div className="settings-num-fields">
          <div className="settings-num-row">
            <label className="settings-num-row__label">{t.settings.scheduleLabel}</label>
            <select className="settings-select" value={cfg.batch_schedule} onChange={e => set('batch_schedule', e.target.value)}>
              <option value="immediate">{t.settings.scheduleImmediate}</option>
              <option value="hourly">{t.settings.scheduleHourly}</option>
              <option value="daily">{t.settings.scheduleDaily}</option>
              <option value="twice_daily">{t.settings.scheduleTwiceDaily}</option>
            </select>
          </div>
          {(cfg.batch_schedule === 'daily' || cfg.batch_schedule === 'twice_daily') && (
            <div className="settings-num-row">
              <label className="settings-num-row__label">{t.settings.scheduleTime1}</label>
              <input className="settings-num-row__input settings-time-input" type="time" value={cfg.batch_time1}
                onChange={e => set('batch_time1', e.target.value)} />
            </div>
          )}
          {cfg.batch_schedule === 'twice_daily' && (
            <div className="settings-num-row">
              <label className="settings-num-row__label">{t.settings.scheduleTime2}</label>
              <input className="settings-num-row__input settings-time-input" type="time" value={cfg.batch_time2}
                onChange={e => set('batch_time2', e.target.value)} />
            </div>
          )}
        </div>
      </div>

      <div className="settings-page__footer">
        {saveStatus === 'error' && <span className="settings-status settings-status--err">{t.settings.saveError(errMsg)}</span>}
        {testStatus === 'error' && <span className="settings-status settings-status--err">{t.settings.testError(errMsg)}</span>}
        {saveStatus === 'saved' && <span className="settings-status settings-status--ok">{t.settings.saved}</span>}
        {testStatus === 'ok'    && <span className="settings-status settings-status--ok">{t.settings.testOk}</span>}
        <button className="btn-secondary" onClick={handleTest} disabled={!anyEnabled || testStatus === 'testing'}>
          {testStatus === 'testing' ? t.settings.testing : t.settings.testBtn}
        </button>
        <button className="btn-primary" onClick={handleSave} disabled={saveStatus === 'saving'}>
          {saveStatus === 'saving' ? t.settings.saving : t.settings.save}
        </button>
      </div>
    </div>
  );
}

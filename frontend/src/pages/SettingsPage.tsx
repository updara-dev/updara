import { useEffect, useState } from 'react';
import { fetchNotificationSettings, saveNotificationSettings, testNotification } from '../api/client';
import type { NotificationSettings } from '../api/client';
import { useT } from '../i18n';

const empty: NotificationSettings = {
  ntfy_url: '', ntfy_topic: '', ntfy_enabled: false,
  telegram_token: '', telegram_chat_id: '', telegram_enabled: false,
  cooldown_days: 3, min_count: 0,
  batch_schedule: 'immediate', batch_time1: '07:00', batch_time2: '19:00',
  show_lts_upgrades: true,
};

export function SettingsPage() {
  const { t } = useT();
  const [cfg, setCfg] = useState<NotificationSettings>(empty);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [testStatus, setTestStatus] = useState<'idle' | 'testing' | 'ok' | 'error'>('idle');
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

  const anyEnabled = cfg.ntfy_enabled || cfg.telegram_enabled;

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

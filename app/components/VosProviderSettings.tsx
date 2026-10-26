import React, { useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { useI18n } from '../context/I18nContext';
import { assistantStatusChanged, karaokeStatusChanged, openRouterSettingsChanged } from '../services/studioQueries';

type Capability = 'prompt_enhancement' | 'speech_to_text' | 'cover_art';
type Selection = { capability: string; mode: 'local' | 'open_router'; local_engine: string | null; cloud_model: string | null };
type Model = { id: string; name: string; capabilities: Capability[] };
export function VosProviderSettings() {
  const { language } = useI18n(); const zh = language === 'zh';
  const [models, setModels] = useState<Model[]>([]);
  const [selections, setSelections] = useState<Selection[]>([]);
  const [busy, setBusy] = useState(false); const [error, setError] = useState('');
  async function json(path: string, init?: RequestInit) {
    const r = await fetch(path, init); const value = await r.json();
    if (!r.ok) throw new Error(value.error || `V-Scheduler: ${r.status}`);
    return value;
  }
  async function load(refresh = false) {
    setBusy(true); setError('');
    try {
      const [catalog, config] = await Promise.all([
        json(`/v1/provider/catalog${refresh ? '/refresh' : ''}`, refresh ? { method: 'POST' } : undefined), json('/v1/configuration'),
      ]);
      setModels(catalog.models ?? []); setSelections(config.selections ?? []);
    } catch (e) { setError(String(e)); } finally { setBusy(false); }
  }
  useEffect(() => { void load(); }, []);
  async function save(capability: Capability, model: string) {
    setBusy(true); setError('');
    try {
      const selection: Selection = { capability, mode: 'open_router', cloud_model: model || null, local_engine: null };
      const config = await json('/v1/configuration', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ selections: [selection] }) });
      setSelections(config.selections ?? []); assistantStatusChanged(); karaokeStatusChanged(); openRouterSettingsChanged();
    } catch (e) { setError(String(e)); } finally { setBusy(false); }
  }
  return <section className="space-y-5">
    <div className="rounded-xl border border-zinc-200 p-5 space-y-3 dark:border-white/10">
      <h2 className="text-lg font-semibold">V-Scheduler</h2>
      <p className="text-sm text-zinc-500">{zh ? '在 V-Scheduler 中配置供应商、密钥和上游。这里按各项能力选择当前账户可用的模型，调用统一经过 V-Scheduler。' : 'Configure providers, credentials and upstreams in V-Scheduler. Select models available to your account for each capability here; requests go through V-Scheduler.'}</p>
      <button disabled={busy} onClick={() => void load(true)} className="inline-flex items-center gap-2 rounded-lg border border-zinc-300 px-3 py-2 text-sm disabled:opacity-50 dark:border-zinc-700"><RefreshCw size={16} className={busy ? 'animate-spin' : ''} />{zh ? '刷新模型' : 'Refresh models'}</button>
    </div>
    <div className="rounded-xl border border-zinc-200 p-5 space-y-2 dark:border-white/10"><h3 className="font-medium">{zh ? '音乐生成' : 'Music generation'}</h3><p className="text-sm text-zinc-500">{zh ? 'YuE2 · 本机运行，模型由 V-Scheduler 管理。' : 'YuE2 · local inference with models managed by V-Scheduler.'}</p></div>
    {([['prompt_enhancement', zh ? '写作助手' : 'Writing assistant'], ['speech_to_text', zh ? '语音识别' : 'Speech recognition'], ['cover_art', zh ? '封面生成' : 'Cover generation']] as const).map(([capability, title]) => {
      const eligible = models.filter(m => m.capabilities.includes(capability));
      const selection = selections.find(s => s.capability === capability); const selected = selection?.mode === 'open_router' ? selection.cloud_model || '' : '';
      return <div key={capability} className="rounded-xl border border-zinc-200 p-5 space-y-3 dark:border-white/10">
        <label className="block font-medium" htmlFor={`provider-${capability}`}>{title}</label>
        <select id={`provider-${capability}`} disabled={busy} value={selected} onChange={e => void save(capability, e.target.value)} className="w-full rounded-lg border border-zinc-300 bg-transparent px-3 py-2 text-sm dark:border-zinc-700">
          <option value="">{zh ? '未配置' : 'Not configured'}</option>
          {selected && !eligible.some(m => m.id === selected) && <option value={selected}>{selected} · {zh ? '当前不可用，请重新选择' : 'Unavailable; select another model'}</option>}
          {eligible.map(m => <option key={m.id} value={m.id}>{m.name}</option>)}
        </select>
        {!eligible.length && <p className="text-sm text-zinc-500">{zh ? 'V-Scheduler 暂无声明支持此能力和接口的模型。请在 V-Scheduler 中配置后刷新。' : 'V-Scheduler has no models declaring this capability and API. Configure one in V-Scheduler, then refresh.'}</p>}
      </div>;
    })}
    {error && <p role="alert" className="text-sm text-rose-500 break-words">{error}</p>}
  </section>;
}

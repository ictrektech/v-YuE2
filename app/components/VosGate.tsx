import React, { useEffect, useState } from 'react';
import { isVos, vosBase, vosRequest } from '../services/vos';
import { useI18n } from '../context/I18nContext';
import { setStudioAccount } from '../services/storage';

export function VosGate({ children }: { children: React.ReactNode }) {
  const { language } = useI18n();
  const zh = language === 'zh';
  const [state, setState] = useState<{ accepted: boolean; revision: string }>();
  const [checked, setChecked] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  async function connect() {
    setError(''); setBusy(true);
    try {
      const res = await vosRequest('/vos/session', { method: 'POST' });
      if (!res.ok) throw new Error(await res.text());
      const session = await res.json();
      setStudioAccount(session.sub);
      setState(session);
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }
  useEffect(() => { if (isVos) void connect(); }, []);
  if (!isVos || state?.accepted) return <>{children}</>;
  async function accept() {
    if (!checked || !state) return;
    setBusy(true); setError('');
    try {
      const res = await vosRequest('/vos/license', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ accepted: true, revision: state.revision }) });
      if (!res.ok) throw new Error(await res.text());
      setState({ ...state, accepted: true });
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }
  return <main className="min-h-screen bg-zinc-950 text-zinc-100 flex items-center justify-center p-6">
    <section className="w-full max-w-xl rounded-2xl border border-zinc-800 bg-zinc-900 p-8 space-y-5">
      <img src={`${vosBase}/brand/yue2.png`} alt="" className="h-14 w-14 rounded-xl" />
      <h1 className="text-2xl font-semibold">v-YuE2</h1>
      {!state ? <><p>{zh ? '正在连接 VOS 账户…' : 'Connecting to your VOS account…'}</p>{!busy && <button onClick={() => void connect()}>{zh ? '重新连接' : 'Reconnect'}</button>}</> : <>
        <h2 className="text-lg font-medium">{zh ? '开始前，请了解模型许可' : 'Before you start: model licenses'}</h2>
        <p className="text-zinc-300">{zh ? '应用免费提供，安装包不含模型。只有在你选择下载后，V-Scheduler 才会从 vivibit 的 ModelScope 仓库下载模型到本机；生成在本机运行。' : 'The app is free and contains no model weights. V-Scheduler downloads your selected models from vivibit on ModelScope only when requested. Generation runs locally.'}</p>
        <p className="text-zinc-300">{zh ? 'YuE2 主模型和 VAE 使用 CC BY-NC 4.0，并有个人创作者作品变现特许；企业商业使用权重需另行授权。SheetSage2、MERT 和 companion 等附加模型不自动享有该特许。' : 'YuE2 backbone and VAE use CC BY-NC 4.0 with an individual-creator output monetization permission. Company commercial use requires separate authorization. Additional models such as SheetSage2, MERT and the companion do not automatically receive that permission.'}</p>
        <div className="flex flex-wrap gap-4 text-pink-300 underline"><a href={`${vosBase}/licenses/YuE2-MODEL_LICENSE.txt`} target="_blank" rel="noreferrer">{zh ? '模型原始许可' : 'Original model license'}</a><a href={`${vosBase}/THIRD_PARTY_NOTICES.md`} target="_blank" rel="noreferrer">{zh ? '第三方许可' : 'Third-party notices'}</a></div>
        <label className="flex gap-3 items-start"><input type="checkbox" className="mt-1" checked={checked} onChange={e => setChecked(e.target.checked)} /><span>{zh ? '我已阅读许可，并会按各模型允许的范围使用。' : 'I have read the licenses and will use each model within its permitted scope.'}</span></label>
        <button disabled={!checked || busy} onClick={() => void accept()} className="rounded-lg bg-pink-600 px-5 py-2 font-medium disabled:opacity-40">{zh ? '进入工作台' : 'Open studio'}</button>
      </>}
      {error && <p role="alert" className="text-red-300 break-words">{error}</p>}
    </section>
  </main>;
}

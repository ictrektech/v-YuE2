import React, { useEffect, useState } from 'react';
import { useI18n } from '../context/I18nContext';
import { vosBase } from '../services/vos';

export function VosAbout() {
  const { language } = useI18n();
  const zh = language === 'zh';
  const [info, setInfo] = useState<{ version: string; profile: string; image: string }>();
  useEffect(() => { void fetch('/vos/about').then(r => r.ok ? r.json() : undefined).then(setInfo).catch(() => {}); }, []);
  return <section className="space-y-5 text-sm text-zinc-600 dark:text-zinc-300">
    <h2 className="text-xl font-semibold text-zinc-900 dark:text-white">v-YuE2</h2>
    <p>{zh ? '免费本地歌曲创作与可编辑乐谱工作台。' : 'A free local song studio with editable scores.'}</p>
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-2">
      <dt>{zh ? '应用版本' : 'App version'}</dt><dd>{info?.version ?? __APP_VERSION__}</dd>
      <dt>{zh ? '安装平台' : 'Installation profile'}</dt><dd>{info?.profile ?? '—'}</dd>
      <dt>{zh ? '服务镜像' : 'Service image'}</dt><dd className="break-all">{info?.image?.split('/').at(-1) ?? '—'}</dd>
    </dl>
    <p>{zh ? '安装包和服务镜像不含模型权重。模型由用户点击“准备模型”后，通过 V-Scheduler 从 vivibit / ModelScope 下载到本机。' : 'No model weights are bundled. Select Prepare models to download the required files locally through V-Scheduler from vivibit / ModelScope.'}</p>
    <p>{zh ? '程序与模型分别遵守其许可。个人创作者特许不自动覆盖企业商用及附加模型。' : 'Program and model licenses apply separately. Individual-creator permission does not automatically cover company commercial use or additional models.'}</p>
    <div className="flex flex-wrap gap-4 text-pink-600 underline dark:text-pink-300">
      <a href="https://github.com/ictrektech/v-YuE2" target="_blank" rel="noreferrer">{zh ? '项目源码' : 'Source code'}</a>
      <a href={`${vosBase}/THIRD_PARTY_NOTICES.md`} target="_blank" rel="noreferrer">{zh ? '第三方许可' : 'Third-party notices'}</a>
      <a href={`${vosBase}/licenses/YuE2-MODEL_LICENSE.txt`} target="_blank" rel="noreferrer">{zh ? '模型原始许可' : 'Original model license'}</a>
    </div>
    <p>{zh ? '基于 YuE2 Studio 和 yue2.cpp，保留原作者署名与许可。' : 'Based on YuE2 Studio and yue2.cpp, retaining original author attribution and licenses.'}</p>
  </section>;
}

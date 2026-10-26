import { studioStorage } from '../services/storage';
import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { isVos } from '../services/vos';
import { translations, Language, TranslationKey } from '../i18n/translations';

interface I18nContextType {
  language: Language;
  setLanguage: (lang: Language) => void;
  t: (key: TranslationKey) => string;
  /** "1 song", "3 песни", "61 песня": the form each language asks for. */
  songCount: (count: number) => string;
}

// One context object for the life of the page: when a strings file changes in
// development, this module is evaluated again, and a second context would leave
// every consumer outside the provider that is already mounted.
const I18nContext: React.Context<I18nContextType | undefined> =
  import.meta.hot?.data.i18nContext ?? createContext<I18nContextType | undefined>(undefined);
if (import.meta.hot) import.meta.hot.data.i18nContext = I18nContext;

export const I18nProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [language, setLanguage] = useState<Language>(() => {
    const stored = studioStorage.getItem('language') as Language;
    if (stored === 'zh' || stored === 'en' || stored === 'ja' || stored === 'ko' || stored === 'ru') return stored;
    if (typeof navigator !== 'undefined' && navigator.language?.startsWith('ru')) return 'ru';
    return 'en';
  });

  const handleSetLanguage = (lang: Language) => {
    setLanguage(lang);
    studioStorage.setItem('language', lang);
  };

  useEffect(() => {
    if (!isVos) return;
    let stop: (() => void) | undefined;
    let stopUser: (() => void) | undefined;
    let disposed = false;
    let revision = 0;
    const accept = (state: { locale?: unknown }) => {
      const locale = String(state.locale ?? 'en').toLowerCase().split('-')[0];
      setLanguage((['zh', 'en', 'ja', 'ko', 'ru'].includes(locale) ? locale : 'en') as Language);
    };
    const timer = setInterval(() => {
      const bridge = window.vos_platform;
      if (!bridge) return;
      clearInterval(timer);
      stop = bridge.broadcast?.on('locale-change', state => { revision++; accept(state); });
      stopUser = bridge.broadcast?.on('user-change', () => window.location.reload());
      const before = revision;
      void bridge.getState?.().then(state => { if (!disposed && revision === before) accept(state); }).catch(() => {});
    }, 50);
    return () => { disposed = true; clearInterval(timer); stop?.(); stopUser?.(); };
  }, []);

  const t = (key: TranslationKey): string => {
    const value = translations[language][key] || key;
    if (!isVos) return value;
    const hints: Partial<Record<TranslationKey, string>> = {
      assistantOpenRouterHint: language === 'zh' ? '使用 V-Scheduler 中配置的写作模型。' : 'Use a writing model configured in V-Scheduler.',
      karaokeOpenRouterHint: language === 'zh' ? '使用 V-Scheduler 中支持转写及时间戳的模型。' : 'Use a V-Scheduler transcription model with timestamps.',
    };
    return hints[key] ?? value.replace(/OpenRouter/g, 'V-Scheduler');
  };

  const songCount = (count: number): string =>
    t(`songCount_${new Intl.PluralRules(language).select(count)}` as TranslationKey).replace('{count}', String(count));

  return (
    <I18nContext.Provider value={{ language, setLanguage: handleSetLanguage, t, songCount }}>
      {children}
    </I18nContext.Provider>
  );
};

export const useI18n = () => {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error('useI18n must be used within I18nProvider');
  }
  return context;
};

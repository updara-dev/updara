import { createContext, useContext, useState } from 'react';
import type { ReactNode } from 'react';
import { en } from './en';
import { de } from './de';
import type { Translations } from './types';

export type Lang = 'en' | 'de';

const TRANSLATIONS: Record<Lang, Translations> = { en, de };

interface I18nContextValue {
  t: Translations;
  lang: Lang;
  setLang: (l: Lang) => void;
}

const I18nContext = createContext<I18nContextValue>({ t: en, lang: 'en', setLang: () => {} });

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() =>
    (localStorage.getItem('updara_lang') as Lang) ?? 'en'
  );

  const setLang = (l: Lang) => {
    localStorage.setItem('updara_lang', l);
    setLangState(l);
  };

  return (
    <I18nContext.Provider value={{ t: TRANSLATIONS[lang], lang, setLang }}>
      {children}
    </I18nContext.Provider>
  );
}

export const useT = () => useContext(I18nContext);

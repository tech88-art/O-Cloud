import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import zhCN from './zh-CN.json';
import enUS from './en-US.json';

/**
 * react-i18next setup. Default language is zh-CN per project conventions
 * (frontend/CLAUDE.md §4.4). Keys are flat-namespaced (page.<name>.<key>).
 *
 * On boot we hydrate the user's last-chosen locale from localStorage so the
 * Header dropdown choice survives reload (P1-T-106 AC). `persistLocale` is
 * the single write path — Layout calls it on dropdown change.
 */
export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];

const STORAGE_KEY = 'ocloud.locale';
const DEFAULT_LOCALE: Locale = 'zh-CN';

function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** Read locale from localStorage; fall back to default if absent or invalid. */
export function loadPersistedLocale(): Locale {
  if (typeof window === 'undefined') return DEFAULT_LOCALE;
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    return isLocale(stored) ? stored : DEFAULT_LOCALE;
  } catch {
    // localStorage can throw in strict privacy modes / SSR; fall back silently.
    return DEFAULT_LOCALE;
  }
}

/** Write locale to localStorage. Silently no-ops if storage is unavailable. */
export function persistLocale(locale: Locale): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_KEY, locale);
  } catch {
    // ignore — non-fatal
  }
}

const initialLocale = loadPersistedLocale();

void i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCN },
    'en-US': { translation: enUS },
  },
  lng: initialLocale,
  fallbackLng: DEFAULT_LOCALE,
  interpolation: { escapeValue: false },
  returnNull: false,
});

export default i18n;

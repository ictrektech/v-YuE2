import { isVos } from './vos';

let account = 'pending';
export function setStudioAccount(subject: string) { account = encodeURIComponent(subject); }
const scope = () => isVos ? `com.ictrek.v-yue2:${account}:` : '';
const keys = () => Object.keys(window.localStorage).filter(key => key.startsWith(scope()));
/** Browser drafts and preferences follow the authenticated account too. */
export const studioStorage = {
  getItem: (key: string) => window.localStorage.getItem(scope() + key),
  setItem: (key: string, value: string) => window.localStorage.setItem(scope() + key, value),
  removeItem: (key: string) => window.localStorage.removeItem(scope() + key),
  clear: () => keys().forEach(key => window.localStorage.removeItem(key)),
  key: (index: number) => keys()[index]?.slice(scope().length) ?? null,
  get length() { return keys().length; },
};

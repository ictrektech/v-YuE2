/** VOS OAuth2 PKCE bridge. Tokens stay in memory; media uses an HttpOnly session. */
import { sha256 } from '@noble/hashes/sha2.js';
export const isVos = import.meta.env.VITE_VOS === 'true';
export const vosBase = '/app/com.ictrek.v-yue2';
export const appPath = (path: string) => isVos ? vosBase + path : path;
const nativeFetch = globalThis.fetch.bind(globalThis);
type OAuth = {
  authorize: (input: Record<string, string>) => Promise<{ code: string; state: string }>;
  token: (input: Record<string, string>) => Promise<{ access_token: string; expires_in?: number }>;
};
declare global { interface Window { vos_platform?: {
  api?: { v1000?: { oauth2?: OAuth } };
  getState?: () => Promise<{ locale?: unknown }>;
  broadcast?: { on: (event: string, listener: (state: { locale?: unknown }) => void) => () => void };
} } }
let session: { token: string; expires: number } | undefined;
let pending: Promise<string> | undefined;
const encode = (v: Uint8Array) => btoa(String.fromCharCode(...v)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
async function signIn(): Promise<string> {
  let oauth: OAuth | undefined;
  for (let i = 0; i < 100; i++) {
    oauth = window.vos_platform?.api?.v1000?.oauth2;
    if (oauth) break;
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  if (!oauth) throw new Error('请从 VOS 打开 v-YuE2 / Open v-YuE2 from VOS');
  const verifier = encode(crypto.getRandomValues(new Uint8Array(32)));
  const state = encode(crypto.getRandomValues(new Uint8Array(24)));
  const challenge = encode(sha256(new TextEncoder().encode(verifier)));
  const authorization = await oauth.authorize({ client_id: 'com.ictrek.v-yue2', response_type: 'code', scope: 'openid profile email', state, nonce: encode(crypto.getRandomValues(new Uint8Array(24))), code_challenge: challenge, code_challenge_method: 'S256' });
  if (authorization.state !== state || !authorization.code) throw new Error('VOS OIDC state mismatch');
  const tokens = await oauth.token({ grant_type: 'authorization_code', client_id: 'com.ictrek.v-yue2', code: authorization.code, code_verifier: verifier });
  if (!tokens.access_token) throw new Error('VOS access token missing');
  session = { token: tokens.access_token, expires: Date.now() + Math.max(30, (tokens.expires_in ?? 3600) - 60) * 1000 };
  const connected = await nativeFetch(`${vosBase}/vos/session`, { method: 'POST', headers: { Authorization: `Bearer ${tokens.access_token}` } });
  if (!connected.ok) { session = undefined; throw new Error('VOS session unavailable / VOS 会话不可用'); }
  return session.token;
}
export async function vosToken() {
  if (session && session.expires > Date.now()) return session.token;
  pending ??= signIn().finally(() => { pending = undefined; });
  return pending;
}
export function resetVosSession() { session = undefined; }
export async function vosRequest(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  headers.set('Authorization', `Bearer ${await vosToken()}`);
  const response = await fetch(`${vosBase}${path}`, { ...init, headers });
  if (response.status === 401) resetVosSession();
  return response;
}

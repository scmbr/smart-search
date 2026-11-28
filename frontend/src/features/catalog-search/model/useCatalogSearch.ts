import { useEffect, useRef, useState } from 'react';
import { ApiError, searchCatalog, type SearchRequest, type SearchResponse } from '@/shared/api';

type State = { phase: 'idle' | 'loading' } | { phase: 'success'; data: SearchResponse } | { phase: 'error'; error: ApiError };
type Kind = NonNullable<SearchRequest['filters']>['offer_kind'];
type Pending = { body: SearchRequest; key: string };
function sessionID() {
  try {
    const existing = sessionStorage.getItem('smart-search-session');
    if (existing && /^[A-Za-z0-9_-]{1,128}$/.test(existing)) return existing;
    const id = crypto.randomUUID(); sessionStorage.setItem('smart-search-session', id); return id;
  } catch { return crypto.randomUUID(); }
}
export function useCatalogSearch() {
  const [query, setQueryValue] = useState('');
  const [availableOnly, setAvailableValue] = useState(false);
  const [kind, setKindValue] = useState<Kind>(undefined);
  const [state, setState] = useState<State>({ phase: 'idle' });
  const [session] = useState(sessionID);
  const sequence = useRef(0);
  const active = useRef<AbortController | null>(null);
  const pending = useRef<Pending | null>(null);
  const running = useRef(false);
  useEffect(() => () => { sequence.current++; active.current?.abort(); }, []);
  function invalidate() { sequence.current++; active.current?.abort(); pending.current = null; running.current = false; setState({ phase: 'idle' }); }
  function setQuery(value: string) { invalidate(); setQueryValue(value); }
  function setAvailableOnly(value: boolean) { invalidate(); setAvailableValue(value); }
  function setKind(value: Kind) { invalidate(); setKindValue(value); }
  async function execute(request: Pending) {
    active.current?.abort(); const controller = new AbortController(); active.current = controller;
    const current = ++sequence.current; pending.current = request; running.current = true; setState({ phase: 'loading' });
    try {
      const data = await searchCatalog(request.body, request.key, controller.signal);
      if (current === sequence.current && !controller.signal.aborted) setState({ phase: 'success', data });
    } catch (error) {
      if (current === sequence.current && !controller.signal.aborted) {
        setState({ phase: 'error', error: error instanceof ApiError ? error : new ApiError('Не удалось выполнить поиск.', true) });
      }
    } finally {
      if (current === sequence.current) running.current = false;
    }
  }
  async function searchFor(text: string) {
    if (!text.trim() || [...text].length > 256) return;
    setQueryValue(text);
    const body: SearchRequest = { query: text, session_id: session, limit: 4, filters: {
      availability_policy: availableOnly ? 'available_only' : 'priced_first', ...(kind ? { offer_kind: kind } : {}),
    } };
    if (running.current && pending.current && JSON.stringify(pending.current.body) === JSON.stringify(body)) return;
    return execute({ body, key: crypto.randomUUID() });
  }
  async function submit() { if (state.phase !== 'loading') await searchFor(query); }
  async function retry() { if (pending.current && state.phase === 'error') await execute(pending.current); }
  return { query, setQuery, availableOnly, setAvailableOnly, kind, setKind, state, searchFor, submit, retry };
}

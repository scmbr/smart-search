import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import { ApiError, searchCatalog, type SearchResponse } from '@/shared/api';
import { useCatalogSearch } from './useCatalogSearch';

vi.mock('@/shared/api', async (original) => ({ ...await original<typeof import('@/shared/api')>(), searchCatalog: vi.fn() }));
const api = vi.mocked(searchCatalog);
const response = (query: string) => ({ query, matches: [] }) as unknown as SearchResponse;
beforeEach(() => { api.mockReset(); sessionStorage.clear(); });
test('ignores an older response even if transport ignores abort', async () => {
  let first!: (value: SearchResponse) => void;
  let second!: (value: SearchResponse) => void;
  api.mockImplementationOnce(() => new Promise((resolve) => { first = resolve; }));
  api.mockImplementationOnce(() => new Promise((resolve) => { second = resolve; }));
  const { result } = renderHook(useCatalogSearch);
  act(() => { void result.current.searchFor('AnyDesk'); });
  act(() => { void result.current.searchFor('Office'); });
  await act(async () => { second(response('Office')); });
  await act(async () => { first(response('AnyDesk')); });
  expect(result.current.state.phase).toBe('success');
  if (result.current.state.phase === 'success') expect(result.current.state.data.query).toBe('Office');
});
test('retries an uncertain request with the same idempotency key', async () => {
  api.mockRejectedValueOnce(new ApiError('Unavailable', true)).mockResolvedValueOnce(response('AnyDesk'));
  const { result } = renderHook(useCatalogSearch);
  await act(async () => { await result.current.searchFor('AnyDesk'); });
  expect(result.current.state.phase).toBe('error');
  await act(async () => { await result.current.retry(); });
  expect(api.mock.calls[0][1]).toBe(api.mock.calls[1][1]);
  await waitFor(() => expect(result.current.state.phase).toBe('success'));
});
test('changing filters aborts and invalidates visible results without another submission', async () => {
  api.mockResolvedValueOnce(response('AnyDesk'));
  const { result } = renderHook(useCatalogSearch);
  await act(async () => { await result.current.searchFor('AnyDesk'); });
  expect(result.current.state.phase).toBe('success');
  act(() => result.current.setAvailableOnly(true));
  expect(result.current.state.phase).toBe('idle');
  expect(api).toHaveBeenCalledTimes(1);
  expect(api.mock.calls[0][2].aborted).toBe(true);
});
test('does not submit the same request twice while in flight', () => {
  api.mockImplementation(() => new Promise(() => {}));
  const { result } = renderHook(useCatalogSearch);
  act(() => { void result.current.searchFor('AnyDesk'); void result.current.searchFor('AnyDesk'); });
  expect(api).toHaveBeenCalledTimes(1);
});

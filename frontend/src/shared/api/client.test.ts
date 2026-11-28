import { afterEach, expect, test, vi } from 'vitest';
import { ApiError, searchCatalog } from './client';

afterEach(() => vi.unstubAllGlobals());
test('sends a session, idempotency key and cancellation signal to the local API', async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 'service_unavailable' }), { status: 503 }));
  vi.stubGlobal('fetch', fetcher);
  const signal = new AbortController().signal;
  await expect(searchCatalog({ query: 'AnyDesk', session_id: 'session', limit: 4 }, 'request', signal)).rejects.toBeInstanceOf(ApiError);
  expect(fetcher).toHaveBeenCalledWith('/api/v1/search', expect.objectContaining({
    signal, method: 'POST', headers: expect.objectContaining({ 'Idempotency-Key': 'request' }),
  }));
});
test('does not expose arbitrary server error text', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ message: 'secret DSN' }), { status: 503 })));
  try { await searchCatalog({ query: 'x', session_id: 's', limit: 4 }, 'k', new AbortController().signal); }
  catch (error) { expect(String(error)).not.toContain('secret'); }
});
test('accepts catalog response but rejects zero known prices and unsafe product links', async () => {
  const card = { product_id: 'p', family_id: 'f', name: 'Product', family_confirmed: true,
    selected_offer: { name: 'License', available: true, url: 'https://store.softline.ru/vendor/offer/', price: { currency: 'RUB', status: 'known', amount_minor: 100 } } };
  const body = { search_id: 's', query: 'Product', matches: [card], top4: [], clarification: [], degraded_reasons: [], mode: 'lexical', total_matches: null };
  const fetcher = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body))));
  vi.stubGlobal('fetch', fetcher);
  const request = { query: 'Product', session_id: 's', limit: 4 };
  await expect(searchCatalog(request, 'k', new AbortController().signal)).resolves.toHaveProperty('query', 'Product');
  card.selected_offer.price.amount_minor = 0;
  await expect(searchCatalog(request, 'k', new AbortController().signal)).rejects.toBeInstanceOf(ApiError);
  card.selected_offer.price.amount_minor = 100;
  card.selected_offer.url = 'javascript:alert(1)';
  await expect(searchCatalog(request, 'k', new AbortController().signal)).rejects.toBeInstanceOf(ApiError);
});

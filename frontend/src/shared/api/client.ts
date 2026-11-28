import type { components } from './generated/schema';

export type SearchRequest = components['schemas']['SearchRequest'];
export type SearchResponse = components['schemas']['SearchResponse'];
export type ProductCard = components['schemas']['Card'];

export class ApiError extends Error {
  constructor(message: string, public readonly retriable: boolean) { super(message); this.name = 'ApiError'; }
}
function validResponse(value: unknown): value is SearchResponse {
  if (!value || typeof value !== 'object') return false;
  const data = value as Partial<SearchResponse>;
  if (typeof data.search_id !== 'string' || typeof data.query !== 'string' ||
      !Array.isArray(data.matches) || !Array.isArray(data.top4) ||
      !Array.isArray(data.clarification) || !Array.isArray(data.degraded_reasons) ||
      !['lexical', 'hybrid'].includes(data.mode ?? '') ||
      !(data.total_matches === null || (typeof data.total_matches === 'number' && Number.isSafeInteger(data.total_matches) && data.total_matches >= 0)) ||
      data.matches.length > 20 || data.top4.length > 4 || !data.clarification.every((text) => typeof text === 'string')) return false;
  return data.matches.every((card) => {
    if (!card || typeof card !== 'object') return false;
    const offer = card.selected_offer;
    if (typeof card.product_id !== 'string' || typeof card.family_id !== 'string' || typeof card.name !== 'string' || typeof card.family_confirmed !== 'boolean' || !offer ||
        typeof offer.name !== 'string' || typeof offer.available !== 'boolean' || !offer.price) return false;
    try { const url = new URL(offer.url); if (url.protocol !== 'https:' || url.host !== 'store.softline.ru' || url.username || url.password) return false; }
    catch { return false; }
    const price = offer.price;
    return price.currency === 'RUB' && (price.status === 'unknown' ? price.amount_minor === null :
      price.status === 'known' && Number.isSafeInteger(price.amount_minor) && price.amount_minor > 0);
  });
}
export async function searchCatalog(body: SearchRequest, key: string, signal: AbortSignal): Promise<SearchResponse> {
  let response: Response;
  try {
    response = await fetch('/api/v1/search', {
      method: 'POST', signal, headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
      body: JSON.stringify(body),
    });
  } catch (error) {
    if (signal.aborted) throw error;
    throw new ApiError('Нет связи с сервером. Проверьте подключение и повторите поиск.', true);
  }
  if (!response.ok) {
    if (response.status === 400) throw new ApiError('Не удалось разобрать запрос или фильтры. Уточните формулировку.', false);
    if (response.status === 409) throw new ApiError('Запрос изменился. Отправьте поиск заново.', false);
    if (response.status === 413) throw new ApiError('Запрос слишком большой. Сократите его.', false);
    throw new ApiError('Поиск временно недоступен. Попробуйте ещё раз.', true);
  }
  let data: unknown;
  try { data = await response.json(); } catch { throw new ApiError('Не удалось прочитать ответ поиска.', true); }
  if (!validResponse(data)) throw new ApiError('Сервер вернул некорректный результат. Повторите поиск.', true);
  return data;
}

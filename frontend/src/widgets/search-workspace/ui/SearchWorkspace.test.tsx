import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { ApiError, searchCatalog, type SearchResponse } from '@/shared/api';
import { SearchWorkspace } from './SearchWorkspace';

vi.mock('@/shared/api', async (original) => ({ ...await original<typeof import('@/shared/api')>(), searchCatalog: vi.fn() }));
const api = vi.mocked(searchCatalog);
const empty: SearchResponse = {
  search_id: 'synthetic', query: 'AnyDesk', context: { origin: 'unspecified', segment: 'anonymous' },
  catalog_version: 'synthetic', index_version: 'synthetic', mode: 'lexical', degraded_reasons: [],
  corrections: [], matches: [], alternatives: [], recommendations: [], total_matches: 0,
  clarification: [], duration_ms: 1, next_cursor: null, top4: [], shortfall_reason: 'insufficient_relevant_families', unpriced_fallback_used: false,
};
beforeEach(() => api.mockReset());

test('shows canonical interpretation and clarification independently of shortfall', async () => {
  api.mockResolvedValue({ ...empty, query: 'фотошор', shortfall_reason: null,
    corrections: [{ original: 'фотошор', replacement: 'Adobe Photoshop', kind: 'typo' }],
    clarification: ['Уточните продукт и производителя: Paint, Point'] });
  render(<SearchWorkspace />);
  await userEvent.type(screen.getByRole('searchbox', { name: 'Что вы ищете?' }), 'фотошор{enter}');
  expect(await screen.findByText(/Распознано: Adobe Photoshop/)).toBeInTheDocument();
  expect(screen.getByText('Уточните продукт и производителя: Paint, Point')).toBeInTheDocument();
  expect(screen.getByRole('searchbox')).toHaveValue('фотошор');
});

test('ambiguous search asks for clarification without claiming no products exist', async () => {
  api.mockResolvedValue({ ...empty, query: 'corle', total_matches: null, shortfall_reason: null,
    clarification: ['Уточните продукт и производителя: Corel Corporation, Qlik Core'] });
  render(<SearchWorkspace />);
  await userEvent.type(screen.getByRole('searchbox'), 'corle{enter}');
  expect(await screen.findByRole('heading', { name: 'Уточните запрос' })).toBeInTheDocument();
  expect(screen.queryByText('Подборка пока неполная')).toBeNull();
  expect(screen.queryByRole('heading', { name: 'Совпадений не найдено' })).toBeNull();
});
test('keyboard submission shows a truthful empty result and shortfall', async () => {
  api.mockResolvedValue(empty);
  render(<SearchWorkspace />);
  await userEvent.type(screen.getByRole('searchbox', { name: 'Что вы ищете?' }), 'AnyDesk{enter}');
  expect(await screen.findByRole('heading', { name: 'Совпадений не найдено' })).toBeInTheDocument();
  expect(screen.getByText(/Подтверждено семейств в подборке: 0 из 4/)).toBeInTheDocument();
  expect(api).toHaveBeenCalledTimes(1);
});
test('server failure remains an error and retry reuses the request identity', async () => {
  api.mockRejectedValueOnce(new ApiError('Поиск временно недоступен.', true)).mockResolvedValueOnce(empty);
  render(<SearchWorkspace />);
  await userEvent.click(screen.getByRole('button', { name: 'AnyDesk' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Поиск не завершён');
  expect(screen.queryByRole('heading', { name: 'Совпадений не найдено' })).toBeNull();
  await userEvent.click(screen.getByRole('button', { name: 'Повторить попытку' }));
  expect(await screen.findByRole('heading', { name: 'Совпадений не найдено' })).toBeInTheDocument();
  expect(api.mock.calls[0][1]).toBe(api.mock.calls[1][1]);
});

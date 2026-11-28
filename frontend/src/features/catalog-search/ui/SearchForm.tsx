import type { SearchRequest } from '@/shared/api';

type Kind = NonNullable<SearchRequest['filters']>['offer_kind'];
type Props = {
  query: string; onQuery: (value: string) => void; onSubmit: () => void;
  availableOnly: boolean; onAvailable: (value: boolean) => void;
  kind: Kind; onKind: (value: Kind) => void; loading: boolean;
};
export function SearchForm(p: Props) {
  return <form onSubmit={(event) => { event.preventDefault(); p.onSubmit(); }} className="search-form">
    <label htmlFor="catalog-query" className="field-label">Что вы ищете?</label>
    <div className="search-input-row">
      <span className="search-symbol" aria-hidden="true">⌕</span>
      <input id="catalog-query" type="search" value={p.query} onChange={(event) => p.onQuery(event.target.value)}
        placeholder="Название продукта, версия или артикул" maxLength={256} autoComplete="off" required />
      <button className="button button--primary" type="submit" disabled={!p.query.trim() || p.loading}>
        {p.loading ? 'Ищем…' : 'Найти'} <span aria-hidden="true">↗</span>
      </button>
    </div>
    <div className="search-filters">
      <label className="checkbox"><input type="checkbox" checked={p.availableOnly} onChange={(event) => p.onAvailable(event.target.checked)} />Только доступные</label>
      <label className="select-label">Лицензия
        <select aria-label="Тип лицензии" value={p.kind ?? ''} onChange={(event) => p.onKind(event.target.value as Kind || undefined)}>
          <option value="">По запросу</option><option value="new_license">Новая лицензия</option>
          <option value="renewal">Продление</option><option value="upgrade">Обновление</option><option value="support">Поддержка</option>
        </select>
      </label>
    </div>
  </form>;
}

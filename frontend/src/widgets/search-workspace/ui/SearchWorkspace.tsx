import { useCatalogSearch, SearchForm } from '@/features/catalog-search';
import { ProductCard } from '@/entities/product';

const examples = ['AnyDesk', 'Кибер Бэкап', 'КриптоПро CSP 5.0'];
export function SearchWorkspace() {
  const search = useCatalogSearch(); const state = search.state;
  return <section className="workspace" aria-label="Поиск по каталогу">
    <div className="search-panel">
      <SearchForm query={search.query} onQuery={search.setQuery} onSubmit={() => { void search.submit(); }}
        availableOnly={search.availableOnly} onAvailable={search.setAvailableOnly} kind={search.kind} onKind={search.setKind} loading={state.phase === 'loading'} />
      <div className="examples"><span>Например</span>{examples.map((query) => <button key={query} type="button" onClick={() => { void search.searchFor(query); }}>{query}<span aria-hidden="true">↗</span></button>)}</div>
    </div>
    {state.phase === 'idle' && <div className="welcome-state"><div className="catalog-glyph" aria-hidden="true"><i /><i /><i /><i /></div>
      <h2>Всё начинается с запроса</h2><p>Введите название продукта или выберите пример выше.<br />Мы покажем предложения из каталога и условия лицензий.</p>
      <div className="welcome-note"><span aria-hidden="true">↳</span> Опечатки, варианты написания и артикулы — тоже можно</div>
    </div>}
    <div aria-live="polite" aria-atomic="true" className="sr-only">{state.phase === 'success' ? `Показано предложений: ${state.data.matches.length}` : ''}</div>
    {state.phase === 'loading' && <div className="loading-state" role="status"><span className="spinner" />Ищем предложения в каталоге…</div>}
    {state.phase === 'error' && <div className="message-panel message-panel--error" role="alert"><h2>Поиск не завершён</h2><p>{state.error.message}</p>
      {state.error.retriable && <button className="button" onClick={() => { void search.retry(); }}>Повторить попытку</button>}
    </div>}
    {state.phase === 'success' && <div className="results">
      <div className="results-heading"><div><span className="eyebrow">РЕЗУЛЬТАТЫ ПОИСКА</span><h2>{state.data.matches.length ? `Предложения по запросу «${state.data.query}»` : 'Совпадений не найдено'}</h2></div>
        {state.data.matches.length > 0 && <span className="result-count">Показано: {state.data.matches.length}</span>}
      </div>
      {state.data.shortfall_reason && <div className="message-panel message-panel--notice"><strong>Подборка пока неполная</strong>
        <p>Подтверждено семейств в подборке: {state.data.top4.length} из 4.</p>
        {state.data.clarification.map((message, index) => <p key={index}>{message}</p>)}
      </div>}
      {state.data.degraded_reasons.length > 0 && <p className="mode-note" role="status">Часть возможностей поиска временно недоступна. Показаны доступные результаты.</p>}
      {state.data.matches.length === 0 ? <div className="empty-state"><p>Попробуйте короче указать название, проверить версию или изменить фильтры.</p></div> :
        <div className="product-grid">{state.data.matches.map((card, index) => <ProductCard key={card.product_id} card={card} position={index + 1} />)}</div>}
      <p className="results-footnote">Цена и наличие — по снимку каталога. Условия покупки и лицензирования проверьте перед оформлением.</p>
    </div>}
  </section>;
}

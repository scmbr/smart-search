import { SearchWorkspace } from '@/widgets/search-workspace';

export function SearchPage() {
  return <div className="site-shell">
    <header className="site-header"><a className="brand" href="/" aria-label="Каталог — на главную"><span className="brand-mark" aria-hidden="true"><i /><i /><i /><i /></span><span>каталог<span className="brand-divider">/</span><span className="brand-caption">умный поиск</span></span></a>
      <span className="demo-label"><span />Локальное демо</span>
    </header>
    <main>
      <section className="hero"><div><div className="eyebrow"><span className="accent-line" />ПО ДАННЫМ SOFTLINE</div>
        <h1>Программное обеспечение.<br /><span>Найдите своё решение.</span></h1>
        <p className="hero-copy">Названия, версии и варианты лицензий —<br className="desktop-break" /> в одном поиске по каталогу.</p>
      </div><div className="hero-aside"><span className="aside-number">01 — 04</span><p>Предложения из каталога.<br />Цена и наличие по снимку.</p><span className="aside-line" /></div></section>
      <SearchWorkspace />
    </main>
    <footer className="site-footer"><span>КАТАЛОГ / SEARCH LAB</span><span>Демонстрационный снимок · не актуальный складской учёт</span></footer>
  </div>;
}

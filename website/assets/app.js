(() => {
  const root = document.documentElement;
  const theme = document.querySelector('#theme-toggle');
  const syncTheme = () => theme.setAttribute('aria-pressed', String(root.dataset.theme === 'dark'));
  syncTheme();
  theme.addEventListener('click', () => {
    root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
    try { localStorage.setItem('goeval-theme', root.dataset.theme); } catch {}
    syncTheme();
  });

  document.querySelectorAll('.language-go').forEach(code => {
    const source = code.textContent;
    const tokens = /("(?:\\.|[^"\\])*"|`[^`]*`|\/\/[^\n]*|\b(?:package|import|func|return|if|else|for|range|type|struct|interface|var|const|nil|true|false|defer|go|select|case|switch|break|continue|map|chan)\b|\b\d+(?:\.\d+)?\b)/g;
    const fragment = document.createDocumentFragment();
    let position = 0;
    for (const match of source.matchAll(tokens)) {
      fragment.append(document.createTextNode(source.slice(position, match.index)));
      const span = document.createElement('span');
      span.className = match[0].startsWith('//') ? 'token-comment' : /^["`]/.test(match[0]) ? 'token-string' : /^\d/.test(match[0]) ? 'token-number' : 'token-keyword';
      span.textContent = match[0]; fragment.append(span); position = match.index + match[0].length;
    }
    fragment.append(document.createTextNode(source.slice(position)));
    code.replaceChildren(fragment);
  });

  document.querySelectorAll('.copy-button').forEach(button => {
    button.addEventListener('click', async () => {
      const text = button.closest('.code-block').querySelector('code').textContent;
      try {
        await navigator.clipboard.writeText(text);
        button.textContent = 'Copied'; button.dataset.copied = 'true';
      } catch { button.textContent = 'Select code to copy'; }
      setTimeout(() => { button.textContent = 'Copy'; delete button.dataset.copied; }, 2000);
    });
  });

  const menu = document.querySelector('#menu-toggle');
  const sidebar = document.querySelector('#sidebar');
  const overlay = document.querySelector('#menu-overlay');
  const setMenu = open => { sidebar.classList.toggle('is-open', open); menu.setAttribute('aria-expanded', String(open)); overlay.hidden = !open; };
  menu.addEventListener('click', () => setMenu(menu.getAttribute('aria-expanded') !== 'true'));
  overlay.addEventListener('click', () => setMenu(false));

  const dialog = document.querySelector('#search-dialog');
  const input = document.querySelector('#search-input');
  const results = document.querySelector('#search-results');
  const status = document.querySelector('#search-status');
  let index;
  let load;
  async function openSearch() {
    setMenu(false);
    if (!dialog.open) dialog.showModal();
    input.focus();
    if (!index) {
      if (!load) load = fetch('search.json').then(response => { if (!response.ok) throw new Error('Search unavailable'); return response.json(); });
      try { index = await load; search(); }
      catch { status.textContent = 'Search could not load. Use the navigation to browse the guides.'; load = undefined; }
    }
  }
  document.querySelector('#search-trigger').addEventListener('click', openSearch);
  document.querySelector('#search-close').addEventListener('click', () => dialog.close());
  document.addEventListener('keydown', event => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); openSearch(); }
    if (event.key === 'Escape') setMenu(false);
  });
  dialog.addEventListener('click', event => { if (event.target === dialog && (event.clientX < dialog.getBoundingClientRect().left || event.clientX > dialog.getBoundingClientRect().right || event.clientY < dialog.getBoundingClientRect().top || event.clientY > dialog.getBoundingClientRect().bottom)) dialog.close(); });
  function search() {
    results.replaceChildren();
    const query = input.value.trim().toLowerCase();
    if (!query) { status.textContent = 'Start typing to find a guide.'; return; }
    if (!index) { status.textContent = 'Loading search…'; return; }
    const terms = query.split(/\s+/);
    const matches = index.map(page => {
      const title = page.title.toLowerCase(); const content = (page.description + ' ' + page.text).toLowerCase();
      const score = terms.every(term => title.includes(term) || content.includes(term)) ? terms.reduce((sum, term) => sum + (title.includes(term) ? 10 : 1), 0) : 0;
      return { page, score };
    }).filter(item => item.score > 0).sort((a,b) => b.score - a.score).slice(0, 8);
    status.textContent = matches.length ? `${matches.length} matching guide${matches.length === 1 ? '' : 's'}` : 'No results. Try a metric name or a shorter phrase.';
    for (const { page } of matches) {
      const link = document.createElement('a'); link.className = 'search-result'; link.href = page.url;
      const title = document.createElement('strong'); title.textContent = page.title;
      const description = document.createElement('p'); description.textContent = page.description;
      link.append(title, description); results.append(link);
    }
  }
  input.addEventListener('input', search);
  dialog.addEventListener('keydown', event => {
    const links = Array.from(results.querySelectorAll('a'));
    if (!links.length) return;
    const active = links.indexOf(document.activeElement);
    if (event.key === 'ArrowDown') { event.preventDefault(); links[(active + 1) % links.length].focus(); }
    if (event.key === 'ArrowUp') { event.preventDefault(); if (active <= 0) input.focus(); else links[active-1].focus(); }
    if (event.key === 'Enter' && document.activeElement === input) { event.preventDefault(); links[0].click(); }
  });

  const headings = document.querySelectorAll('.prose h2[id],.prose h3[id]');
  if ('IntersectionObserver' in window) {
    const observer = new IntersectionObserver(entries => {
      for (const entry of entries) if (entry.isIntersecting) {
        document.querySelectorAll('.page-toc nav a').forEach(link => link.classList.toggle('toc-active', link.hash === '#' + entry.target.id));
      }
    }, { rootMargin: '-10% 0px -65% 0px' });
    headings.forEach(heading => observer.observe(heading));
  }
})();

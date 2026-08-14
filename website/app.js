/**
 * CINESCOPE — Client Application
 * Router, API Client, and UI Component Renderers
 */

const API_BASE = 'https://review-aggregator-api.kevinunderscoreladan.workers.dev';
const PAGE_SIZE = 24;

// Available genre filters
const GENRES = [
  'All',
  'Action',
  'Adventure',
  'Animation',
  'Comedy',
  'Crime',
  'Drama',
  'Horror',
  'Mystery',
  'Romance',
  'Science Fiction',
  'Thriller',
  'Family'
];

// App State
const state = {
  currentRoute: '',
  genre: 'All',
  sort: 'overall_sentiment',
  order: 'desc',
  page: 1,
  totalMovies: 0,
  searchQuery: '',
};

// DOM Elements
const appEl = document.getElementById('app');
const searchForm = document.getElementById('search-form');
const searchInput = document.getElementById('search-input');
const searchClearBtn = document.getElementById('search-clear-btn');
const statusCountEl = document.getElementById('status-count');

/* ==========================================================================
   API Client
   ========================================================================== */

async function apiFetch(endpoint) {
  const url = `${API_BASE}${endpoint}`;
  try {
    const res = await fetch(url);
    if (!res.ok) {
      const errJson = await res.json().catch(() => ({}));
      throw new Error(errJson.error || `HTTP ${res.status}: ${res.statusText}`);
    }
    return await res.json();
  } catch (err) {
    console.error(`API Fetch Error [${endpoint}]:`, err);
    throw err;
  }
}

async function fetchCatalogMovies() {
  const offset = (state.page - 1) * PAGE_SIZE;
  const params = new URLSearchParams({
    limit: PAGE_SIZE.toString(),
    offset: offset.toString(),
    sort: state.sort,
    order: state.order,
  });

  if (state.genre && state.genre !== 'All') {
    params.set('genre', state.genre);
  }

  return await apiFetch(`/api/v1/movies?${params.toString()}`);
}

async function fetchMovieDetail(id) {
  return await apiFetch(`/api/v1/movies/${encodeURIComponent(id)}`);
}

async function fetchSearchResults(query) {
  const params = new URLSearchParams({
    q: query,
    limit: '40',
  });
  return await apiFetch(`/api/v1/movies/search?${params.toString()}`);
}

/* ==========================================================================
   Router
   ========================================================================== */

function parseRoute() {
  const hash = window.location.hash.slice(1) || '/';
  
  if (hash.startsWith('/movie/')) {
    const movieId = hash.replace('/movie/', '').trim();
    return { name: 'detail', id: movieId };
  }
  
  if (hash.startsWith('/search')) {
    const urlParams = new URLSearchParams(hash.split('?')[1] || '');
    return { name: 'search', query: urlParams.get('q') || '' };
  }

  return { name: 'catalog' };
}

async function handleRouteChange() {
  const route = parseRoute();
  state.currentRoute = route.name;

  // Scroll to top on navigation
  window.scrollTo({ top: 0, behavior: 'smooth' });

  try {
    if (route.name === 'detail') {
      renderLoading('Loading movie intelligence...');
      const data = await fetchMovieDetail(route.id);
      if (!data || !data.result) {
        renderError('Movie not found', () => handleRouteChange());
        return;
      }
      renderDetail(data.result);
    } else if (route.name === 'search') {
      if (!route.query.trim()) {
        window.location.hash = '#/';
        return;
      }
      searchInput.value = route.query;
      searchClearBtn.style.display = 'block';
      renderLoading(`Searching for "${route.query}"...`);
      const data = await fetchSearchResults(route.query);
      renderSearchResults(route.query, data.results || []);
    } else {
      // Catalog View
      searchInput.value = '';
      searchClearBtn.style.display = 'none';
      renderLoading();
      const data = await fetchCatalogMovies();
      state.totalMovies = data.total || 0;
      updateStatusCount(state.totalMovies);
      renderCatalog(data);
    }
  } catch (err) {
    renderError(err.message || 'Failed to load data from Edge API', () => handleRouteChange());
  }
}

/* ==========================================================================
   View Renderers
   ========================================================================== */

function renderLoading(message = 'Loading movie catalog...') {
  appEl.innerHTML = `
    <div class="catalog-header">
      <div class="catalog-headline">
        <h1 class="catalog-title">${escapeHtml(message)}</h1>
      </div>
    </div>
    <div class="skeleton-grid">
      ${Array.from({ length: 12 }).map(() => `<div class="skeleton skeleton-card"></div>`).join('')}
    </div>
  `;
}

function renderError(message, retryFn) {
  appEl.innerHTML = `
    <div class="error-container">
      <div class="error-icon">
        <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"></circle>
          <line x1="12" y1="8" x2="12" y2="12"></line>
          <line x1="12" y1="16" x2="12.01" y2="16"></line>
        </svg>
      </div>
      <h2 class="error-title">Unable to Load Movies</h2>
      <p class="error-message">${escapeHtml(message)}</p>
      <button class="btn btn-primary" id="retry-btn">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="23 4 23 10 17 10"></polyline>
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path>
        </svg>
        Try Again
      </button>
    </div>
  `;

  document.getElementById('retry-btn')?.addEventListener('click', () => {
    if (retryFn) retryFn();
  });
}

function renderCatalog(data) {
  const movies = data.results || [];
  const total = data.total || 0;
  const totalPages = Math.ceil(total / PAGE_SIZE) || 1;

  appEl.innerHTML = `
    <div class="catalog-header">
      <div class="catalog-headline">
        <h1 class="catalog-title">Discover Movies</h1>
        <p class="catalog-subtitle">
          Real-time audience sentiment aggregated across Reddit &amp; Letterboxd, summarized with LLMs.
        </p>
      </div>

      <!-- Filter & Sort Toolbar -->
      <div class="filter-toolbar">
        <div class="genre-chips" id="genre-chips">
          ${GENRES.map((g) => `
            <button class="genre-chip ${state.genre === g ? 'active' : ''}" data-genre="${g}">
              ${g}
            </button>
          `).join('')}
        </div>

        <div class="sort-select-wrapper">
          <label for="sort-select" class="sort-label">Sort By</label>
          <select id="sort-select" class="sort-select">
            <option value="overall_sentiment" ${state.sort === 'overall_sentiment' ? 'selected' : ''}>Top Sentiment (AI)</option>
            <option value="release_date" ${state.sort === 'release_date' ? 'selected' : ''}>Release Date</option>
            <option value="imdb_score" ${state.sort === 'imdb_score' ? 'selected' : ''}>IMDb Score</option>
            <option value="rotten_tomatoes" ${state.sort === 'rotten_tomatoes' ? 'selected' : ''}>Rotten Tomatoes</option>
            <option value="last_updated" ${state.sort === 'last_updated' ? 'selected' : ''}>Recently Analyzed</option>
          </select>
        </div>
      </div>
    </div>

    <!-- Movie Cards Grid -->
    ${movies.length === 0 ? `
      <div class="empty-summary-card">
        <div class="empty-summary-icon">
          <svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"></circle>
            <path d="M8 15h8M9 9h.01M15 9h.01"></path>
          </svg>
        </div>
        <h3 class="empty-summary-title">No movies found</h3>
        <p class="empty-summary-desc">Try choosing a different genre or clearing filters.</p>
      </div>
    ` : `
      <div class="movies-grid">
        ${movies.map(renderMovieCard).join('')}
      </div>
    `}

    <!-- Pagination Controls -->
    ${total > PAGE_SIZE ? `
      <div class="pagination-bar">
        <div class="pagination-info">
          Page <strong>${state.page}</strong> of <strong>${totalPages}</strong> (${total.toLocaleString()} movies)
        </div>
        <div class="pagination-actions">
          <button class="btn btn-secondary" id="prev-page-btn" ${state.page <= 1 ? 'disabled' : ''}>
            &larr; Previous
          </button>
          <button class="btn btn-secondary" id="next-page-btn" ${state.page >= totalPages ? 'disabled' : ''}>
            Next &rarr;
          </button>
        </div>
      </div>
    ` : ''}
  `;

  // Attach Event Listeners
  attachCatalogEvents(totalPages);
}

function renderSearchResults(query, movies) {
  appEl.innerHTML = `
    <div class="catalog-header">
      <a href="#/" class="detail-nav-back" style="margin-bottom: 1.25rem;">
        &larr; Back to Catalog
      </a>
      <div class="catalog-headline">
        <h1 class="catalog-title">Search Results for "${escapeHtml(query)}"</h1>
        <p class="catalog-subtitle">Found ${movies.length} matching titles in the catalog</p>
      </div>
    </div>

    ${movies.length === 0 ? `
      <div class="empty-summary-card">
        <div class="empty-summary-icon">
          <svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
          </svg>
        </div>
        <h3 class="empty-summary-title">No matches found</h3>
        <p class="empty-summary-desc">We couldn't find any movie matching "${escapeHtml(query)}". Try another title or check spelling.</p>
        <a href="#/" class="btn btn-primary" style="margin-top: 0.5rem;">Explore Full Catalog</a>
      </div>
    ` : `
      <div class="movies-grid">
        ${movies.map(renderMovieCard).join('')}
      </div>
    `}
  `;
}

function renderMovieCard(movie) {
  const sentiment = movie.summary?.overall_sentiment;
  const sentimentClass = getSentimentClass(sentiment);
  const sentimentText = sentiment !== undefined && sentiment !== null ? `${sentiment}%` : 'Unrated';

  const year = movie.release_date ? new Date(movie.release_date).getFullYear() : '';
  const genresList = (movie.genres || []).slice(0, 2).join(' • ');

  return `
    <a href="#/movie/${encodeURIComponent(movie.id)}" class="movie-card" data-id="${movie.id}">
      <div class="card-poster-wrapper">
        ${movie.poster_url ? `
          <img 
            src="${escapeHtml(movie.poster_url)}" 
            alt="${escapeHtml(movie.title)} poster" 
            class="card-poster-img"
            loading="lazy"
            onerror="this.onerror=null; this.parentElement.innerHTML='<div class=\\'card-poster-fallback\\'><svg viewBox=\\'0 0 24 24\\' width=\\'36\\' height=\\'36\\' fill=\\'none\\' stroke=\\'currentColor\\' stroke-width=\\'1.5\\'><rect x=\\'2\\' y=\\'2\\' width=\\'20\\' height=\\'20\\' rx=\\'2.18\\'></rect><line x1=\\'7\\' y1=\\'2\\' x2=\\'7\\' y2=\\'22\\'></line><line x1=\\'17\\' y1=\\'2\\' x2=\\'17\\' y2=\\'22\\'></line></svg><span>${escapeHtml(movie.title)}</span></div>';"
          >
        ` : `
          <div class="card-poster-fallback">
            <svg viewBox="0 0 24 24" width="36" height="36" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="2" width="20" height="20" rx="2.18"></rect>
              <line x1="7" y1="2" x2="7" y2="22"></line>
              <line x1="17" y1="2" x2="17" y2="22"></line>
            </svg>
            <span>${escapeHtml(movie.title)}</span>
          </div>
        `}
        
        <div class="card-badge-top">
          <span class="sentiment-badge ${sentimentClass}" title="AI Sentiment Score">
            ${sentiment !== undefined && sentiment !== null ? `
              <svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor">
                <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z"></path>
              </svg>
            ` : ''}
            ${sentimentText}
          </span>
        </div>
      </div>

      <div class="card-content">
        <h3 class="card-title" title="${escapeHtml(movie.title)}">${escapeHtml(movie.title)}</h3>
        
        <div class="card-genres">${escapeHtml(genresList || 'Movie')}</div>

        <div class="card-meta">
          <span>${year || 'N/A'}</span>
          <div class="card-scores">
            ${movie.scores?.imdb ? `
              <span class="score-tag imdb" title="IMDb Rating">⭐ ${escapeHtml(movie.scores.imdb)}</span>
            ` : ''}
            ${movie.scores?.rotten_tomatoes ? `
              <span class="score-tag rt" title="Rotten Tomatoes Score">🍅 ${escapeHtml(movie.scores.rotten_tomatoes)}</span>
            ` : ''}
          </div>
        </div>
      </div>
    </a>
  `;
}

function renderDetail(movie) {
  const summary = movie.summary;
  const sentiment = summary?.overall_sentiment;
  const sentimentClass = getSentimentClass(sentiment);
  const releaseYear = movie.release_date ? new Date(movie.release_date).getFullYear() : 'N/A';
  const fullReleaseDate = movie.release_date ? new Date(movie.release_date).toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }) : '';
  const reviewsCount = movie.review_count_analyzed || 0;

  const pros = summary?.pros || [];
  const cons = summary?.cons || [];
  const themes = summary?.common_themes || [];
  const hasDeepSummary = Boolean(summary && (summary.audience_consensus || pros.length > 0 || cons.length > 0));

  appEl.innerHTML = `
    <div class="detail-view">
      <!-- Back Navigation -->
      <a href="#/" class="detail-nav-back" id="back-btn">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="19" y1="12" x2="5" y2="12"></line>
          <polyline points="12 19 5 12 12 5"></polyline>
        </svg>
        Back to Catalog
      </a>

      <!-- Hero Header Section -->
      <section class="detail-hero">
        ${movie.poster_url ? `
          <div class="detail-backdrop" style="background-image: url('${escapeHtml(movie.poster_url)}');"></div>
        ` : ''}

        <div class="detail-hero-content">
          <div class="detail-poster-container">
            ${movie.poster_url ? `
              <img src="${escapeHtml(movie.poster_url)}" alt="${escapeHtml(movie.title)} poster" class="detail-poster-img">
            ` : `
              <div class="card-poster-fallback" style="height: 100%;">
                <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.5">
                  <rect x="2" y="2" width="20" height="20" rx="2.18"></rect>
                  <line x1="7" y1="2" x2="7" y2="22"></line>
                  <line x1="17" y1="2" x2="17" y2="22"></line>
                </svg>
                <span>${escapeHtml(movie.title)}</span>
              </div>
            `}
          </div>

          <div class="detail-header-info">
            <h1 class="detail-title">${escapeHtml(movie.title)}</h1>

            <div class="detail-meta-row">
              <span class="meta-pill">📅 ${releaseYear}</span>
              ${fullReleaseDate ? `<span class="meta-pill">${fullReleaseDate}</span>` : ''}
              ${(movie.genres || []).map((g) => `<span class="meta-pill genre">${escapeHtml(g)}</span>`).join('')}
            </div>

            <!-- Score Dashboard -->
            <div class="detail-metrics-board">
              <div class="metric-card sentiment-highlight">
                <span class="metric-label">AI Sentiment</span>
                <span class="metric-value ${sentimentClass}">
                  ${sentiment !== undefined && sentiment !== null ? `${sentiment}%` : '—'}
                </span>
                <span class="metric-sub">${getSentimentLabel(sentiment)}</span>
              </div>

              ${movie.scores?.imdb ? `
                <div class="metric-card">
                  <span class="metric-label">IMDb Rating</span>
                  <span class="metric-value" style="color: var(--imdb-gold);">
                    ⭐ ${escapeHtml(movie.scores.imdb)}
                  </span>
                  <span class="metric-sub">Audience / 10</span>
                </div>
              ` : ''}

              ${movie.scores?.rotten_tomatoes ? `
                <div class="metric-card">
                  <span class="metric-label">Rotten Tomatoes</span>
                  <span class="metric-value" style="color: var(--rt-red);">
                    🍅 ${escapeHtml(movie.scores.rotten_tomatoes)}
                  </span>
                  <span class="metric-sub">Critic Consensus</span>
                </div>
              ` : ''}

              <div class="metric-card">
                <span class="metric-label">Audience Reviews</span>
                <span class="metric-value">${reviewsCount}</span>
                <span class="metric-sub">Reddit &amp; Letterboxd</span>
              </div>
            </div>

            <!-- Synopsis Overview -->
            ${movie.overview ? `
              <div class="detail-overview-section">
                <span class="section-label">Synopsis</span>
                <p class="detail-overview-text">${escapeHtml(movie.overview)}</p>
              </div>
            ` : ''}
          </div>
        </div>
      </section>

      <!-- AI Review Intelligence -->
      ${hasDeepSummary ? `
        <section class="summary-container">
          <div class="summary-header">
            <div class="summary-title-group">
              <div class="ai-pulse-icon">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
                </svg>
              </div>
              <h2 class="summary-title">AI Review Intelligence</h2>
            </div>
            <span class="review-count-badge">Synthesized from ${reviewsCount} community reviews</span>
          </div>

          <!-- Audience Consensus -->
          ${summary.audience_consensus ? `
            <div class="consensus-card">
              <div class="consensus-card-header">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                  <path d="M14.017 21v-7.391c0-5.704 3.731-9.57 8.983-10.609l.995 2.151c-2.432.917-3.995 3.638-3.995 5.849h4v10h-9.983zm-14.017 0v-7.391c0-5.704 3.748-9.57 9-10.609l.996 2.151c-2.433.917-3.996 3.638-3.996 5.849h3.983v10h-9.983z"/>
                </svg>
                Audience Consensus
              </div>
              <p class="consensus-text">"${escapeHtml(summary.audience_consensus)}"</p>
            </div>
          ` : ''}

          <!-- Recommendation Callout -->
          ${summary.recommendation ? `
            <div class="recommendation-card">
              <div class="recommendation-icon">
                <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path>
                  <polyline points="22 4 12 14.01 9 11.01"></polyline>
                </svg>
              </div>
              <div class="recommendation-content">
                <h3 class="recommendation-heading">Who Should Watch This?</h3>
                <p class="recommendation-text">${escapeHtml(summary.recommendation)}</p>
              </div>
            </div>
          ` : ''}

          <!-- Pros & Cons Breakdown -->
          ${(pros.length > 0 || cons.length > 0) ? `
            <div class="pros-cons-grid">
              <div class="points-card pros">
                <div class="points-card-header">
                  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="20 6 9 17 4 12"></polyline>
                  </svg>
                  What Audiences Loved
                </div>
                <ul class="points-list">
                  ${pros.map((pro) => `
                    <li class="point-item">
                      <span class="point-bullet">✓</span>
                      <span>${escapeHtml(pro)}</span>
                    </li>
                  `).join('')}
                </ul>
              </div>

              <div class="points-card cons">
                <div class="points-card-header">
                  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <line x1="18" y1="6" x2="6" y2="18"></line>
                    <line x1="6" y1="6" x2="18" y2="18"></line>
                  </svg>
                  Critical Caveats &amp; Flaws
                </div>
                <ul class="points-list">
                  ${cons.map((con) => `
                    <li class="point-item">
                      <span class="point-bullet">✕</span>
                      <span>${escapeHtml(con)}</span>
                    </li>
                  `).join('')}
                </ul>
              </div>
            </div>
          ` : ''}

          <!-- Key Discussion Themes -->
          ${themes.length > 0 ? `
            <div class="themes-section">
              <h3 class="themes-heading">Common Discussion Themes</h3>
              <div class="themes-cloud">
                ${themes.map((theme) => `
                  <span class="theme-tag"># ${escapeHtml(theme)}</span>
                `).join('')}
              </div>
            </div>
          ` : ''}
        </section>
      ` : `
        <!-- Unanalyzed State for Catalog Only Titles -->
        <div class="empty-summary-card">
          <div class="empty-summary-icon">
            <svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"></circle>
              <polyline points="12 6 12 12 16 14"></polyline>
            </svg>
          </div>
          <h3 class="empty-summary-title">Deep AI Review Summary Pending</h3>
          <p class="empty-summary-desc">
            This title has been cataloged from TMDB. The automated review harvester processes popular releases on schedule via GitHub Actions to collect Reddit &amp; Letterboxd sentiment.
          </p>
        </div>
      `}
    </div>
  `;
}

/* ==========================================================================
   Event Handlers & Interactivity
   ========================================================================= */

function attachCatalogEvents(totalPages) {
  // Genre Filter Buttons
  const genreChips = document.querySelectorAll('.genre-chip');
  genreChips.forEach((chip) => {
    chip.addEventListener('click', (e) => {
      const selected = e.currentTarget.getAttribute('data-genre');
      if (selected !== state.genre) {
        state.genre = selected;
        state.page = 1;
        handleRouteChange();
      }
    });
  });

  // Sort Dropdown
  const sortSelect = document.getElementById('sort-select');
  if (sortSelect) {
    sortSelect.addEventListener('change', (e) => {
      state.sort = e.target.value;
      state.page = 1;
      handleRouteChange();
    });
  }

  // Pagination Buttons
  const prevBtn = document.getElementById('prev-page-btn');
  const nextBtn = document.getElementById('next-page-btn');

  if (prevBtn) {
    prevBtn.addEventListener('click', () => {
      if (state.page > 1) {
        state.page -= 1;
        handleRouteChange();
      }
    });
  }

  if (nextBtn) {
    nextBtn.addEventListener('click', () => {
      if (state.page < totalPages) {
        state.page += 1;
        handleRouteChange();
      }
    });
  }
}

function updateStatusCount(total) {
  if (statusCountEl && total) {
    statusCountEl.textContent = `${total.toLocaleString()} Movies`;
  }
}

// Search Form Submit
searchForm.addEventListener('submit', (e) => {
  e.preventDefault();
  const query = searchInput.value.trim();
  if (query) {
    window.location.hash = `#/search?q=${encodeURIComponent(query)}`;
  }
});

// Search Input Changes
searchInput.addEventListener('input', (e) => {
  const val = e.target.value.trim();
  searchClearBtn.style.display = val ? 'block' : 'none';
});

// Clear Search
searchClearBtn.addEventListener('click', () => {
  searchInput.value = '';
  searchClearBtn.style.display = 'none';
  if (state.currentRoute === 'search') {
    window.location.hash = '#/';
  }
});

/* ==========================================================================
   Utility Helpers
   ========================================================================== */

function getSentimentClass(score) {
  if (score === null || score === undefined) return 'none';
  if (score >= 70) return 'high';
  if (score >= 40) return 'mid';
  return 'low';
}

function getSentimentLabel(score) {
  if (score === null || score === undefined) return 'Awaiting Analysis';
  if (score >= 80) return 'Overwhelmingly Positive';
  if (score >= 65) return 'Generally Favorable';
  if (score >= 45) return 'Mixed or Average';
  if (score >= 30) return 'Leaning Negative';
  return 'Unfavorable';
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

/* ==========================================================================
   Initialization
   ========================================================================== */

window.addEventListener('hashchange', handleRouteChange);
window.addEventListener('DOMContentLoaded', handleRouteChange);

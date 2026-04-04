type SortKey = 'track' | 'title' | 'artist' | 'album' | 'year' | 'plays' | 'dateAdded' | 'duration';
type SortDirection = 'asc' | 'desc';

interface ArchiveBrowserMeta {
  totalTracks: number;
  totalArtists: number;
  totalAlbums: number;
  totalGenres: number;
  totalDurationSeconds: number;
  totalDurationFormatted: string;
}

interface ArchiveBrowserGenre {
  slug: string;
  name: string;
  artistCount: number;
  albumCount: number;
  trackCount: number;
}

interface ArchiveBrowserArtist {
  slug: string;
  name: string;
  albumCount: number;
  trackCount: number;
  genreSlugs: string[];
}

interface ArchiveBrowserAlbum {
  slug: string;
  name: string;
  artistSlugs: string[];
  genreSlugs: string[];
  trackCount: number;
  year?: number;
}

interface ArchiveBrowserTrack {
  id: string;
  name: string;
  artistSlug: string;
  albumSlug: string;
  genreSlug: string;
  year?: number;
  trackNumber?: number;
  discNumber?: number;
  playCount: number;
  dateAdded?: string;
  dateAddedUnix?: number;
  duration: number;
  durationFormatted: string;
}

interface ArchiveBrowserData {
  meta: ArchiveBrowserMeta;
  genres: ArchiveBrowserGenre[];
  artists: ArchiveBrowserArtist[];
  albums: ArchiveBrowserAlbum[];
  tracks: ArchiveBrowserTrack[];
}

interface ImageMaps {
  albumImages: Record<string, string>;
  artistImages: Record<string, string>;
}

interface BrowserState {
  genre: string;
  artist: string;
  album: string;
  q: string;
  sort: SortKey;
  dir: SortDirection;
}

const DEFAULT_STATE: BrowserState = {
  genre: '',
  artist: '',
  album: '',
  q: '',
  sort: 'artist',
  dir: 'asc',
};

const SORT_KEYS = new Set<SortKey>(['track', 'title', 'artist', 'album', 'year', 'plays', 'dateAdded', 'duration']);
const ROW_HEIGHT = 48;
const ROW_OVERSCAN = 10;
type BrowserRefs = NonNullable<ReturnType<typeof bindRefs>>;

export async function mountLibraryBrowser(root: HTMLElement): Promise<void> {
  const dataRoot = root.dataset.dataRoot;
  const libraryUrl = root.dataset.libraryUrl;

  if (!dataRoot || !libraryUrl) {
    root.innerHTML = renderFatal('Library browser is missing its startup configuration.');
    return;
  }

  root.innerHTML = renderShell();

  const refs = bindRefs(root);
  if (!refs) {
    root.innerHTML = renderFatal('Library browser failed to initialize its view.');
    return;
  }

  try {
    const [data, imageMaps] = await Promise.all([
      fetchJson<ArchiveBrowserData>(`${dataRoot}/archive-browser.json`),
      fetchImageMaps(dataRoot),
    ]);

    const artistsBySlug = new Map(data.artists.map((artist) => [artist.slug, artist]));
    const albumsBySlug = new Map(data.albums.map((album) => [album.slug, album]));
    const genresBySlug = new Map(data.genres.map((genre) => [genre.slug, genre]));

    const app = {
      data,
      imageMaps,
      artistsBySlug,
      albumsBySlug,
      genresBySlug,
      rootUrl: libraryUrl,
      state: reconcileState(readStateFromUrl(window.location.search), { artistsBySlug, albumsBySlug, genresBySlug }),
      selectedTrackId: '',
      searchTimer: 0 as number | undefined,
      visibleTracks: [] as ArchiveBrowserTrack[],
    };

    refs.searchInput.value = app.state.q;
    refs.songScroller.addEventListener('scroll', () => {
      renderSongRows(app.visibleTracks, app, refs);
    });

    refs.searchInput.addEventListener('input', () => {
      window.clearTimeout(app.searchTimer);
      app.searchTimer = window.setTimeout(() => {
        app.state.q = refs.searchInput.value.trim();
        syncStateToUrl(app.state, libraryUrl, true);
        render(app, refs, libraryUrl);
      }, 120);
    });

    root.addEventListener('click', (event) => {
      const target = event.target instanceof HTMLElement ? event.target : null;
      if (!target) {
        return;
      }

      const paneButton = target.closest<HTMLElement>('[data-pane][data-value]');
      if (paneButton) {
        event.preventDefault();
        applyPaneSelection(app.state, paneButton.dataset.pane ?? '', paneButton.dataset.value ?? '');
        syncStateToUrl(app.state, libraryUrl, false);
        render(app, refs, libraryUrl);
        return;
      }

      const sortButton = target.closest<HTMLElement>('[data-sort]');
      if (sortButton) {
        event.preventDefault();
        const sort = sortButton.dataset.sort as SortKey;
        if (SORT_KEYS.has(sort)) {
          if (app.state.sort === sort) {
            app.state.dir = app.state.dir === 'asc' ? 'desc' : 'asc';
          } else {
            app.state.sort = sort;
            app.state.dir = sort === 'track' ? 'asc' : 'asc';
          }
          syncStateToUrl(app.state, libraryUrl, false);
          render(app, refs, libraryUrl);
        }
        return;
      }

      const clearButton = target.closest<HTMLElement>('[data-action]');
      if (clearButton) {
        event.preventDefault();
        const action = clearButton.dataset.action;
        if (action === 'clear-selection') {
          app.state = { ...DEFAULT_STATE };
          refs.searchInput.value = '';
          app.selectedTrackId = '';
          syncStateToUrl(app.state, libraryUrl, false);
          render(app, refs, libraryUrl);
        } else if (action === 'clear-search') {
          app.state.q = '';
          refs.searchInput.value = '';
          syncStateToUrl(app.state, libraryUrl, true);
          render(app, refs, libraryUrl);
        }
        return;
      }

      const filterLink = target.closest<HTMLElement>('[data-link-filter]');
      if (filterLink) {
        event.preventDefault();
        const kind = filterLink.dataset.linkFilter;
        const value = filterLink.dataset.value ?? '';
        if (kind === 'artist') {
          app.state.artist = value;
          app.state.album = '';
        } else if (kind === 'album') {
          app.state.artist = filterLink.dataset.artist ?? app.state.artist;
          app.state.album = value;
        }
        app.state = reconcileState(app.state, app);
        syncStateToUrl(app.state, libraryUrl, false);
        render(app, refs, libraryUrl);
        return;
      }

      const row = target.closest<HTMLElement>('[data-track-id]');
      if (row) {
        app.selectedTrackId = row.dataset.trackId ?? '';
        renderInspector(app, refs, libraryUrl);
      }
    });

    window.addEventListener('popstate', () => {
      app.state = reconcileState(readStateFromUrl(window.location.search), app);
      refs.searchInput.value = app.state.q;
      render(app, refs, libraryUrl);
    });

    render(app, refs, libraryUrl);
  } catch (error) {
    console.error(error);
    root.innerHTML = renderFatal('Library data failed to load. Run the web-data pipeline and try again.');
  }
}

function bindRefs(root: HTMLElement) {
  const searchInput = root.querySelector<HTMLInputElement>('#library-search');
  const searchReset = root.querySelector<HTMLButtonElement>('#library-search-reset');
  const genrePane = root.querySelector<HTMLElement>('#genre-pane');
  const artistPane = root.querySelector<HTMLElement>('#artist-pane');
  const albumPane = root.querySelector<HTMLElement>('#album-pane');
  const statusBar = root.querySelector<HTMLElement>('#library-status');
  const summaryRail = root.querySelector<HTMLElement>('#library-summary-rail');
  const inspector = root.querySelector<HTMLElement>('#library-inspector');
  const songScroller = root.querySelector<HTMLElement>('#song-scroller');
  const songSpacer = root.querySelector<HTMLElement>('#song-spacer');
  const songRows = root.querySelector<HTMLElement>('#song-rows');
  const songHeader = root.querySelector<HTMLElement>('#song-header');

  if (!searchInput || !searchReset || !genrePane || !artistPane || !albumPane || !statusBar || !summaryRail || !inspector || !songScroller || !songSpacer || !songRows || !songHeader) {
    return null;
  }

  return {
    searchInput,
    searchReset,
    genrePane,
    artistPane,
    albumPane,
    statusBar,
    summaryRail,
    inspector,
    songScroller,
    songSpacer,
    songRows,
    songHeader,
  };
}

function render(app: any, refs: BrowserRefs, libraryUrl: string) {
  app.state = reconcileState(app.state, app);

  const genreTracks = app.state.genre
    ? app.data.tracks.filter((track: ArchiveBrowserTrack) => track.genreSlug === app.state.genre)
    : app.data.tracks;

  const artistTracks = app.state.artist
    ? genreTracks.filter((track: ArchiveBrowserTrack) => track.artistSlug === app.state.artist)
    : genreTracks;

  const selectedTracks = app.state.album
    ? artistTracks.filter((track: ArchiveBrowserTrack) => track.albumSlug === app.state.album)
    : artistTracks;

  const visibleTracks = filterTracksBySearch(selectedTracks, app.state.q, app.artistsBySlug, app.albumsBySlug);
  app.visibleTracks = sortTracks(visibleTracks, app.state, app.artistsBySlug, app.albumsBySlug);

  if (app.selectedTrackId && !app.visibleTracks.some((track: ArchiveBrowserTrack) => track.id === app.selectedTrackId)) {
    app.selectedTrackId = '';
  }

  refs.searchReset.hidden = app.state.q.length === 0;
  refs.summaryRail.innerHTML = renderSummaryRail(app, genreTracks, artistTracks, selectedTracks);
  refs.statusBar.innerHTML = renderStatusBar(app, visibleTracks);
  refs.genrePane.innerHTML = renderPaneList({
    title: 'Genre',
    items: app.data.genres.map((genre: ArchiveBrowserGenre) => ({
      value: genre.slug,
      label: genre.name,
      count: genre.trackCount,
    })),
    activeValue: app.state.genre,
    allLabel: 'All Genres',
    allCount: app.data.meta.totalTracks,
    pane: 'genre',
  });
  refs.artistPane.innerHTML = renderPaneList({
    title: 'Artist',
    items: buildArtistPaneItems(app, genreTracks),
    activeValue: app.state.artist,
    allLabel: 'All Artists',
    allCount: genreTracks.length,
    pane: 'artist',
  });
  refs.albumPane.innerHTML = renderPaneList({
    title: 'Album',
    items: buildAlbumPaneItems(app, artistTracks),
    activeValue: app.state.album,
    allLabel: 'All Albums',
    allCount: artistTracks.length,
    pane: 'album',
  });
  refs.songHeader.innerHTML = renderSongHeader(app.state);
  const maxScrollTop = Math.max(0, app.visibleTracks.length * ROW_HEIGHT - refs.songScroller.clientHeight);
  if (refs.songScroller.scrollTop > maxScrollTop) {
    refs.songScroller.scrollTop = maxScrollTop;
  }
  renderSongRows(app.visibleTracks, app, refs);
  renderInspector(app, refs, libraryUrl);
}

function renderSongRows(tracks: ArchiveBrowserTrack[], app: any, refs: BrowserRefs) {
  if (tracks.length === 0) {
    refs.songSpacer.style.height = '0px';
    refs.songRows.style.transform = 'translateY(0px)';
    refs.songRows.innerHTML = `
      <div class="song-empty-state">
        <strong>No songs match this slice.</strong>
        <span>Change the pane selection or clear the search field to widen the library.</span>
      </div>
    `;
    return;
  }

  const viewportHeight = refs.songScroller.clientHeight || 560;
  const scrollTop = refs.songScroller.scrollTop;
  const visibleCount = Math.ceil(viewportHeight / ROW_HEIGHT) + ROW_OVERSCAN;
  const startIndex = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - Math.floor(ROW_OVERSCAN / 2));
  const endIndex = Math.min(tracks.length, startIndex + visibleCount);
  const offsetTop = startIndex * ROW_HEIGHT;
  const slice = tracks.slice(startIndex, endIndex);

  refs.songSpacer.style.height = `${tracks.length * ROW_HEIGHT}px`;
  refs.songRows.style.transform = `translateY(${offsetTop}px)`;
  refs.songRows.innerHTML = slice.map((track) => renderSongRow(track, app)).join('');
}

function renderInspector(app: any, refs: BrowserRefs, libraryUrl: string) {
  const selectedTrack = app.selectedTrackId
    ? app.visibleTracks.find((track: ArchiveBrowserTrack) => track.id === app.selectedTrackId)
    : null;
  const artist = app.state.artist ? app.artistsBySlug.get(app.state.artist) : null;
  const album = app.state.album ? app.albumsBySlug.get(app.state.album) : null;
  const genre = app.state.genre ? app.genresBySlug.get(app.state.genre) : null;

  const focusTitle = selectedTrack
    ? selectedTrack.name
    : album?.name || artist?.name || genre?.name || 'Library Browser';
  const focusLabel = selectedTrack
    ? 'Selected Song'
    : album
      ? 'Album Focus'
      : artist
        ? 'Artist Focus'
        : genre
          ? 'Genre Focus'
          : 'Collection Focus';

  const focusImage = resolveFocusArtwork(app, selectedTrack, album, artist);
  const stats = buildInspectorStats(app, selectedTrack, album, artist, genre);
  const description = selectedTrack
    ? `${app.artistsBySlug.get(selectedTrack.artistSlug)?.name ?? 'Unknown Artist'} | ${app.albumsBySlug.get(selectedTrack.albumSlug)?.name ?? 'Unknown Album'}`
    : buildSelectionTrail(app);

  refs.inspector.innerHTML = `
    <div class="inspector-card">
      <div class="inspector-art ${focusImage ? 'has-art' : 'no-art'}">
        ${focusImage ? `<img src="${escapeHtml(focusImage)}" alt="${escapeHtml(focusTitle)}" loading="lazy" />` : `<div class="inspector-placeholder">${escapeHtml(buildPlaceholderInitials(focusTitle))}</div>`}
      </div>
      <div class="inspector-copy">
        <p class="inspector-kicker">${escapeHtml(focusLabel)}</p>
        <h2>${escapeHtml(focusTitle)}</h2>
        <p class="inspector-description">${escapeHtml(description)}</p>
      </div>
      <div class="inspector-stats">
        ${stats.map((stat) => `<article><span>${escapeHtml(stat.label)}</span><strong>${escapeHtml(stat.value)}</strong></article>`).join('')}
      </div>
      <div class="inspector-actions">
        <a class="inspector-link" href="${escapeHtml(buildShareHref(app.state, libraryUrl))}">Copyable View URL</a>
        <button type="button" class="inspector-reset" data-action="clear-selection">Reset Library</button>
      </div>
    </div>
  `;
}

function renderSummaryRail(app: any, genreTracks: ArchiveBrowserTrack[], artistTracks: ArchiveBrowserTrack[], selectedTracks: ArchiveBrowserTrack[]) {
  const selectedDuration = selectedTracks.reduce((sum, track) => sum + track.duration, 0);
  const cards = [
    { label: 'Library', value: `${formatNumber(app.data.meta.totalTracks)} songs`, meta: `${formatNumber(app.data.meta.totalArtists)} artists` },
    { label: 'Current Slice', value: `${formatNumber(selectedTracks.length)} songs`, meta: formatDuration(selectedDuration) },
    { label: 'Genre Scope', value: `${formatNumber(genreTracks.length)} songs`, meta: app.state.genre ? app.genresBySlug.get(app.state.genre)?.name ?? 'Active genre' : 'Whole collection' },
    { label: 'Artist Scope', value: `${formatNumber(artistTracks.length)} songs`, meta: app.state.artist ? app.artistsBySlug.get(app.state.artist)?.name ?? 'Active artist' : 'All artists' },
  ];

  return cards.map((card) => `
    <article class="summary-card">
      <span>${escapeHtml(card.label)}</span>
      <strong>${escapeHtml(card.value)}</strong>
      <small>${escapeHtml(card.meta)}</small>
    </article>
  `).join('');
}

function renderStatusBar(app: any, visibleTracks: ArchiveBrowserTrack[]) {
  const searchLabel = app.state.q ? `Filtered by "${app.state.q}"` : 'No text filter';
  const selectedDuration = visibleTracks.reduce((sum, track) => sum + track.duration, 0);

  return `
    <div class="status-copy">
      <span class="status-pill">Vintage browser</span>
      <strong>${escapeHtml(buildSelectionTrail(app))}</strong>
      <span>${formatNumber(visibleTracks.length)} songs</span>
      <span>${escapeHtml(formatDuration(selectedDuration))}</span>
      <span>${escapeHtml(searchLabel)}</span>
    </div>
    <div class="status-actions">
      <button type="button" data-action="clear-search" ${app.state.q ? '' : 'disabled'}>Clear Search</button>
      <button type="button" data-action="clear-selection">Reset</button>
    </div>
  `;
}

function renderPaneList(config: {
  title: string;
  items: Array<{ value: string; label: string; count: number }>;
  activeValue: string;
  allLabel: string;
  allCount: number;
  pane: string;
}) {
  return `
    <div class="pane-card">
      <div class="pane-heading">
        <span>${escapeHtml(config.title)}</span>
        <small>${formatNumber(config.items.length)}</small>
      </div>
      <div class="pane-scroll">
        <button class="pane-row ${config.activeValue === '' ? 'is-active' : ''}" data-pane="${escapeHtml(config.pane)}" data-value="">
          <span>${escapeHtml(config.allLabel)}</span>
          <strong>${formatNumber(config.allCount)}</strong>
        </button>
        ${config.items.map((item) => `
          <button class="pane-row ${config.activeValue === item.value ? 'is-active' : ''}" data-pane="${escapeHtml(config.pane)}" data-value="${escapeHtml(item.value)}">
            <span>${escapeHtml(item.label)}</span>
            <strong>${formatNumber(item.count)}</strong>
          </button>
        `).join('')}
      </div>
    </div>
  `;
}

function renderSongHeader(state: BrowserState) {
  const columns: Array<{ key: SortKey; label: string; align?: 'right' }> = [
    { key: 'track', label: '#' },
    { key: 'title', label: 'Title' },
    { key: 'artist', label: 'Artist' },
    { key: 'album', label: 'Album' },
    { key: 'year', label: 'Year', align: 'right' },
    { key: 'plays', label: 'Plays', align: 'right' },
    { key: 'dateAdded', label: 'Added' },
    { key: 'duration', label: 'Time', align: 'right' },
  ];

  return columns.map((column) => {
    const active = state.sort === column.key;
    const arrow = active ? (state.dir === 'asc' ? '↑' : '↓') : '';
    return `
      <button class="song-header-cell ${column.align === 'right' ? 'align-right' : ''} ${active ? 'is-active' : ''}" data-sort="${column.key}">
        <span>${escapeHtml(column.label)}</span>
        <em>${arrow}</em>
      </button>
    `;
  }).join('');
}

function renderSongRow(track: ArchiveBrowserTrack, app: any) {
  const artist = app.artistsBySlug.get(track.artistSlug);
  const album = app.albumsBySlug.get(track.albumSlug);
  const isSelected = app.selectedTrackId === track.id;

  return `
    <div class="song-row ${isSelected ? 'is-selected' : ''}" data-track-id="${escapeHtml(track.id)}">
      <div class="song-cell song-track">${escapeHtml(renderTrackOrdinal(track))}</div>
      <div class="song-cell song-title">
        <span class="song-title-main">${escapeHtml(track.name)}</span>
      </div>
      <div class="song-cell">
        <a href="${escapeHtml(buildLibraryHref(app.state, app, { artist: track.artistSlug, album: '' }))}" data-link-filter="artist" data-value="${escapeHtml(track.artistSlug)}">
          ${escapeHtml(artist?.name ?? 'Unknown Artist')}
        </a>
      </div>
      <div class="song-cell">
        <a href="${escapeHtml(buildLibraryHref(app.state, app, { artist: track.artistSlug, album: track.albumSlug }))}" data-link-filter="album" data-artist="${escapeHtml(track.artistSlug)}" data-value="${escapeHtml(track.albumSlug)}">
          ${escapeHtml(album?.name ?? 'Unknown Album')}
        </a>
      </div>
      <div class="song-cell align-right">${track.year ? formatNumber(track.year) : '-'}</div>
      <div class="song-cell align-right">${track.playCount > 0 ? formatNumber(track.playCount) : '-'}</div>
      <div class="song-cell">${escapeHtml(track.dateAdded ?? '-')}</div>
      <div class="song-cell align-right">${escapeHtml(track.durationFormatted)}</div>
    </div>
  `;
}

function buildArtistPaneItems(app: any, tracks: ArchiveBrowserTrack[]) {
  const counts = new Map<string, number>();
  tracks.forEach((track) => {
    counts.set(track.artistSlug, (counts.get(track.artistSlug) ?? 0) + 1);
  });

  return app.data.artists
    .filter((artist: ArchiveBrowserArtist) => counts.has(artist.slug))
    .map((artist: ArchiveBrowserArtist) => ({
      value: artist.slug,
      label: artist.name,
      count: counts.get(artist.slug) ?? 0,
    }));
}

function buildAlbumPaneItems(app: any, tracks: ArchiveBrowserTrack[]) {
  const counts = new Map<string, number>();
  tracks.forEach((track) => {
    counts.set(track.albumSlug, (counts.get(track.albumSlug) ?? 0) + 1);
  });

  return app.data.albums
    .filter((album: ArchiveBrowserAlbum) => counts.has(album.slug))
    .map((album: ArchiveBrowserAlbum) => ({
      value: album.slug,
      label: album.name,
      count: counts.get(album.slug) ?? 0,
    }));
}

function buildSelectionTrail(app: any) {
  const labels = ['Library'];
  if (app.state.genre) {
    labels.push(app.genresBySlug.get(app.state.genre)?.name ?? 'Genre');
  }
  if (app.state.artist) {
    labels.push(app.artistsBySlug.get(app.state.artist)?.name ?? 'Artist');
  }
  if (app.state.album) {
    labels.push(app.albumsBySlug.get(app.state.album)?.name ?? 'Album');
  }
  return labels.join(' / ');
}

function buildInspectorStats(app: any, selectedTrack: ArchiveBrowserTrack | null, album: ArchiveBrowserAlbum | null, artist: ArchiveBrowserArtist | null, genre: ArchiveBrowserGenre | null) {
  if (selectedTrack) {
    return [
      { label: 'Track No.', value: renderTrackOrdinal(selectedTrack) },
      { label: 'Plays', value: selectedTrack.playCount > 0 ? formatNumber(selectedTrack.playCount) : '0' },
      { label: 'Year', value: selectedTrack.year ? formatNumber(selectedTrack.year) : '-' },
      { label: 'Added', value: selectedTrack.dateAdded || '-' },
      { label: 'Duration', value: selectedTrack.durationFormatted },
    ];
  }

  if (album) {
    return [
      { label: 'Tracks', value: formatNumber(album.trackCount) },
      { label: 'Artists', value: formatNumber(album.artistSlugs.length) },
      { label: 'Genres', value: formatNumber(album.genreSlugs.length) },
      { label: 'Year', value: album.year ? formatNumber(album.year) : '-' },
      { label: 'Route', value: buildSelectionTrail(app) },
    ];
  }

  if (artist) {
    return [
      { label: 'Tracks', value: formatNumber(artist.trackCount) },
      { label: 'Albums', value: formatNumber(artist.albumCount) },
      { label: 'Genres', value: formatNumber(artist.genreSlugs.length) },
      { label: 'Route', value: buildSelectionTrail(app) },
    ];
  }

  if (genre) {
    return [
      { label: 'Tracks', value: formatNumber(genre.trackCount) },
      { label: 'Artists', value: formatNumber(genre.artistCount) },
      { label: 'Albums', value: formatNumber(genre.albumCount) },
      { label: 'Route', value: buildSelectionTrail(app) },
    ];
  }

  return [
    { label: 'Songs', value: formatNumber(app.data.meta.totalTracks) },
    { label: 'Artists', value: formatNumber(app.data.meta.totalArtists) },
    { label: 'Albums', value: formatNumber(app.data.meta.totalAlbums) },
    { label: 'Genres', value: formatNumber(app.data.meta.totalGenres) },
    { label: 'Duration', value: app.data.meta.totalDurationFormatted },
  ];
}

function resolveFocusArtwork(app: any, selectedTrack: ArchiveBrowserTrack | null, album: ArchiveBrowserAlbum | null, artist: ArchiveBrowserArtist | null) {
  if (selectedTrack) {
    const key = `${selectedTrack.artistSlug}|${selectedTrack.albumSlug}`;
    return app.imageMaps.albumImages[key] || app.imageMaps.artistImages[selectedTrack.artistSlug] || '';
  }

  if (album) {
    for (const artistSlug of album.artistSlugs) {
      const key = `${artistSlug}|${album.slug}`;
      if (app.imageMaps.albumImages[key]) {
        return app.imageMaps.albumImages[key];
      }
    }
    return album.artistSlugs.map((artistSlug) => app.imageMaps.artistImages[artistSlug]).find(Boolean) || '';
  }

  if (artist) {
    return app.imageMaps.artistImages[artist.slug] || '';
  }

  return '';
}

function buildPlaceholderInitials(value: string) {
  const parts = value.replace(/[^a-zA-Z0-9\s]/g, '').split(/\s+/).filter(Boolean);
  if (parts.length === 0) {
    return 'MP';
  }
  if (parts.length === 1) {
    return parts[0].slice(0, 2).toUpperCase();
  }
  return `${parts[0][0] ?? ''}${parts[1][0] ?? ''}`.toUpperCase();
}

function renderTrackOrdinal(track: ArchiveBrowserTrack) {
  if (track.discNumber && track.trackNumber) {
    return `${track.discNumber}.${String(track.trackNumber).padStart(2, '0')}`;
  }
  if (track.trackNumber) {
    return String(track.trackNumber).padStart(2, '0');
  }
  return '-';
}

function sortTracks(
  tracks: ArchiveBrowserTrack[],
  state: BrowserState,
  artistsBySlug: Map<string, ArchiveBrowserArtist>,
  albumsBySlug: Map<string, ArchiveBrowserAlbum>,
) {
  const sorted = [...tracks];
  const factor = state.dir === 'asc' ? 1 : -1;

  sorted.sort((left, right) => {
    let result = 0;

    switch (state.sort) {
      case 'track':
        result = compareNumbers(left.discNumber ?? 0, right.discNumber ?? 0)
          || compareNumbers(left.trackNumber ?? 0, right.trackNumber ?? 0)
          || compareStrings(left.name, right.name);
        break;
      case 'title':
        result = compareStrings(left.name, right.name);
        break;
      case 'artist':
        result = compareStrings(artistsBySlug.get(left.artistSlug)?.name ?? '', artistsBySlug.get(right.artistSlug)?.name ?? '')
          || compareStrings(left.name, right.name);
        break;
      case 'album':
        result = compareStrings(albumsBySlug.get(left.albumSlug)?.name ?? '', albumsBySlug.get(right.albumSlug)?.name ?? '')
          || compareStrings(left.name, right.name);
        break;
      case 'year':
        result = compareNumbers(left.year ?? 0, right.year ?? 0) || compareStrings(left.name, right.name);
        break;
      case 'plays':
        result = compareNumbers(left.playCount, right.playCount) || compareStrings(left.name, right.name);
        break;
      case 'dateAdded':
        result = compareNumbers(left.dateAddedUnix ?? 0, right.dateAddedUnix ?? 0) || compareStrings(left.name, right.name);
        break;
      case 'duration':
        result = compareNumbers(left.duration, right.duration) || compareStrings(left.name, right.name);
        break;
      default:
        result = compareStrings(left.name, right.name);
    }

    return result * factor;
  });

  return sorted;
}

function filterTracksBySearch(
  tracks: ArchiveBrowserTrack[],
  query: string,
  artistsBySlug: Map<string, ArchiveBrowserArtist>,
  albumsBySlug: Map<string, ArchiveBrowserAlbum>,
) {
  if (!query) {
    return tracks;
  }

  const lowered = query.toLowerCase();
  return tracks.filter((track) => {
    const artistName = artistsBySlug.get(track.artistSlug)?.name ?? '';
    const albumName = albumsBySlug.get(track.albumSlug)?.name ?? '';
    return `${track.name} ${artistName} ${albumName}`.toLowerCase().includes(lowered);
  });
}

function reconcileState(state: Partial<BrowserState>, lookups: any): BrowserState {
  const next: BrowserState = {
    genre: state.genre ?? '',
    artist: state.artist ?? '',
    album: state.album ?? '',
    q: state.q ?? '',
    sort: SORT_KEYS.has((state.sort as SortKey) ?? DEFAULT_STATE.sort) ? (state.sort as SortKey) : DEFAULT_STATE.sort,
    dir: state.dir === 'desc' ? 'desc' : 'asc',
  };

  if (next.genre && !lookups.genresBySlug.has(next.genre)) {
    next.genre = '';
  }
  if (next.artist && !lookups.artistsBySlug.has(next.artist)) {
    next.artist = '';
  }
  if (next.album && !lookups.albumsBySlug.has(next.album)) {
    next.album = '';
  }
  if (next.genre && next.artist) {
    const artist = lookups.artistsBySlug.get(next.artist);
    if (artist && !artist.genreSlugs.includes(next.genre)) {
      next.artist = '';
    }
  }
  if (next.genre && next.album) {
    const album = lookups.albumsBySlug.get(next.album);
    if (album && !album.genreSlugs.includes(next.genre)) {
      next.album = '';
    }
  }
  if (next.artist && next.album) {
    const album = lookups.albumsBySlug.get(next.album);
    if (album && !album.artistSlugs.includes(next.artist)) {
      next.album = '';
    }
  }

  return next;
}

function readStateFromUrl(search: string): Partial<BrowserState> {
  const params = new URLSearchParams(search);
  return {
    genre: params.get('genre') ?? '',
    artist: params.get('artist') ?? '',
    album: params.get('album') ?? '',
    q: params.get('q') ?? '',
    sort: (params.get('sort') as SortKey | null) ?? DEFAULT_STATE.sort,
    dir: (params.get('dir') as SortDirection | null) ?? DEFAULT_STATE.dir,
  };
}

function syncStateToUrl(state: BrowserState, libraryUrl: string, replace: boolean) {
  const nextUrl = buildShareHref(state, libraryUrl);
  const currentUrl = `${window.location.pathname}${window.location.search}`;
  if (currentUrl === nextUrl) {
    return;
  }
  if (replace) {
    window.history.replaceState({}, '', nextUrl);
    return;
  }
  window.history.pushState({}, '', nextUrl);
}

function buildShareHref(state: Partial<BrowserState>, libraryUrl: string) {
  const params = new URLSearchParams();
  if (state.genre) params.set('genre', state.genre);
  if (state.artist) params.set('artist', state.artist);
  if (state.album) params.set('album', state.album);
  if (state.q) params.set('q', state.q);
  if (state.sort && state.sort !== DEFAULT_STATE.sort) params.set('sort', state.sort);
  if (state.dir && state.dir !== DEFAULT_STATE.dir) params.set('dir', state.dir);

  const query = params.toString();
  return query ? `${libraryUrl}?${query}` : libraryUrl;
}

function buildLibraryHref(state: BrowserState, app: any, next: Partial<BrowserState>) {
  return buildShareHref(reconcileState({ ...state, ...next }, app), app.rootUrl ?? window.location.pathname);
}

function applyPaneSelection(state: BrowserState, pane: string, value: string) {
  if (pane === 'genre') {
    state.genre = value;
    state.artist = '';
    state.album = '';
    return;
  }
  if (pane === 'artist') {
    state.artist = value;
    state.album = '';
    return;
  }
  if (pane === 'album') {
    state.album = value;
  }
}

async function fetchImageMaps(dataRoot: string): Promise<ImageMaps> {
  const [albumImages, artistImages] = await Promise.all([
    fetchJson<{ byKey?: Record<string, string> }>(`${dataRoot}/album-images.json`).catch(() => ({ byKey: {} })),
    fetchJson<{ byArtistSlug?: Record<string, string> }>(`${dataRoot}/artist-images.json`).catch(() => ({ byArtistSlug: {} })),
  ]);

  return {
    albumImages: albumImages.byKey ?? {},
    artistImages: artistImages.byArtistSlug ?? {},
  };
}

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Failed to fetch ${url}: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function renderShell() {
  return `
    <section class="library-window">
      <header class="library-chrome">
        <div class="chrome-ledger">
          <div class="chrome-lights" aria-hidden="true">
            <span class="light red"></span>
            <span class="light amber"></span>
            <span class="light green"></span>
          </div>
          <div>
            <p class="chrome-kicker">MP3 Archive</p>
            <h1>Library Browser</h1>
          </div>
        </div>
        <p class="chrome-copy">A three-pane archive view tuned to feel closer to early iTunes than a directory of static detail pages.</p>
      </header>

      <section class="library-toolbar">
        <label class="library-search" for="library-search">
          <span>Search current slice</span>
          <div class="library-search-field">
            <input id="library-search" type="search" autocomplete="off" placeholder="Track, artist, or album" />
            <button id="library-search-reset" type="button" data-action="clear-search" hidden>Clear</button>
          </div>
        </label>
        <div class="library-summary-rail" id="library-summary-rail"></div>
      </section>

      <section class="pane-grid">
        <div id="genre-pane"></div>
        <div id="artist-pane"></div>
        <div id="album-pane"></div>
      </section>

      <section class="library-main">
        <div class="song-table-shell">
          <div class="song-table-topline" id="library-status"></div>
          <div class="song-header-row" id="song-header"></div>
          <div class="song-scroller" id="song-scroller">
            <div id="song-spacer"></div>
            <div class="song-rows" id="song-rows"></div>
          </div>
        </div>
        <aside class="library-inspector" id="library-inspector"></aside>
      </section>
    </section>
  `;
}

function renderFatal(message: string) {
  return `
    <section class="library-fatal">
      <p class="library-fatal-label">Archive Browser Offline</p>
      <h1>${escapeHtml(message)}</h1>
      <p>The static shell rendered, but the archive dataset was not available.</p>
    </section>
  `;
}

function escapeHtml(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function formatNumber(value: number) {
  return value.toLocaleString();
}

function formatDuration(seconds: number) {
  if (!seconds) {
    return '0m';
  }

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours === 0) {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${String(secs).padStart(2, '0')}`;
  }
  return `${hours}h ${minutes}m`;
}

function compareStrings(left: string, right: string) {
  return left.localeCompare(right, undefined, { sensitivity: 'base' });
}

function compareNumbers(left: number, right: number) {
  return left - right;
}

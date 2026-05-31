type SortKey = 'name' | 'time' | 'artist' | 'album' | 'genre' | 'rating' | 'playCount' | 'lastPlayed';
type SortDir = 'asc' | 'desc';
type InfoTab = 'summary' | 'info' | 'options' | 'artwork';

interface ITunesTrack {
  id: string;
  name: string;
  artist: string;
  composer: string;
  album: string;
  grouping: string;
  genre: string;
  size: number;
  duration: number;
  durationFormatted: string;
  discNumber?: number;
  discCount?: number;
  trackNumber?: number;
  trackCount?: number;
  year: string;
  dateModified: string;
  dateAdded: string;
  bitRate: string;
  sampleRate: string;
  volumeAdjustment: string;
  kind: string;
  equalizer: string;
  comments: string;
  playCount: number;
  lastPlayed: string;
  rating: number;
  location: string;
  sourceFile: string;
  sourceLine: number;
}

interface ITunesPlaylist {
  id: string;
  name: string;
  sourceFile: string;
  trackCount: number;
  trackIds: string[];
}

interface ITunesData {
  meta: {
    totalTracks: number;
    totalPlaylists: number;
    totalDurationSeconds: number;
    totalDurationFormatted: string;
    totalSizeBytes: number;
  };
  playlists: ITunesPlaylist[];
  tracks: ITunesTrack[];
}

interface Refs {
  sourceList: HTMLElement;
  genrePane: HTMLElement;
  artistPane: HTMLElement;
  albumPane: HTMLElement;
  songHeader: HTMLElement;
  songScroller: HTMLElement;
  songSpacer: HTMLElement;
  songRows: HTMLElement;
  searchInput: HTMLInputElement;
  lcd: HTMLElement;
  status: HTMLElement;
  modalRoot: HTMLElement;
}

interface AppState {
  sourceId: string;
  genre: string;
  artist: string;
  album: string;
  query: string;
  sort: SortKey;
  dir: SortDir;
  browseMode: boolean;
  selectedIds: Set<string>;
  playingId: string;
  playing: boolean;
  playPos: number;
  shuffleOn: boolean;
  repeatMode: number;
  eqOn: boolean;
  infoTrackId: string;
  infoTab: InfoTab;
  visibleTracks: ITunesTrack[];
}

interface App {
  data: ITunesData;
  tracksById: Map<string, ITunesTrack>;
  playlistsById: Map<string, ITunesPlaylist>;
  refs: Refs;
  state: AppState;
  playbackTimer: number | undefined;
}

const rowHeight = 20;
const rowOverscan = 10;
const colTemplate = '20px 240px 56px 170px 210px 108px 78px 74px 120px';
const allSourceId = 'library';

const columns: {
  id: SortKey;
  label: string;
  align?: 'left' | 'right' | 'center';
  get: (track: ITunesTrack) => string | number;
  sort: (a: ITunesTrack, b: ITunesTrack) => number;
}[] = [
  { id: 'name', label: 'Song Name', get: (track) => track.name, sort: textSort('name') },
  { id: 'time', label: 'Time', align: 'right', get: (track) => track.durationFormatted, sort: numSort('duration') },
  { id: 'artist', label: 'Artist', get: (track) => track.artist, sort: textSort('artist') },
  { id: 'album', label: 'Album', get: (track) => track.album, sort: textSort('album') },
  { id: 'genre', label: 'Genre', get: (track) => track.genre, sort: textSort('genre') },
  { id: 'rating', label: 'My Rating', align: 'center', get: (track) => track.rating, sort: numSort('rating') },
  { id: 'playCount', label: 'Play Count', align: 'center', get: (track) => track.playCount || '', sort: numSort('playCount') },
  { id: 'lastPlayed', label: 'Last Played', get: (track) => track.lastPlayed, sort: textSort('lastPlayed') },
];

export async function mountITunesBrowser(root: HTMLElement): Promise<void> {
  const dataUrl = root.dataset.dataUrl;
  const homeUrl = root.dataset.homeUrl ?? '/';

  if (!dataUrl) {
    root.innerHTML = renderBootError('Missing iTunes data URL.');
    return;
  }

  try {
    const data = await fetchJson<ITunesData>(dataUrl);
    root.innerHTML = renderShell(homeUrl);
    const refs = bindRefs(root);
    if (!refs) {
      root.innerHTML = renderBootError('iTunes could not initialize its interface.');
      return;
    }

    const app: App = {
      data,
      tracksById: new Map(data.tracks.map((track) => [track.id, track])),
      playlistsById: new Map(data.playlists.map((playlist) => [playlist.id, playlist])),
      refs,
      state: {
        sourceId: allSourceId,
        genre: '',
        artist: '',
        album: '',
        query: '',
        sort: 'artist',
        dir: 'asc',
        browseMode: true,
        selectedIds: new Set(),
        playingId: '',
        playing: false,
        playPos: 0,
        shuffleOn: false,
        repeatMode: 0,
        eqOn: false,
        infoTrackId: '',
        infoTab: 'summary',
        visibleTracks: [],
      },
      playbackTimer: undefined,
    };

    bindEvents(app);
    render(app);
  } catch (error) {
    console.error(error);
    root.innerHTML = renderBootError('iTunes library data failed to load. Run npm run build:itunes.');
  }
}

function bindRefs(root: HTMLElement): Refs | null {
  const sourceList = root.querySelector<HTMLElement>('#itunes-source-list');
  const genrePane = root.querySelector<HTMLElement>('#itunes-genre-pane');
  const artistPane = root.querySelector<HTMLElement>('#itunes-artist-pane');
  const albumPane = root.querySelector<HTMLElement>('#itunes-album-pane');
  const songHeader = root.querySelector<HTMLElement>('#itunes-song-header');
  const songScroller = root.querySelector<HTMLElement>('#itunes-song-scroller');
  const songSpacer = root.querySelector<HTMLElement>('#itunes-song-spacer');
  const songRows = root.querySelector<HTMLElement>('#itunes-song-rows');
  const searchInput = root.querySelector<HTMLInputElement>('#itunes-search');
  const lcd = root.querySelector<HTMLElement>('#itunes-lcd');
  const status = root.querySelector<HTMLElement>('#itunes-status');
  const modalRoot = root.querySelector<HTMLElement>('#itunes-modal-root');

  if (!sourceList || !genrePane || !artistPane || !albumPane || !songHeader || !songScroller || !songSpacer || !songRows || !searchInput || !lcd || !status || !modalRoot) {
    return null;
  }

  return {
    sourceList,
    genrePane,
    artistPane,
    albumPane,
    songHeader,
    songScroller,
    songSpacer,
    songRows,
    searchInput,
    lcd,
    status,
    modalRoot,
  };
}

function bindEvents(app: App) {
  const root = app.refs.sourceList.closest<HTMLElement>('.itunes-desktop');
  if (!root) return;

  app.refs.searchInput.addEventListener('input', () => {
    app.state.query = app.refs.searchInput.value.trim();
    render(app);
  });

  app.refs.songScroller.addEventListener('scroll', () => {
    renderTrackRows(app);
  });

  root.addEventListener('click', (event) => {
    const target = event.target instanceof Element ? event.target : null;
    if (!target) return;

    if (target.classList.contains('itunes-modal-shade')) {
      app.state.infoTrackId = '';
      renderInfoModal(app);
      return;
    }

    const source = target.closest<HTMLElement>('[data-source-id]');
    if (source) {
      app.state.sourceId = source.dataset.sourceId ?? allSourceId;
      app.state.genre = '';
      app.state.artist = '';
      app.state.album = '';
      app.state.selectedIds.clear();
      app.refs.songScroller.scrollTop = 0;
      render(app);
      return;
    }

    const pane = target.closest<HTMLElement>('[data-pane][data-value]');
    if (pane) {
      const kind = pane.dataset.pane;
      const value = pane.dataset.value ?? '';
      if (kind === 'genre') {
        app.state.genre = value;
        app.state.artist = '';
        app.state.album = '';
      } else if (kind === 'artist') {
        app.state.artist = value;
        app.state.album = '';
      } else if (kind === 'album') {
        app.state.album = value;
      }
      app.refs.songScroller.scrollTop = 0;
      render(app);
      return;
    }

    const sort = target.closest<HTMLElement>('[data-sort]');
    if (sort) {
      const nextSort = sort.dataset.sort as SortKey;
      if (columns.some((column) => column.id === nextSort)) {
        if (app.state.sort === nextSort) {
          app.state.dir = app.state.dir === 'asc' ? 'desc' : 'asc';
        } else {
          app.state.sort = nextSort;
          app.state.dir = 'asc';
        }
        app.refs.songScroller.scrollTop = 0;
        render(app);
      }
      return;
    }

    const action = target.closest<HTMLElement>('[data-action]');
    if (action) {
      handleAction(app, action.dataset.action ?? '');
      return;
    }

    const tab = target.closest<HTMLElement>('[data-info-tab]');
    if (tab) {
      app.state.infoTab = (tab.dataset.infoTab as InfoTab) || 'summary';
      renderInfoModal(app);
      return;
    }

    const row = target.closest<HTMLElement>('[data-track-id]');
    if (row) {
      const id = row.dataset.trackId ?? '';
      if (event.shiftKey || event.metaKey || event.ctrlKey) {
        if (app.state.selectedIds.has(id)) {
          app.state.selectedIds.delete(id);
        } else {
          app.state.selectedIds.add(id);
        }
      } else {
        app.state.selectedIds = new Set([id]);
      }
      updateRenderedTrackState(app);
    }
  });

  root.addEventListener('dblclick', (event) => {
    const target = event.target instanceof Element ? event.target : null;
    const row = target?.closest<HTMLElement>('[data-track-id]');
    if (!row) return;
    app.state.playingId = row.dataset.trackId ?? '';
    app.state.playing = true;
    app.state.playPos = 0;
    updatePlaybackTimer(app);
    render(app);
  });

  root.addEventListener('contextmenu', (event) => {
    const target = event.target instanceof Element ? event.target : null;
    const row = target?.closest<HTMLElement>('[data-track-id]');
    if (!row) return;
    event.preventDefault();
    app.state.infoTrackId = row.dataset.trackId ?? '';
    app.state.infoTab = 'summary';
    renderInfoModal(app);
  });
}

function handleAction(app: App, action: string) {
  if (action === 'play') {
    if (app.state.playingId) {
      app.state.playing = !app.state.playing;
    } else if (app.state.visibleTracks.length) {
      app.state.playingId = app.state.visibleTracks[0].id;
      app.state.playing = true;
      app.state.playPos = 0;
    }
    updatePlaybackTimer(app);
    render(app);
  } else if (action === 'next') {
    movePlayback(app, 1);
  } else if (action === 'prev') {
    movePlayback(app, -1);
  } else if (action === 'browse') {
    app.state.browseMode = !app.state.browseMode;
    render(app);
  } else if (action === 'shuffle') {
    app.state.shuffleOn = !app.state.shuffleOn;
    render(app);
  } else if (action === 'repeat') {
    app.state.repeatMode = (app.state.repeatMode + 1) % 3;
    render(app);
  } else if (action === 'eq') {
    app.state.eqOn = !app.state.eqOn;
    render(app);
  } else if (action === 'clear-search') {
    app.state.query = '';
    app.refs.searchInput.value = '';
    render(app);
  } else if (action === 'get-info') {
    const selected = [...app.state.selectedIds][0] || app.state.visibleTracks[0]?.id || '';
    app.state.infoTrackId = selected;
    app.state.infoTab = 'summary';
    renderInfoModal(app);
  } else if (action === 'close-info') {
    app.state.infoTrackId = '';
    renderInfoModal(app);
  }
}

function render(app: App) {
  const sourceTracks = getSourceTracks(app);
  const tracksAfterGenre = app.state.genre ? sourceTracks.filter((track) => paneValue(track.genre, 'none') === app.state.genre) : sourceTracks;
  const tracksAfterArtist = app.state.artist ? tracksAfterGenre.filter((track) => paneValue(track.artist, 'unknown') === app.state.artist) : tracksAfterGenre;
  const tracksAfterAlbum = app.state.album ? tracksAfterArtist.filter((track) => paneValue(track.album, 'unknown') === app.state.album) : tracksAfterArtist;

  app.state.visibleTracks = sortTracks(searchTracks(tracksAfterAlbum, app.state.query), app.state.sort, app.state.dir);
  app.refs.songSpacer.style.height = `${app.state.visibleTracks.length * rowHeight}px`;

  renderSourceList(app);
  renderBrowsePanes(app, sourceTracks, tracksAfterGenre, tracksAfterArtist);
  renderHeader(app);
  renderLcd(app);
  renderStatus(app);
  renderTrackRows(app);
  renderInfoModal(app);
}

function renderSourceList(app: App) {
  const playlistRows = app.data.playlists.map((playlist) => {
    const selected = app.state.sourceId === playlist.id ? ' selected' : '';
    return `<button class="itunes-source${selected}" type="button" data-source-id="${escapeAttr(playlist.id)}">
      <span class="itunes-source-icon">${iconPlaylist()}</span>
      <span class="itunes-source-name">${escapeHtml(playlist.name)}</span>
      <span class="itunes-source-count">${playlist.trackCount}</span>
    </button>`;
  });

  app.refs.sourceList.innerHTML = `
    <button class="itunes-source${app.state.sourceId === allSourceId ? ' selected' : ''}" type="button" data-source-id="${allSourceId}">
      <span class="itunes-source-icon">${iconLibrary()}</span>
      <span class="itunes-source-name">Library</span>
      <span class="itunes-source-count">${app.data.meta.totalTracks}</span>
    </button>
    <div class="itunes-source-divider"></div>
    ${playlistRows.join('')}
  `;
}

function renderBrowsePanes(app: App, sourceTracks: ITunesTrack[], tracksAfterGenre: ITunesTrack[], tracksAfterArtist: ITunesTrack[]) {
  if (!app.state.browseMode) {
    app.refs.genrePane.closest('.itunes-browse')?.classList.add('hidden');
    return;
  }

  app.refs.genrePane.closest('.itunes-browse')?.classList.remove('hidden');
  renderPane(app.refs.genrePane, 'genre', buildPaneValues(sourceTracks, (track) => paneValue(track.genre, 'none')), app.state.genre, 'Genres');
  renderPane(app.refs.artistPane, 'artist', buildPaneValues(tracksAfterGenre, (track) => paneValue(track.artist, 'unknown')), app.state.artist, 'Artists');
  renderPane(app.refs.albumPane, 'album', buildPaneValues(tracksAfterArtist, (track) => paneValue(track.album, 'unknown')), app.state.album, 'Albums');
}

function renderPane(node: HTMLElement, pane: string, values: string[], selected: string, label: string) {
  const allSelected = selected ? '' : ' selected';
  const rows = [`<button class="itunes-pane-row${allSelected}" type="button" data-pane="${pane}" data-value="">All (${values.length} ${label})</button>`];
  rows.push(...values.map((value) => `<button class="itunes-pane-row${selected === value ? ' selected' : ''}" type="button" data-pane="${pane}" data-value="${escapeAttr(value)}">${escapeHtml(value)}</button>`));
  node.innerHTML = rows.join('');
}

function renderHeader(app: App) {
  app.refs.songHeader.style.gridTemplateColumns = colTemplate;
  app.refs.songHeader.innerHTML = `
    <div class="itunes-head-cell center"></div>
    ${columns.map((column) => {
      const active = app.state.sort === column.id;
      const arrow = active ? (app.state.dir === 'asc' ? '&#9650;' : '&#9660;') : '';
      return `<button class="itunes-head-cell ${column.align ?? ''}${active ? ' active' : ''}" type="button" data-sort="${column.id}">
        <span>${column.label}</span><span class="itunes-sort-arrow">${arrow}</span>
      </button>`;
    }).join('')}
  `;
}

function renderTrackRows(app: App) {
  const { songScroller, songSpacer, songRows } = app.refs;
  const total = app.state.visibleTracks.length;
  const viewHeight = songScroller.clientHeight || 320;
  const start = Math.max(0, Math.floor(songScroller.scrollTop / rowHeight) - rowOverscan);
  const count = Math.ceil(viewHeight / rowHeight) + rowOverscan * 2;
  const end = Math.min(total, start + count);
  const rows = app.state.visibleTracks.slice(start, end);

  songSpacer.style.height = `${total * rowHeight}px`;
  songRows.style.transform = `translateY(${start * rowHeight}px)`;
  songRows.innerHTML = rows.map((track, index) => renderTrackRow(app, track, start + index)).join('');
}

function updateRenderedTrackState(app: App) {
  app.refs.songRows.querySelectorAll<HTMLElement>('[data-track-id]').forEach((row) => {
    const id = row.dataset.trackId ?? '';
    row.classList.toggle('selected', app.state.selectedIds.has(id));
    const indicator = row.querySelector<HTMLElement>('.itunes-track-cell.center');
    if (indicator) {
      if (app.state.playingId === id) {
        indicator.innerHTML = app.state.playing ? '&#9658;' : '&#9632;';
      } else {
        indicator.textContent = '';
      }
    }
  });
}

function renderTrackRow(app: App, track: ITunesTrack, index: number) {
  const selected = app.state.selectedIds.has(track.id);
  const playing = app.state.playingId === track.id;
  return `<div class="itunes-track-row${selected ? ' selected' : ''}" style="grid-template-columns:${colTemplate}" data-track-id="${escapeAttr(track.id)}">
    <div class="itunes-track-cell center">${playing ? (app.state.playing ? '&#9658;' : '&#9632;') : ''}</div>
    <div class="itunes-track-cell">${escapeHtml(track.name || '(no title)')}</div>
    <div class="itunes-track-cell right">${escapeHtml(track.durationFormatted)}</div>
    <div class="itunes-track-cell">${escapeHtml(track.artist || '(unknown)')}</div>
    <div class="itunes-track-cell">${escapeHtml(track.album || '(unknown)')}</div>
    <div class="itunes-track-cell">${escapeHtml(track.genre || '(none)')}</div>
    <div class="itunes-track-cell center stars">${stars(track.rating)}</div>
    <div class="itunes-track-cell center">${track.playCount || ''}</div>
    <div class="itunes-track-cell">${escapeHtml(track.lastPlayed)}</div>
  </div>`;
}

function renderLcd(app: App) {
  const track = app.state.playingId ? app.tracksById.get(app.state.playingId) : undefined;
  if (!track) {
    app.refs.lcd.innerHTML = `
      <div class="itunes-lcd-title">iTunes</div>
      <div class="itunes-lcd-subtitle">Double-click a track to play</div>
    `;
    return;
  }

  const pct = track.duration ? Math.min(100, (app.state.playPos / track.duration) * 100) : 0;
  app.refs.lcd.innerHTML = `
    <div class="itunes-lcd-title">${escapeHtml(track.name || '(no title)')}</div>
    <div class="itunes-lcd-subtitle">${escapeHtml(track.artist || '(unknown)')} &mdash; ${escapeHtml(track.album || '(unknown)')}</div>
    <div class="itunes-progress-row">
      <span>${formatTime(app.state.playPos)}</span>
      <div class="itunes-progress"><div class="itunes-progress-fill" style="width:${pct}%"></div></div>
      <span>-${formatTime(Math.max(0, track.duration - app.state.playPos))}</span>
    </div>
  `;
}

function renderStatus(app: App) {
  const songs = app.state.visibleTracks.length;
  const totalSeconds = app.state.visibleTracks.reduce((sum, track) => sum + track.duration, 0);
  const totalBytes = app.state.visibleTracks.reduce((sum, track) => sum + track.size, 0);
  app.refs.status.textContent = `${songs.toLocaleString()} songs, ${durationWords(totalSeconds)}, ${formatBytes(totalBytes)}`;
}

function renderInfoModal(app: App) {
  if (!app.state.infoTrackId) {
    app.refs.modalRoot.innerHTML = '';
    return;
  }

  const track = app.tracksById.get(app.state.infoTrackId);
  if (!track) {
    app.refs.modalRoot.innerHTML = '';
    return;
  }

  const albumTracks = app.data.tracks.filter((candidate) => candidate.artist === track.artist && candidate.album === track.album);
  const tabs: InfoTab[] = ['summary', 'info', 'options', 'artwork'];
  app.refs.modalRoot.innerHTML = `
    <div class="itunes-modal-shade">
      <div class="itunes-window itunes-info-window" role="dialog" aria-modal="true" aria-label="${escapeAttr(track.name)} Info">
        <div class="itunes-titlebar">
          <button class="itunes-title-button close" type="button" data-action="close-info" aria-label="Close"></button>
          <div class="itunes-title">${escapeHtml(track.name || 'Track')} Info</div>
          <div class="itunes-title-actions"><span class="itunes-title-button collapse"></span></div>
        </div>
        <div class="itunes-tabs">
          ${tabs.map((tab) => `<button class="itunes-tab${app.state.infoTab === tab ? ' active' : ''}" type="button" data-info-tab="${tab}">${capitalize(tab)}</button>`).join('')}
        </div>
        <div class="itunes-info-body">${renderInfoTab(app.state.infoTab, track, albumTracks.length)}</div>
        <div class="itunes-info-actions">
          <button class="platinum-btn" type="button" data-action="close-info">Cancel</button>
          <button class="platinum-btn default" type="button" data-action="close-info">OK</button>
        </div>
      </div>
    </div>
  `;
}

function renderInfoTab(tab: InfoTab, track: ITunesTrack, albumTrackCount: number) {
  if (tab === 'summary') {
    return `<div class="itunes-info-summary">
      <div class="itunes-art">${escapeHtml(albumArtText(track))}</div>
      <div>
        <h2>${escapeHtml(track.name || '(no title)')}</h2>
        <p>${escapeHtml(track.artist || '(unknown)')} &mdash; ${escapeHtml(track.album || '(unknown)')}</p>
        <table>
          <tbody>
            ${infoRow('Kind', track.kind || 'MPEG audio file')}
            ${infoRow('Size', formatBytes(track.size))}
            ${infoRow('Bit Rate', track.bitRate ? `${track.bitRate} kbps` : '')}
            ${infoRow('Sample Rate', track.sampleRate ? `${track.sampleRate} Hz` : '')}
            ${infoRow('Duration', track.durationFormatted)}
            ${infoRow('Play Count', String(track.playCount || 0))}
            ${infoRow('Last Played', track.lastPlayed || '-')}
            ${infoRow('Date Added', track.dateAdded)}
            ${infoRow('Playlist Export', track.sourceFile)}
            ${infoRow('Where', track.location, 'break')}
          </tbody>
        </table>
      </div>
    </div>`;
  }

  if (tab === 'info') {
    return `
      ${fieldRow('Name', track.name)}
      ${fieldRow('Artist', track.artist)}
      ${fieldRow('Composer', track.composer)}
      ${fieldRow('Album', track.album)}
      ${fieldRow('Track Number', `${track.trackNumber || ''}${albumTrackCount ? ` of ${albumTrackCount}` : ''}`)}
      ${fieldRow('Year', track.year)}
      ${fieldRow('Genre', track.genre)}
      ${fieldRow('Comments', track.comments, true)}
    `;
  }

  if (tab === 'options') {
    return `
      ${fieldRow('Volume Adjustment', track.volumeAdjustment || '0')}
      ${fieldRow('Equalizer Preset', track.equalizer || 'None')}
      ${fieldRow('My Rating', stars(track.rating), false, true)}
      ${fieldRow('Start Time', '0:00')}
      ${fieldRow('Stop Time', track.durationFormatted)}
    `;
  }

  return `<div class="itunes-artwork-tab">
    <div class="itunes-artwork-large">
      <strong>${escapeHtml(track.album || 'No Artwork')}</strong>
      <span>${escapeHtml(track.artist || '(unknown)')}</span>
    </div>
    <p>Drag artwork here</p>
  </div>`;
}

function getSourceTracks(app: App) {
  if (app.state.sourceId === allSourceId) return app.data.tracks;
  const playlist = app.playlistsById.get(app.state.sourceId);
  if (!playlist) return app.data.tracks;
  return playlist.trackIds.map((id) => app.tracksById.get(id)).filter((track): track is ITunesTrack => Boolean(track));
}

function searchTracks(tracks: ITunesTrack[], query: string) {
  const q = query.toLowerCase();
  if (!q) return tracks;
  return tracks.filter((track) => [
    track.name,
    track.artist,
    track.album,
    track.genre,
    track.composer,
    track.comments,
    track.location,
    track.sourceFile,
  ].some((value) => value.toLowerCase().includes(q)));
}

function sortTracks(tracks: ITunesTrack[], sort: SortKey, dir: SortDir) {
  const column = columns.find((item) => item.id === sort) ?? columns[0];
  const sorted = [...tracks].sort(column.sort);
  if (dir === 'desc') sorted.reverse();
  return sorted;
}

function buildPaneValues(tracks: ITunesTrack[], getValue: (track: ITunesTrack) => string) {
  return [...new Set(tracks.map(getValue))].sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }));
}

function paneValue(value: string, fallback: string) {
  const clean = value.trim();
  return clean || `(${fallback})`;
}

function movePlayback(app: App, step: number) {
  const visible = app.state.visibleTracks;
  const currentIndex = visible.findIndex((track) => track.id === app.state.playingId);
  const nextIndex = currentIndex < 0 ? 0 : Math.max(0, Math.min(visible.length - 1, currentIndex + step));
  const nextTrack = visible[nextIndex];
  if (!nextTrack) return;
  app.state.playingId = nextTrack.id;
  app.state.playing = true;
  app.state.playPos = 0;
  updatePlaybackTimer(app);
  render(app);
}

function updatePlaybackTimer(app: App) {
  window.clearInterval(app.playbackTimer);
  if (!app.state.playing || !app.state.playingId) return;
  app.playbackTimer = window.setInterval(() => {
    const track = app.tracksById.get(app.state.playingId);
    if (!track) return;
    if (app.state.playPos >= track.duration) {
      movePlayback(app, 1);
      return;
    }
    app.state.playPos += 1;
    renderLcd(app);
    renderTrackRows(app);
  }, 1000);
}

function renderShell(homeUrl: string) {
  return `<div class="itunes-desktop">
    <div class="itunes-menubar">
      <a class="itunes-apple" href="${escapeAttr(homeUrl)}" aria-label="Back to MP3 Collection">${iconApple()}</a>
      <span class="itunes-menu-item">File</span>
      <span class="itunes-menu-item">Edit</span>
      <span class="itunes-menu-item">Controls</span>
      <span class="itunes-menu-item">Visuals</span>
      <span class="itunes-menu-item">Advanced</span>
      <span class="itunes-menu-item">Help</span>
      <span class="itunes-menu-spacer"></span>
      <span class="itunes-clock">2:32 PM</span>
      <span class="itunes-app-badge"><span>&#9834;</span> iTunes</span>
    </div>
    <div class="itunes-window itunes-main-window">
      <div class="itunes-titlebar">
        <span class="itunes-title-button close"></span>
        <div class="itunes-title">iTunes</div>
        <div class="itunes-title-actions">
          <span class="itunes-title-button collapse"></span>
          <span class="itunes-title-button zoom"></span>
        </div>
      </div>
      <div class="itunes-player-row">
        <div class="itunes-transport">
          <button class="itunes-round-btn" type="button" data-action="prev" title="Previous">${iconBack()}</button>
          <button class="itunes-round-btn play" type="button" data-action="play" title="Play/Pause">${iconPlay()}</button>
          <button class="itunes-round-btn" type="button" data-action="next" title="Next">${iconFwd()}</button>
        </div>
        <div class="itunes-volume">${iconSpeakerSmall()}<div class="itunes-volume-track"><span></span></div>${iconSpeakerLarge()}</div>
        <div class="itunes-lcd bevel-in" id="itunes-lcd"></div>
        <div class="itunes-tools">
          <button class="itunes-icon-btn" type="button" data-action="eq" title="Equalizer">${iconEq()}</button>
          <button class="itunes-icon-btn" type="button" data-action="get-info" title="Get Info">${iconInfo()}</button>
          <button class="itunes-icon-btn" type="button" title="Eject">${iconEject()}</button>
          <label class="itunes-search">${iconSearch()}<input id="itunes-search" type="search" placeholder="Search" autocomplete="off" /><button type="button" data-action="clear-search" aria-label="Clear search">x</button></label>
        </div>
      </div>
      <div class="itunes-body">
        <aside class="itunes-sources" id="itunes-source-list" aria-label="iTunes playlists"></aside>
        <main class="itunes-track-area">
          <div class="itunes-browse">
            <section><h2>Genre</h2><div id="itunes-genre-pane"></div></section>
            <section><h2>Artist</h2><div id="itunes-artist-pane"></div></section>
            <section><h2>Album</h2><div id="itunes-album-pane"></div></section>
          </div>
          <div class="itunes-table">
            <div class="itunes-song-header" id="itunes-song-header"></div>
            <div class="itunes-song-scroller" id="itunes-song-scroller">
              <div id="itunes-song-spacer"></div>
              <div class="itunes-song-rows" id="itunes-song-rows"></div>
            </div>
          </div>
        </main>
      </div>
      <div class="itunes-statusbar">
        <button class="itunes-status-btn" type="button" title="New Playlist">+</button>
        <button class="itunes-status-btn" type="button" data-action="shuffle" title="Shuffle">${iconShuffle()}</button>
        <button class="itunes-status-btn" type="button" data-action="repeat" title="Repeat">${iconRepeat()}</button>
        <div class="itunes-status-summary" id="itunes-status"></div>
        <button class="itunes-status-btn active" type="button" data-action="browse" title="Browse">${iconBrowse()}</button>
      </div>
      <div class="itunes-grip"></div>
    </div>
    <div id="itunes-modal-root"></div>
  </div>`;
}

function textSort(key: keyof ITunesTrack) {
  return (a: ITunesTrack, b: ITunesTrack) => String(a[key] ?? '').localeCompare(String(b[key] ?? ''), undefined, { numeric: true, sensitivity: 'base' });
}

function numSort(key: keyof ITunesTrack) {
  return (a: ITunesTrack, b: ITunesTrack) => Number(a[key] ?? 0) - Number(b[key] ?? 0);
}

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url} returned ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function formatTime(seconds: number) {
  const safe = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(safe / 60);
  return `${minutes}:${String(safe % 60).padStart(2, '0')}`;
}

function durationWords(seconds: number) {
  const hours = seconds / 3600;
  if (hours < 1) return `${Math.floor(seconds / 60)} minutes`;
  if (hours < 24) return `${hours.toFixed(1)} hours`;
  return `${(hours / 24).toFixed(1)} days`;
}

function formatBytes(bytes: number) {
  if (!bytes) return '0 MB';
  const mb = bytes / 1024 / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
}

function stars(rating: number) {
  const count = Math.round((rating || 0) / 20);
  return Array.from({ length: 5 }, (_, index) => index < count ? '&#9733;' : '&#183;').join('');
}

function fieldRow(label: string, value: string, tall = false, raw = false) {
  return `<div class="itunes-info-row"><label>${label}</label><div class="itunes-info-value${tall ? ' tall' : ''}">${raw ? value : escapeHtml(value || '')}</div></div>`;
}

function infoRow(label: string, value: string, className = '') {
  return `<tr><td class="key">${label}</td><td class="${className}">${escapeHtml(value || '')}</td></tr>`;
}

function albumArtText(track: ITunesTrack) {
  return (track.album || 'No Artwork').split(/\s+/).slice(0, 3).join(' ');
}

function capitalize(value: string) {
  return `${value[0].toUpperCase()}${value.slice(1)}`;
}

function escapeHtml(value: string | number) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function escapeAttr(value: string | number) {
  return escapeHtml(value);
}

function renderBootError(message: string) {
  return `<div class="itunes-boot"><div class="itunes-boot-window"><strong>iTunes</strong><p>${escapeHtml(message)}</p></div></div>`;
}

function iconApple() {
  return '<svg viewBox="0 0 14 16" aria-hidden="true"><path fill="currentColor" d="M8.4.6c0 1.1-.85 2-1.85 2.05C6.6 1.55 7.35.6 8.4.6zm-.2 2.6c.9 0 1.55.65 2.3.65.7 0 1.35-.65 2.3-.65 1 0 1.95.55 2.5 1.45-2.2 1.25-1.85 4.4.4 5.25-.4 1-.85 1.95-1.65 2.7-.55.55-1.3 1.15-2.15 1.15-.75 0-1.05-.5-1.95-.5-.95 0-1.25.5-1.95.5-1 0-1.65-.7-2.25-1.35-1.55-1.75-1.95-4.6-.7-6.3.6-.8 1.55-1.35 2.55-1.35.95 0 1.55.65 2.6.45z"/></svg>';
}

function iconPlay() {
  return '<svg viewBox="0 0 14 14" aria-hidden="true"><polygon points="3,2 12,7 3,12" fill="currentColor"/></svg>';
}

function iconBack() {
  return '<svg viewBox="0 0 14 14" aria-hidden="true"><polygon points="11,2 3,7 11,12" fill="currentColor"/><rect x="2" y="2" width="1.5" height="10" fill="currentColor"/></svg>';
}

function iconFwd() {
  return '<svg viewBox="0 0 14 14" aria-hidden="true"><polygon points="3,2 11,7 3,12" fill="currentColor"/><rect x="10.5" y="2" width="1.5" height="10" fill="currentColor"/></svg>';
}

function iconSpeakerSmall() {
  return '<svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0,3.5 L0,6.5 L2.5,6.5 L5,8.5 L5,1.5 L2.5,3.5 Z" fill="currentColor"/></svg>';
}

function iconSpeakerLarge() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><path d="M0,4 L0,8 L2.5,8 L5,10.5 L5,1.5 L2.5,4 Z" fill="currentColor"/><path d="M6.5,3 Q9,6 6.5,9M7.5,1.5 Q11,6 7.5,10.5" stroke="currentColor" stroke-width="1" fill="none"/></svg>';
}

function iconSearch() {
  return '<svg viewBox="0 0 11 11" aria-hidden="true"><circle cx="4.5" cy="4.5" r="3" stroke="currentColor" stroke-width="1.2" fill="none"/><line x1="6.7" y1="6.7" x2="10" y2="10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>';
}

function iconLibrary() {
  return '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="2" y="2" width="3" height="12" fill="#ffaa44" stroke="#553300"/><rect x="6" y="2" width="3" height="12" fill="#66aaee" stroke="#003366"/><rect x="10" y="2" width="3" height="12" fill="#cc4444" stroke="#660000"/></svg>';
}

function iconPlaylist() {
  return '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="2" y="2" width="11" height="12" fill="#f8f4d0" stroke="#000"/><line x1="4" y1="5" x2="11" y2="5" stroke="#555"/><line x1="4" y1="7" x2="11" y2="7" stroke="#555"/><line x1="4" y1="9" x2="11" y2="9" stroke="#555"/><line x1="4" y1="11" x2="9" y2="11" stroke="#555"/></svg>';
}

function iconEq() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><rect x="1.5" y="2" width="1.5" height="8" fill="currentColor"/><rect x="5" y="4" width="1.5" height="6" fill="currentColor"/><rect x="8.5" y="1" width="1.5" height="9" fill="currentColor"/></svg>';
}

function iconInfo() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><circle cx="6" cy="6" r="5" fill="none" stroke="currentColor"/><rect x="5.4" y="5" width="1.2" height="4" fill="currentColor"/><rect x="5.4" y="2.6" width="1.2" height="1.2" fill="currentColor"/></svg>';
}

function iconEject() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><polygon points="6,2 11,8 1,8" fill="currentColor"/><rect x="1" y="9" width="10" height="1.5" fill="currentColor"/></svg>';
}

function iconShuffle() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><path d="M0 3 L3 3 L9 9 L12 9 M9 9 L11 7 M9 9 L11 11M0 9 L3 9 L9 3 L12 3 M9 3 L11 1 M9 3 L11 5" stroke="currentColor" stroke-width="1.2" fill="none"/></svg>';
}

function iconRepeat() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2 4 Q2 2 4 2 L10 2 M8 0 L10 2 L8 4M10 8 Q10 10 8 10 L2 10 M4 12 L2 10 L4 8" stroke="currentColor" stroke-width="1.2" fill="none"/></svg>';
}

function iconBrowse() {
  return '<svg viewBox="0 0 12 12" aria-hidden="true"><circle cx="6" cy="6" r="3" fill="none" stroke="currentColor" stroke-width="1.2"/><circle cx="6" cy="6" r="1" fill="currentColor"/></svg>';
}

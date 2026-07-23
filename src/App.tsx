import { useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle,
  Check,
  ChevronDown,
  CircleDot,
  Clock3,
  Database,
  Film,
  History,
  LayoutGrid,
  List,
  ListFilter,
  LoaderCircle,
  Menu,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Star,
  Tags,
  Trash2,
  WandSparkles,
  X,
} from 'lucide-react'
import { api } from './api'
import { DetailModal } from './components/DetailModal'
import { Modal } from './components/Modal'
import { PlayerModal } from './components/PlayerModal'
import { PosterRecord, TextRecordRow } from './components/RecordViews'
import { TimelineView } from './components/TimelineView'
import {
  duplicateMessage,
  filterLibraryRecords,
  newlyDuplicatedMediaRefs,
  sameMediaRef,
  type MediaTypeFilter,
  type MetadataFilter,
  type SortKey,
} from './recordFilters'
import { statusMeta } from './status'
import type { AppConfig, MediaRef, PlayLink, RecordInput, RecordItem, Snapshot, Status, TmdbResult } from './types'

const emptyInput = (): RecordInput => ({
  status: 'planned', title: '', mediaRef: null, completedAt: null, createdAt: today(), rating: null,
  progress: null, tags: [], comment: null,
})

export default function App() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<Status | 'all'>('all')
  const [sort, setSort] = useState<SortKey>('file')
  const [metadataFilter, setMetadataFilter] = useState<MetadataFilter>('all')
  const [mediaTypeFilter, setMediaTypeFilter] = useState<MediaTypeFilter>('all')
  const [genreFilter, setGenreFilter] = useState('all')
  const [page, setPage] = useState<'library' | 'timeline'>('library')
  const [viewMode, setViewMode] = useState<'text' | 'poster'>(() => (localStorage.getItem('traker-view') as 'text' | 'poster') || 'text')
  const [detail, setDetail] = useState<RecordItem | null>(null)
  const [editor, setEditor] = useState<{ record?: RecordItem } | null>(null)
  const [matching, setMatching] = useState<RecordItem | null>(null)
  const [deleting, setDeleting] = useState<RecordItem | null>(null)
  const [mobileNav, setMobileNav] = useState(false)
  const [externalChange, setExternalChange] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [quickTitle, setQuickTitle] = useState('')
  const [quickSaving, setQuickSaving] = useState(false)
  const [batchMatching, setBatchMatching] = useState(false)
  const [batchSummary, setBatchSummary] = useState('')
  const [duplicateWarning, setDuplicateWarning] = useState('')
  const [player, setPlayer] = useState<{ title: string; url: string; mediaId: string; posterUrl?: string; serverName: string } | null>(null)

  const load = async () => {
    try { setSnapshot(await api.records()); setError(''); setExternalChange(false) }
    catch (cause) { setError(messageOf(cause)) }
    finally { setLoading(false) }
  }

  useEffect(() => { void load() }, [])
  useEffect(() => {
    const events = new EventSource('/api/events')
    events.addEventListener('changed', (event) => {
      const revision = JSON.parse((event as MessageEvent).data).revision
      if (snapshot && revision !== snapshot.revision) {
        if (editor || matching) setExternalChange(true)
        else { setDetail(null); void load() }
      }
    })
    return () => events.close()
  }, [snapshot?.revision, editor, matching])

  const records = useMemo(() => {
    if (!snapshot) return []
    return filterLibraryRecords(snapshot.records, {
      query,
      status,
      metadata: metadataFilter,
      mediaType: mediaTypeFilter,
      genre: genreFilter,
      sort,
    })
  }, [snapshot, query, status, metadataFilter, mediaTypeFilter, genreFilter, sort])

  const counts = useMemo(() => {
    const base = { all: 0, planned: 0, watching: 0, watched: 0, dropped: 0 }
    snapshot?.records.forEach((record) => { base.all++; base[record.status]++ })
    return base
  }, [snapshot])

  const metadataCounts = useMemo(() => ({
    unmatched: snapshot?.records.filter((record) => !record.mediaRef).length || 0,
    issue: snapshot?.records.filter((record) => record.metadataState === 'missing' || record.metadataState === 'invalid').length || 0,
  }), [snapshot])

  const genres = useMemo(() => Array.from(new Set(snapshot?.records.flatMap((record) => record.metadata?.genres || []) || [])).sort((a, b) => a.localeCompare(b, 'zh-CN')), [snapshot])
  const timelineCount = useMemo(() => snapshot?.records.filter((record) => (record.status === 'watched' || record.status === 'dropped') && record.completedAt).length || 0, [snapshot])

  const handleMutation = (next: Snapshot) => { setSnapshot(next); setDetail(null); setEditor(null); setMatching(null); setDeleting(null); setExternalChange(false) }
  const handleTmdbMatch = (next: Snapshot, mediaRef: MediaRef) => {
    const matches = next.records.filter((record) => sameMediaRef(record.mediaRef, mediaRef))
    setDuplicateWarning(matches.length > 1 ? duplicateMessage([mediaRef]) : '')
    handleMutation(next)
  }
  const changeView = (mode: 'text' | 'poster') => { setViewMode(mode); localStorage.setItem('traker-view', mode) }
  const searchActor = (actor: string) => {
    setQuery(actor)
    setStatus('all')
    setMetadataFilter('all')
    setMediaTypeFilter('all')
    setGenreFilter('all')
    setPage('library')
    setDetail(null)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
  const handleFailure = (cause: unknown) => {
    if ((cause as { status?: number }).status === 409) { setExternalChange(true); return }
    setError(messageOf(cause))
  }

  const quickAdd = async (event: React.FormEvent) => {
    event.preventDefault()
    const title = quickTitle.trim()
    if (!snapshot || !title || quickSaving) return
    setQuickSaving(true)
    try {
      const next = await api.create(snapshot.revision, { ...emptyInput(), title })
      const created = next.records.at(-1)
      setSnapshot(next)
      setQuickTitle('')
      setError('')
      setExternalChange(false)
      if (created) setMatching(created)
    } catch (cause) {
      handleFailure(cause)
    } finally {
      setQuickSaving(false)
    }
  }

  const autoMatch = async () => {
    if (!snapshot || (metadataCounts.unmatched === 0 && metadataCounts.issue === 0) || batchMatching) return
    setBatchMatching(true)
    setBatchSummary('')
    setDuplicateWarning('')
    try {
      const refreshResult = await api.refreshMissingMetadata(snapshot.revision)
      const matchResult = await api.autoMatchTmdb(refreshResult.snapshot.revision)
      const newDuplicates = newlyDuplicatedMediaRefs(snapshot.records, matchResult.snapshot.records)
      setSnapshot(matchResult.snapshot)
      setError('')
      setExternalChange(false)
      if (newDuplicates.length > 0) setDuplicateWarning(duplicateMessage(newDuplicates))
      const parts = [`补全元数据 ${refreshResult.refreshed} 条`, `自动匹配 ${matchResult.matched} 条`]
      if (matchResult.noResults.length) parts.push(`无结果 ${matchResult.noResults.length} 条`)
      const failed = refreshResult.failed.length + matchResult.failed.length
      if (failed) parts.push(`失败 ${failed} 条`)
      setBatchSummary(`批量处理完成：${parts.join('，')}。`)
    } catch (cause) {
      handleFailure(cause)
    } finally {
      setBatchMatching(false)
    }
  }

  return (
    <div className="app-shell">
      <Sidebar counts={counts} active={page === 'library' ? status : null} timelineCount={timelineCount} onSelect={(value) => { setPage('library'); setStatus(value); setMobileNav(false) }} onTimeline={() => { setPage('timeline'); setMobileNav(false) }} open={mobileNav} onClose={() => setMobileNav(false)} onSettings={() => { setSettingsOpen(true); setMobileNav(false) }} />
      <main className="main-content">
        <header className="topbar">
          <button className="icon-button mobile-menu" onClick={() => setMobileNav(true)} aria-label="打开导航"><Menu size={20} /></button>
          <div>
            <h1>{page === 'timeline' ? '观看时间线' : '我的片单'}</h1>
            <p>{page === 'timeline' ? `${timelineCount} 条有日期的观看记录` : `${counts.all} 条记录 · 本地文本已连接`}</p>
          </div>
          {page === 'library' && <button className="primary-button" onClick={() => setEditor({})}><Plus size={18} /> 添加记录</button>}
        </header>

        {externalChange && <Notice text="数据文件已在其他位置更新，请刷新后继续编辑。" action="刷新" onAction={() => { setDetail(null); setEditor(null); setMatching(null); void load() }} />}
        {error && <Notice text={error} action="重试" onAction={() => void load()} tone="error" />}
        {batchSummary && <Notice text={batchSummary} action="关闭" onAction={() => setBatchSummary('')} tone="success" />}
        {duplicateWarning && <Notice text={duplicateWarning} action="关闭" onAction={() => setDuplicateWarning('')} />}

        {page === 'library' ? <><form className="quick-add" onSubmit={quickAdd}>
          <Plus size={18} />
          <input value={quickTitle} onChange={(event) => setQuickTitle(event.target.value)} placeholder="输入片名，按回车快速添加" aria-label="快速添加影视记录" />
          <button type="submit" disabled={!snapshot || !quickTitle.trim() || quickSaving}>{quickSaving ? <LoaderCircle className="spin" size={17} /> : <span>添加并匹配 TMDB</span>}</button>
        </form>

        <section className="stats-strip" aria-label="片单统计">
          <Stat label="全部" value={counts.all} icon={<Film size={18} />} active={status === 'all'} onClick={() => setStatus('all')} />
          <Stat label="想看" value={counts.planned} icon={<Clock3 size={18} />} active={status === 'planned'} onClick={() => setStatus('planned')} />
          <Stat label="在看" value={counts.watching} icon={<CircleDot size={18} />} active={status === 'watching'} onClick={() => setStatus('watching')} />
          <Stat label="看过" value={counts.watched} icon={<Check size={18} />} active={status === 'watched'} onClick={() => setStatus('watched')} />
        </section>

        <section className="toolbar">
          <label className="search-box"><Search size={18} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、演员、标签或短评" /></label>
          <label className="select-box"><ListFilter size={17} /><select value={sort} onChange={(event) => setSort(event.target.value as SortKey)}><option value="file">文件顺序</option><option value="completed">完成日期</option><option value="created">创建日期</option><option value="rating">个人评分</option></select><ChevronDown size={15} /></label>
          <label className="select-box metadata-select"><Database size={17} /><select value={metadataFilter} onChange={(event) => setMetadataFilter(event.target.value as MetadataFilter)}><option value="all">全部元数据</option><option value="unmatched">未匹配 ({metadataCounts.unmatched})</option><option value="issue">缺失或异常 ({metadataCounts.issue})</option></select><ChevronDown size={15} /></label>
          <label className="select-box media-type-select"><Film size={17} /><select value={mediaTypeFilter} onChange={(event) => setMediaTypeFilter(event.target.value as MediaTypeFilter)}><option value="all">全部类型</option><option value="tm">电影</option><option value="tv">剧集</option></select><ChevronDown size={15} /></label>
          <label className="select-box genre-select"><Tags size={17} /><select value={genreFilter} onChange={(event) => setGenreFilter(event.target.value)}><option value="all">全部题材</option>{genres.map((genre) => <option key={genre} value={genre}>{genre}</option>)}</select><ChevronDown size={15} /></label>
          <div className="view-switch" aria-label="列表显示模式"><button className={viewMode === 'text' ? 'active' : ''} onClick={() => changeView('text')} title="文字模式"><List size={17} /><span>文字</span></button><button className={viewMode === 'poster' ? 'active' : ''} onClick={() => changeView('poster')} title="海报模式"><LayoutGrid size={17} /><span>海报</span></button></div>
          <button className="secondary-button auto-match-button" onClick={() => void autoMatch()} disabled={batchMatching || (metadataCounts.unmatched === 0 && metadataCounts.issue === 0)} title="先补全缺失或异常的元数据，再为未匹配记录采用 TMDB 第一条搜索结果">{batchMatching ? <LoaderCircle className="spin" size={17} /> : <WandSparkles size={17} />}<span>{batchMatching ? '处理中' : '批量匹配'}</span></button>
        </section>

        {loading ? <Loading /> : records.length === 0 ? <Empty hasQuery={Boolean(query || status !== 'all' || metadataFilter !== 'all' || mediaTypeFilter !== 'all' || genreFilter !== 'all')} onAdd={() => setEditor({})} /> : (
          <section className={viewMode === 'poster' ? 'poster-grid' : 'record-list'} aria-label="影视记录">
            {records.map((record) => viewMode === 'poster' ? <PosterRecord key={record.key} record={record} onOpen={() => setDetail(record)} /> : <TextRecordRow key={record.key} record={record} onOpen={() => setDetail(record)} />)}
          </section>
        )}</> : loading ? <Loading /> : <TimelineView records={snapshot?.records || []} onOpen={setDetail} />}
      </main>

      {detail && snapshot && <DetailModal record={detail} onClose={() => setDetail(null)} onEdit={() => { setDetail(null); setEditor({ record: detail }) }} onMatch={() => { setDetail(null); setMatching(detail) }} onRefresh={async () => { try { const next = await api.refreshTmdb(snapshot.revision, detail.key); setSnapshot(next); setDetail(next.records.find((item) => item.key === detail.key) || detail) } catch (cause) { handleFailure(cause) } }} onDelete={() => { setDetail(null); setDeleting(detail) }} onSearchActor={searchActor} onResolvePlayLink={() => api.playLink(detail.mediaRef!)} onPlay={(link: PlayLink) => {
        const url = link.redirectedUrl || link.playUrl
        setPlayer({ title: link.itemName || detail.title, url, mediaId: `${detail.mediaRef!.type}:${detail.mediaRef!.id}`, posterUrl: detail.metadata?.posterUrl, serverName: link.serverName })
      }} />}
      {editor && snapshot && <EditorModal record={editor.record} revision={snapshot.revision} changed={externalChange} onClose={() => setEditor(null)} onSaved={handleMutation} onError={handleFailure} />}
      {matching && snapshot && <TmdbModal record={matching} revision={snapshot.revision} onClose={() => setMatching(null)} onSaved={handleTmdbMatch} onError={handleFailure} />}
      {deleting && snapshot && <ConfirmDelete record={deleting} onClose={() => setDeleting(null)} onConfirm={async () => { try { handleMutation(await api.remove(snapshot.revision, deleting.key)) } catch (cause) { handleFailure(cause) } }} />}
      {settingsOpen && <SettingsModal onClose={() => setSettingsOpen(false)} />}
      {player && <PlayerModal {...player} onClose={() => setPlayer(null)} />}
    </div>
  )
}

function Sidebar({ counts, active, timelineCount, onSelect, onTimeline, open, onClose, onSettings }: { counts: Record<string, number>; active: Status | 'all' | null; timelineCount: number; onSelect: (value: Status | 'all') => void; onTimeline: () => void; open: boolean; onClose: () => void; onSettings: () => void }) {
  const items: Array<{ value: Status | 'all'; label: string; icon: React.ReactNode }> = [
    { value: 'all', label: '全部记录', icon: <Film size={18} /> }, { value: 'planned', label: '想看', icon: <Clock3 size={18} /> },
    { value: 'watching', label: '在看', icon: <CircleDot size={18} /> }, { value: 'watched', label: '看过', icon: <Check size={18} /> },
    { value: 'dropped', label: '弃看', icon: <X size={18} /> },
  ]
  return <><button className={`nav-scrim ${open ? 'visible' : ''}`} onClick={onClose} aria-label="关闭导航" /><aside className={`sidebar ${open ? 'open' : ''}`}>
    <div className="brand"><div className="brand-mark"><Film size={21} /></div><span>Traker</span><button className="icon-button nav-close" onClick={onClose} aria-label="关闭导航"><X size={19} /></button></div>
    <nav><p className="nav-label">片单</p>{items.map((item) => <button key={item.value} className={active === item.value ? 'active' : ''} onClick={() => onSelect(item.value)}>{item.icon}<span>{item.label}</span><b>{counts[item.value]}</b></button>)}<p className="nav-label nav-label-secondary">浏览</p><button className={active === null ? 'active' : ''} onClick={onTimeline}><History size={18} /><span>观看时间线</span><b>{timelineCount}</b></button></nav>
    <div className="sidebar-bottom"><div className="sidebar-footer"><span className="connection-dot" /> 文本数据已同步</div><button className="settings-button" onClick={onSettings}><Settings size={16} />设置</button></div>
  </aside></>
}

function Stat({ label, value, icon, active, onClick }: { label: string; value: number; icon: React.ReactNode; active: boolean; onClick: () => void }) {
  return <button className={active ? 'active' : ''} onClick={onClick}><span className="stat-icon">{icon}</span><span><small>{label}</small><strong>{value}</strong></span></button>
}

function EditorModal({ record, revision, changed, onClose, onSaved, onError }: { record?: RecordItem; revision: string; changed: boolean; onClose: () => void; onSaved: (value: Snapshot) => void; onError: (cause: unknown) => void }) {
  const [form, setForm] = useState<RecordInput>(record ? toInput(record) : emptyInput())
  const [tags, setTags] = useState(record?.tags.join(' ') || '')
  const [saving, setSaving] = useState(false)
  const [confirmed, setConfirmed] = useState(!record?.warnings.length)
  const update = <K extends keyof RecordInput>(key: K, value: RecordInput[K]) => setForm((current) => ({ ...current, [key]: value }))
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); if (changed || !confirmed) return; setSaving(true)
    const payload = { ...form, title: form.title.trim(), tags: tags.split(/[\s,，]+/).filter(Boolean) }
    try { onSaved(record ? await api.update(revision, record.key, payload) : await api.create(revision, payload)) }
    catch (cause) { onError(cause) } finally { setSaving(false) }
  }
  return <Modal title={record ? '编辑记录' : '添加记录'} onClose={onClose}>
    <form onSubmit={submit} className="editor-form">
      <div className="status-control">{(Object.keys(statusMeta) as Status[]).map((item) => <button type="button" key={item} className={form.status === item ? 'active' : ''} onClick={() => { update('status', item); if (item !== 'watched' && item !== 'dropped') update('completedAt', null) }}><span>{statusMeta[item].symbol}</span>{statusMeta[item].label}</button>)}</div>
      <Field label="标题" wide><input autoFocus required value={form.title} onChange={(event) => update('title', event.target.value)} placeholder="电影或剧集名称" /></Field>
      <div className="form-grid"><Field label="创建日期"><input type="date" value={toHtmlDate(form.createdAt)} onChange={(event) => update('createdAt', fromHtmlDate(event.target.value))} /></Field>{(form.status === 'watched' || form.status === 'dropped') && <Field label={form.status === 'watched' ? '完成日期' : '结束日期'}><input type="date" value={toHtmlDate(form.completedAt)} onChange={(event) => update('completedAt', fromHtmlDate(event.target.value))} /></Field>}<Field label="观看进度"><input value={form.progress || ''} onChange={(event) => update('progress', event.target.value || null)} placeholder="S03E02" /></Field></div>
      <Field label="个人评分" wide><div className="rating-input">{[1, 2, 3, 4, 5].map((value) => <button type="button" key={value} className={(form.rating || 0) >= value ? 'selected' : ''} onClick={() => update('rating', form.rating === value ? null : value)}><Star size={22} fill="currentColor" /></button>)}</div></Field>
      <Field label="标签" wide><input value={tags} onChange={(event) => setTags(event.target.value)} placeholder="科幻 经典 日剧" /><small>用空格分隔多个标签</small></Field>
      <Field label="短评" wide><textarea rows={3} value={form.comment || ''} onChange={(event) => update('comment', event.target.value || null)} placeholder="写下简短感受" /></Field>
      {record && record.warnings.length > 0 && <label className="warning-confirm"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /><span><AlertTriangle size={17} />此行有格式警告。保存会把该行改写为标准格式，我已确认原文：<code>{record.rawLine}</code></span></label>}
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>取消</button><button type="submit" className="primary-button" disabled={saving || changed || !form.title.trim() || !confirmed}>{saving ? <LoaderCircle className="spin" size={18} /> : <Check size={18} />}{record ? '保存修改' : '添加到片单'}</button></div>
    </form>
  </Modal>
}

function TmdbModal({ record, revision, onClose, onSaved, onError }: { record: RecordItem; revision: string; onClose: () => void; onSaved: (value: Snapshot, mediaRef: MediaRef) => void; onError: (cause: unknown) => void }) {
  const [query, setQuery] = useState(record.title)
  const [results, setResults] = useState<TmdbResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const search = async (event?: React.FormEvent) => {
    event?.preventDefault()
    const keyword = query.trim()
    setError('')
    if (!keyword) {
      setResults([])
      setError('请输入要搜索的电影或剧集名称。')
      return
    }
    setLoading(true)
    try {
      const next = await api.searchTmdb(keyword)
      setResults(next)
      if (next.length === 0) setError(`TMDB 没有找到“${keyword}”的匹配结果。`)
    } catch (cause) {
      setResults([])
      setError(messageOf(cause))
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => { void search() }, [])
  return <Modal title="匹配 TMDB" onClose={onClose} wide><form className="tmdb-search" onSubmit={search}><label className="search-box"><Search size={18} /><input value={query} onChange={(event) => setQuery(event.target.value)} /></label><button className="secondary-button" disabled={loading}>{loading ? <LoaderCircle className="spin" size={18} /> : <Search size={18} />}搜索</button></form>{error && <p className="inline-error">{error}</p>}<div className="tmdb-results">{results.map((item) => <article key={`${item.type}-${item.id}`}><div className="poster">{item.posterPath ? <img src={`https://image.tmdb.org/t/p/w185${item.posterPath}`} alt="" /> : <Film size={26} />}</div><div><h3>{item.title}</h3><p className="result-meta">{item.type === 'tv' ? '剧集' : '电影'} · {item.date?.slice(0, 4) || '年份未知'} {item.voteAverage > 0 && `· TMDB ${item.voteAverage.toFixed(1)}`}</p><p className="overview">{item.overview || '暂无简介'}</p></div><button className="secondary-button" onClick={async () => { const mediaRef: MediaRef = { type: item.type === 'tv' ? 'tv' : 'tm', id: item.id }; try { onSaved(await api.matchTmdb(revision, record.key, mediaRef.type, mediaRef.id), mediaRef) } catch (cause) { onError(cause) } }}>选择</button></article>)}</div></Modal>
}

function SettingsModal({ onClose }: { onClose: () => void }) {
  const [config, setConfig] = useState<AppConfig | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    api.config().then(setConfig).catch((cause) => setError(messageOf(cause)))
  }, [])
  return <Modal title="设置" onClose={onClose}><div className="settings-content"><div className="settings-row"><span>当前数据文件</span>{config ? <code>{config.dataFile}</code> : error ? <p className="inline-error">{error}</p> : <span className="settings-loading"><LoaderCircle className="spin" size={16} />正在读取</span>}</div><p className="settings-hint">数据文件路径由启动参数、TRAKER_DATA_FILE 或 .env 配置。</p></div></Modal>
}

function ConfirmDelete({ record, onClose, onConfirm }: { record: RecordItem; onClose: () => void; onConfirm: () => void }) { return <Modal title="删除记录" onClose={onClose}><div className="confirm-body"><div className="danger-icon"><Trash2 size={22} /></div><p>确定删除“<strong>{record.title}</strong>”吗？写入前会自动保留备份。</p><div className="modal-actions"><button className="secondary-button" onClick={onClose}>取消</button><button className="danger-button" onClick={onConfirm}><Trash2 size={17} />删除记录</button></div></div></Modal> }
function Field({ label, wide, children }: { label: string; wide?: boolean; children: React.ReactNode }) { return <label className={`field ${wide ? 'wide' : ''}`}><span>{label}</span>{children}</label> }
function Notice({ text, action, onAction, tone }: { text: string; action: string; onAction: () => void; tone?: 'error' | 'success' }) { return <div className={`notice ${tone || ''}`}>{tone === 'success' ? <Check size={18} /> : <AlertTriangle size={18} />}<span>{text}</span><button onClick={onAction}>{action}{tone !== 'success' && <RefreshCw size={15} />}</button></div> }
function Loading() { return <div className="loading"><LoaderCircle className="spin" size={26} /><span>正在读取片单</span></div> }
function Empty({ hasQuery, onAdd }: { hasQuery: boolean; onAdd: () => void }) { return <div className="empty"><div><Film size={26} /></div><h2>{hasQuery ? '没有符合条件的记录' : '片单还是空的'}</h2><p>{hasQuery ? '试试更换关键词或筛选条件。' : '添加第一部想看的电影或剧集。'}</p>{!hasQuery && <button className="primary-button" onClick={onAdd}><Plus size={18} />添加记录</button>}</div> }
function toInput(record: RecordItem): RecordInput { return { status: record.status, title: record.title, mediaRef: record.mediaRef, completedAt: record.completedAt, createdAt: record.createdAt, rating: record.rating, progress: record.progress, tags: record.tags, comment: record.comment } }
function today() { const date = new Date(); return `${date.getFullYear()}.${String(date.getMonth() + 1).padStart(2, '0')}.${String(date.getDate()).padStart(2, '0')}` }
function toHtmlDate(value: string | null) { return value?.replaceAll('.', '-') || '' }
function fromHtmlDate(value: string) { return value ? value.replaceAll('-', '.') : null }
function messageOf(cause: unknown) { return cause instanceof Error ? cause.message : '发生未知错误' }

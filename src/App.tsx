import { useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle,
  CalendarDays,
  Check,
  ChevronDown,
  CircleDot,
  Clock3,
  Film,
  LayoutGrid,
  Link2,
  List,
  ListFilter,
  LoaderCircle,
  Menu,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Star,
  Trash2,
  Tv,
  X,
} from 'lucide-react'
import { api } from './api'
import type { RecordInput, RecordItem, Snapshot, Status, TmdbResult } from './types'

const statusMeta: Record<Status, { label: string; symbol: string; className: string }> = {
  planned: { label: '想看', symbol: '−', className: 'status-planned' },
  watching: { label: '在看', symbol: '›', className: 'status-watching' },
  watched: { label: '看过', symbol: '✓', className: 'status-watched' },
  dropped: { label: '弃看', symbol: '×', className: 'status-dropped' },
}

type SortKey = 'file' | 'completed' | 'created' | 'rating'

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
  const [viewMode, setViewMode] = useState<'text' | 'poster'>(() => (localStorage.getItem('traker-view') as 'text' | 'poster') || 'text')
  const [detail, setDetail] = useState<RecordItem | null>(null)
  const [editor, setEditor] = useState<{ record?: RecordItem } | null>(null)
  const [matching, setMatching] = useState<RecordItem | null>(null)
  const [deleting, setDeleting] = useState<RecordItem | null>(null)
  const [mobileNav, setMobileNav] = useState(false)
  const [externalChange, setExternalChange] = useState(false)

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
    const normalized = query.trim().toLocaleLowerCase()
    const result = snapshot.records.filter((record) => {
      if (status !== 'all' && record.status !== status) return false
      if (!normalized) return true
      return [record.title, record.comment || '', ...record.tags].some((value) => value.toLocaleLowerCase().includes(normalized))
    })
    return [...result].sort((a, b) => {
      if (sort === 'rating') return (b.rating || 0) - (a.rating || 0)
      if (sort === 'created') return (b.createdAt || '').localeCompare(a.createdAt || '')
      if (sort === 'completed') return (b.completedAt || '').localeCompare(a.completedAt || '')
      return a.lineNumber - b.lineNumber
    })
  }, [snapshot, query, status, sort])

  const counts = useMemo(() => {
    const base = { all: 0, planned: 0, watching: 0, watched: 0, dropped: 0 }
    snapshot?.records.forEach((record) => { base.all++; base[record.status]++ })
    return base
  }, [snapshot])

  const handleMutation = (next: Snapshot) => { setSnapshot(next); setDetail(null); setEditor(null); setMatching(null); setDeleting(null); setExternalChange(false) }
  const changeView = (mode: 'text' | 'poster') => { setViewMode(mode); localStorage.setItem('traker-view', mode) }
  const handleFailure = (cause: unknown) => {
    if ((cause as { status?: number }).status === 409) { setExternalChange(true); return }
    setError(messageOf(cause))
  }

  return (
    <div className="app-shell">
      <Sidebar counts={counts} active={status} onSelect={(value) => { setStatus(value); setMobileNav(false) }} open={mobileNav} onClose={() => setMobileNav(false)} />
      <main className="main-content">
        <header className="topbar">
          <button className="icon-button mobile-menu" onClick={() => setMobileNav(true)} aria-label="打开导航"><Menu size={20} /></button>
          <div>
            <h1>我的片单</h1>
            <p>{counts.all} 条记录 · 本地文本已连接</p>
          </div>
          <button className="primary-button" onClick={() => setEditor({})}><Plus size={18} /> 添加记录</button>
        </header>

        {externalChange && <Notice text="数据文件已在其他位置更新，请刷新后继续编辑。" action="刷新" onAction={() => { setDetail(null); setEditor(null); setMatching(null); void load() }} />}
        {error && <Notice text={error} action="重试" onAction={() => void load()} tone="error" />}

        <section className="stats-strip" aria-label="片单统计">
          <Stat label="全部" value={counts.all} icon={<Film size={18} />} active={status === 'all'} onClick={() => setStatus('all')} />
          <Stat label="想看" value={counts.planned} icon={<Clock3 size={18} />} active={status === 'planned'} onClick={() => setStatus('planned')} />
          <Stat label="在看" value={counts.watching} icon={<CircleDot size={18} />} active={status === 'watching'} onClick={() => setStatus('watching')} />
          <Stat label="看过" value={counts.watched} icon={<Check size={18} />} active={status === 'watched'} onClick={() => setStatus('watched')} />
        </section>

        <section className="toolbar">
          <label className="search-box"><Search size={18} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、标签或短评" /></label>
          <label className="select-box"><ListFilter size={17} /><select value={sort} onChange={(event) => setSort(event.target.value as SortKey)}><option value="file">文件顺序</option><option value="completed">完成日期</option><option value="created">创建日期</option><option value="rating">个人评分</option></select><ChevronDown size={15} /></label>
          <div className="view-switch" aria-label="列表显示模式"><button className={viewMode === 'text' ? 'active' : ''} onClick={() => changeView('text')} title="文字模式"><List size={17} /><span>文字</span></button><button className={viewMode === 'poster' ? 'active' : ''} onClick={() => changeView('poster')} title="海报模式"><LayoutGrid size={17} /><span>海报</span></button></div>
        </section>

        {loading ? <Loading /> : records.length === 0 ? <Empty hasQuery={Boolean(query || status !== 'all')} onAdd={() => setEditor({})} /> : (
          <section className={viewMode === 'poster' ? 'poster-grid' : 'record-list'} aria-label="影视记录">
            {records.map((record) => viewMode === 'poster' ? <PosterRecord key={record.key} record={record} onOpen={() => setDetail(record)} /> : <TextRecordRow key={record.key} record={record} onOpen={() => setDetail(record)} />)}
          </section>
        )}
      </main>

      {detail && snapshot && <DetailModal record={detail} onClose={() => setDetail(null)} onEdit={() => { setDetail(null); setEditor({ record: detail }) }} onMatch={() => { setDetail(null); setMatching(detail) }} onRefresh={async () => { try { const next = await api.refreshTmdb(snapshot.revision, detail.key); setSnapshot(next); setDetail(next.records.find((item) => item.key === detail.key) || detail) } catch (cause) { handleFailure(cause) } }} onDelete={() => { setDetail(null); setDeleting(detail) }} />}
      {editor && snapshot && <EditorModal record={editor.record} revision={snapshot.revision} changed={externalChange} onClose={() => setEditor(null)} onSaved={handleMutation} onError={handleFailure} />}
      {matching && snapshot && <TmdbModal record={matching} revision={snapshot.revision} onClose={() => setMatching(null)} onSaved={handleMutation} onError={handleFailure} />}
      {deleting && snapshot && <ConfirmDelete record={deleting} onClose={() => setDeleting(null)} onConfirm={async () => { try { handleMutation(await api.remove(snapshot.revision, deleting.key)) } catch (cause) { handleFailure(cause) } }} />}
    </div>
  )
}

function Sidebar({ counts, active, onSelect, open, onClose }: { counts: Record<string, number>; active: Status | 'all'; onSelect: (value: Status | 'all') => void; open: boolean; onClose: () => void }) {
  const items: Array<{ value: Status | 'all'; label: string; icon: React.ReactNode }> = [
    { value: 'all', label: '全部记录', icon: <Film size={18} /> }, { value: 'planned', label: '想看', icon: <Clock3 size={18} /> },
    { value: 'watching', label: '在看', icon: <CircleDot size={18} /> }, { value: 'watched', label: '看过', icon: <Check size={18} /> },
    { value: 'dropped', label: '弃看', icon: <X size={18} /> },
  ]
  return <><button className={`nav-scrim ${open ? 'visible' : ''}`} onClick={onClose} aria-label="关闭导航" /><aside className={`sidebar ${open ? 'open' : ''}`}>
    <div className="brand"><div className="brand-mark"><Film size={21} /></div><span>Traker</span><button className="icon-button nav-close" onClick={onClose} aria-label="关闭导航"><X size={19} /></button></div>
    <nav><p className="nav-label">片单</p>{items.map((item) => <button key={item.value} className={active === item.value ? 'active' : ''} onClick={() => onSelect(item.value)}>{item.icon}<span>{item.label}</span><b>{counts[item.value]}</b></button>)}</nav>
    <div className="sidebar-footer"><span className="connection-dot" /> 文本数据已同步</div>
  </aside></>
}

function Stat({ label, value, icon, active, onClick }: { label: string; value: number; icon: React.ReactNode; active: boolean; onClick: () => void }) {
  return <button className={active ? 'active' : ''} onClick={onClick}><span className="stat-icon">{icon}</span><span><small>{label}</small><strong>{value}</strong></span></button>
}

function TextRecordRow({ record, onOpen }: { record: RecordItem; onOpen: () => void }) {
  const meta = statusMeta[record.status]
  const date = record.completedAt || record.createdAt || record.metadata?.releaseDate
  return <button className="record-row" onClick={onOpen}>
    <span className={`status-mark ${meta.className}`} title={meta.label}>{meta.symbol}</span>
    <span className="text-record-content">
      <span className="text-record-title">{record.title || '未命名记录'}{record.mediaRef && <span className="media-pill">{record.mediaRef.type === 'tv' ? <Tv size={13} /> : <Film size={13} />}{record.mediaRef.type}:{record.mediaRef.id}</span>}{record.warnings.length > 0 && <span className="warning-pill"><AlertTriangle size={13} />{record.warnings.length}</span>}</span>
      <span className="text-record-meta"><b className={meta.className}>{meta.label}</b>{record.progress && <span>{record.progress}</span>}{date && <span><CalendarDays size={13} />{date}</span>}{record.rating && <span className="rating"><Star size={13} fill="currentColor" />{record.rating}</span>}{record.tags.slice(0, 3).map((tag) => <span className="tag" key={tag}>#{tag}</span>)}{record.comment && <span className="text-comment">“{record.comment}”</span>}</span>
    </span>
    <span className="row-open"><ChevronDown size={17} /></span>
  </button>
}

function PosterRecord({ record, onOpen }: { record: RecordItem; onOpen: () => void }) {
  const meta = statusMeta[record.status]
  return <button className="poster-record" onClick={onOpen}>
    <span className="poster-record-image">{record.metadata?.posterUrl ? <img src={record.metadata.posterUrl} alt="" /> : <span className="poster-placeholder"><Film size={30} /><small>{record.mediaRef ? '刷新 TMDB 详情' : '尚未匹配 TMDB'}</small></span>}<span className={`poster-status ${meta.className}`}>{meta.symbol}</span>{record.rating && <span className="poster-rating"><Star size={12} fill="currentColor" />{record.rating}</span>}</span>
    <span className="poster-record-title">{record.title || '未命名记录'}</span>
  </button>
}

function DetailModal({ record, onClose, onEdit, onMatch, onRefresh, onDelete }: { record: RecordItem; onClose: () => void; onEdit: () => void; onMatch: () => void; onRefresh: () => Promise<void>; onDelete: () => void }) {
  const [refreshing, setRefreshing] = useState(false)
  const meta = statusMeta[record.status]
  const refresh = async () => { setRefreshing(true); await onRefresh(); setRefreshing(false) }
  return <Modal title="影片详情" onClose={onClose} wide>
    <div className="detail-layout">
      <div className="detail-poster">{record.metadata?.posterUrl ? <img src={record.metadata.posterUrl} alt={`${record.title}海报`} /> : <div><Film size={36} /><span>暂无海报</span></div>}</div>
      <div className="detail-main">
        <div className="detail-heading"><span className={`detail-status ${meta.className}`}>{meta.symbol}</span><div><h2>{record.title || '未命名记录'}</h2>{record.metadata?.originalTitle && record.metadata.originalTitle !== record.title && <p>{record.metadata.originalTitle}</p>}</div></div>
        <div className="detail-facts"><span className={meta.className}>{meta.label}</span>{record.mediaRef && <span>{record.mediaRef.type === 'tv' ? '剧集' : '电影'} · {record.mediaRef.type}:{record.mediaRef.id}</span>}{record.metadata?.releaseDate && <span>{record.metadata.releaseDate}</span>}{record.metadata && record.metadata.voteAverage > 0 && <span>TMDB {record.metadata.voteAverage.toFixed(1)}</span>}</div>
        {record.metadata?.overview && <p className="detail-overview">{record.metadata.overview}</p>}
        {record.metadata?.cast && record.metadata.cast.length > 0 && <div className="detail-section"><h3>主要演员</h3><p>{record.metadata.cast.join(' · ')}</p></div>}
        <div className="detail-section"><h3>我的记录</h3><div className="personal-facts">{record.rating && <span className="rating"><Star size={14} fill="currentColor" />{record.rating} / 5</span>}{record.progress && <span>进度 {record.progress}</span>}{record.createdAt && <span>创建于 {record.createdAt}</span>}{record.completedAt && <span>{record.status === 'dropped' ? '结束于' : '完成于'} {record.completedAt}</span>}</div>{record.tags.length > 0 && <div className="detail-tags">{record.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>}{record.comment && <blockquote>{record.comment}</blockquote>}</div>
        {record.warnings.length > 0 && <div className="detail-warning"><AlertTriangle size={16} /><span>{record.warnings.map((warning) => warning.message).join('；')}</span></div>}
      </div>
    </div>
    <div className="detail-actions"><button className="danger-text-button" onClick={onDelete}><Trash2 size={16} />删除</button><div><button className="secondary-button" onClick={onMatch}><Link2 size={16} />匹配 TMDB</button>{record.mediaRef && <button className="secondary-button" onClick={() => void refresh()} disabled={refreshing}>{refreshing ? <LoaderCircle className="spin" size={16} /> : <RefreshCw size={16} />}刷新详情</button>}<button className="primary-button" onClick={onEdit}><Pencil size={16} />编辑记录</button></div></div>
  </Modal>
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

function TmdbModal({ record, revision, onClose, onSaved, onError }: { record: RecordItem; revision: string; onClose: () => void; onSaved: (value: Snapshot) => void; onError: (cause: unknown) => void }) {
  const [query, setQuery] = useState(record.title)
  const [results, setResults] = useState<TmdbResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const search = async (event?: React.FormEvent) => { event?.preventDefault(); setLoading(true); setError(''); try { setResults(await api.searchTmdb(query)) } catch (cause) { setError(messageOf(cause)) } finally { setLoading(false) } }
  useEffect(() => { void search() }, [])
  return <Modal title="匹配 TMDB" onClose={onClose} wide><form className="tmdb-search" onSubmit={search}><label className="search-box"><Search size={18} /><input value={query} onChange={(event) => setQuery(event.target.value)} /></label><button className="secondary-button" disabled={loading}>{loading ? <LoaderCircle className="spin" size={18} /> : <Search size={18} />}搜索</button></form>{error && <p className="inline-error">{error}</p>}<div className="tmdb-results">{results.map((item) => <article key={`${item.type}-${item.id}`}><div className="poster">{item.posterPath ? <img src={`https://image.tmdb.org/t/p/w185${item.posterPath}`} alt="" /> : <Film size={26} />}</div><div><h3>{item.title}</h3><p className="result-meta">{item.type === 'tv' ? '剧集' : '电影'} · {item.date?.slice(0, 4) || '年份未知'} {item.voteAverage > 0 && `· TMDB ${item.voteAverage.toFixed(1)}`}</p><p className="overview">{item.overview || '暂无简介'}</p></div><button className="secondary-button" onClick={async () => { try { onSaved(await api.matchTmdb(revision, record.key, item.type === 'tv' ? 'tv' : 'tm', item.id)) } catch (cause) { onError(cause) } }}>选择</button></article>)}</div></Modal>
}

function ConfirmDelete({ record, onClose, onConfirm }: { record: RecordItem; onClose: () => void; onConfirm: () => void }) { return <Modal title="删除记录" onClose={onClose}><div className="confirm-body"><div className="danger-icon"><Trash2 size={22} /></div><p>确定删除“<strong>{record.title}</strong>”吗？写入前会自动保留备份。</p><div className="modal-actions"><button className="secondary-button" onClick={onClose}>取消</button><button className="danger-button" onClick={onConfirm}><Trash2 size={17} />删除记录</button></div></div></Modal> }
function Modal({ title, onClose, wide, children }: { title: string; onClose: () => void; wide?: boolean; children: React.ReactNode }) { return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}><section className={`modal ${wide ? 'modal-wide' : ''}`} role="dialog" aria-modal="true" aria-label={title}><header><h2>{title}</h2><button className="icon-button" onClick={onClose} aria-label="关闭"><X size={19} /></button></header>{children}</section></div> }
function Field({ label, wide, children }: { label: string; wide?: boolean; children: React.ReactNode }) { return <label className={`field ${wide ? 'wide' : ''}`}><span>{label}</span>{children}</label> }
function Notice({ text, action, onAction, tone }: { text: string; action: string; onAction: () => void; tone?: 'error' }) { return <div className={`notice ${tone || ''}`}><AlertTriangle size={18} /><span>{text}</span><button onClick={onAction}>{action}<RefreshCw size={15} /></button></div> }
function Loading() { return <div className="loading"><LoaderCircle className="spin" size={26} /><span>正在读取片单</span></div> }
function Empty({ hasQuery, onAdd }: { hasQuery: boolean; onAdd: () => void }) { return <div className="empty"><div><Film size={26} /></div><h2>{hasQuery ? '没有符合条件的记录' : '片单还是空的'}</h2><p>{hasQuery ? '试试更换关键词或筛选条件。' : '添加第一部想看的电影或剧集。'}</p>{!hasQuery && <button className="primary-button" onClick={onAdd}><Plus size={18} />添加记录</button>}</div> }
function toInput(record: RecordItem): RecordInput { return { status: record.status, title: record.title, mediaRef: record.mediaRef, completedAt: record.completedAt, createdAt: record.createdAt, rating: record.rating, progress: record.progress, tags: record.tags, comment: record.comment } }
function today() { const date = new Date(); return `${date.getFullYear()}.${String(date.getMonth() + 1).padStart(2, '0')}.${String(date.getDate()).padStart(2, '0')}` }
function toHtmlDate(value: string | null) { return value?.replaceAll('.', '-') || '' }
function fromHtmlDate(value: string) { return value ? value.replaceAll('-', '.') : null }
function messageOf(cause: unknown) { return cause instanceof Error ? cause.message : '发生未知错误' }

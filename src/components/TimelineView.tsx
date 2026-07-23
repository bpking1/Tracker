import { useMemo, useState } from 'react'
import { AlertTriangle, CalendarDays, ChevronDown, Film, History, ListFilter, Search, Star, Tags, Tv } from 'lucide-react'
import { filterTimelineRecords, groupTimelineRecords, type TimelineStatusFilter } from '../recordFilters'
import { statusMeta } from '../status'
import type { RecordItem } from '../types'

export function TimelineView({ records, onOpen }: { records: RecordItem[]; onOpen: (record: RecordItem) => void }) {
  const [query, setQuery] = useState('')
  const [year, setYear] = useState('all')
  const [status, setStatus] = useState<TimelineStatusFilter>('all')
  const [genre, setGenre] = useState('all')

  const dated = useMemo(() => records.filter((record) => (record.status === 'watched' || record.status === 'dropped') && record.completedAt), [records])
  const years = useMemo(() => Array.from(new Set(dated.map((record) => record.completedAt!.slice(0, 4)))).sort((a, b) => b.localeCompare(a)), [dated])
  const genres = useMemo(() => Array.from(new Set(dated.flatMap((record) => record.metadata?.genres || []))).sort((a, b) => a.localeCompare(b, 'zh-CN')), [dated])
  const undated = useMemo(() => records.filter((record) => (record.status === 'watched' || record.status === 'dropped') && !record.completedAt).length, [records])
  const filtered = useMemo(() => filterTimelineRecords(records, { query, year, status, genre }), [records, query, year, status, genre])
  const groups = useMemo(() => groupTimelineRecords(filtered), [filtered])
  const watched = filtered.filter((record) => record.status === 'watched').length
  const dropped = filtered.length - watched

  return <section className="timeline-page">
    <div className="timeline-stats" aria-label="时间线统计"><span><strong>{filtered.length}</strong><small>当前记录</small></span><span><strong>{watched}</strong><small>看过</small></span><span><strong>{dropped}</strong><small>弃看</small></span></div>
    <div className="timeline-toolbar">
      <label className="search-box"><Search size={18} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索时间线中的标题、演员、题材或短评" /></label>
      <label className="select-box"><CalendarDays size={17} /><select value={year} onChange={(event) => setYear(event.target.value)}><option value="all">全部年份</option>{years.map((item) => <option key={item} value={item}>{item} 年</option>)}</select><ChevronDown size={15} /></label>
      <label className="select-box"><ListFilter size={17} /><select value={status} onChange={(event) => setStatus(event.target.value as TimelineStatusFilter)}><option value="all">看过与弃看</option><option value="watched">只看看过</option><option value="dropped">只看弃看</option></select><ChevronDown size={15} /></label>
      <label className="select-box genre-select"><Tags size={17} /><select value={genre} onChange={(event) => setGenre(event.target.value)}><option value="all">全部题材</option>{genres.map((item) => <option key={item} value={item}>{item}</option>)}</select><ChevronDown size={15} /></label>
    </div>

    {groups.length === 0 ? <div className="timeline-empty"><History size={28} /><h2>没有符合条件的观看记录</h2><p>时间线只展示填写了完成或结束日期的看过、弃看记录。</p></div> : <div className="timeline-groups">
      {groups.map((yearGroup) => <section className="timeline-year" key={yearGroup.year}>
        <header><h2>{yearGroup.year}</h2><span>{yearGroup.months.reduce((total, month) => total + month.items.length, 0)} 条记录</span></header>
        {yearGroup.months.map((monthGroup) => <section className="timeline-month" key={`${yearGroup.year}-${monthGroup.month}`}>
          <h3>{Number(monthGroup.month)} 月</h3>
          <div className="timeline-track">
            {monthGroup.items.map((record) => {
              const meta = statusMeta[record.status]
              return <button className="timeline-entry" key={record.key} onClick={() => onOpen(record)}>
                <time dateTime={record.completedAt!.replaceAll('.', '-')}><strong>{Number(record.completedAt!.slice(8, 10))}</strong><small>日</small></time>
                <span className={`timeline-dot ${meta.className}`} />
                <span className="timeline-poster">{record.metadata?.posterUrl ? <img src={record.metadata.posterUrl} alt="" /> : <Film size={20} />}</span>
                <span className="timeline-entry-main">
                  <span className="timeline-entry-title">{record.title || '未命名记录'}</span>
                  <span className="timeline-entry-meta"><b className={meta.className}>{meta.label}</b>{record.mediaRef && <span>{record.mediaRef.type === 'tv' ? <Tv size={12} /> : <Film size={12} />}{record.mediaRef.type === 'tv' ? '剧集' : '电影'}</span>}{record.rating && <span className="rating"><Star size={12} fill="currentColor" />{record.rating}</span>}{record.metadata?.genres.slice(0, 2).map((item) => <span className="timeline-genre" key={item}>{item}</span>)}</span>
                  {record.comment && <span className="timeline-comment">“{record.comment}”</span>}
                </span>
                <ChevronDown className="timeline-open" size={17} />
              </button>
            })}
          </div>
        </section>)}
      </section>)}
    </div>}
    {undated > 0 && <p className="timeline-undated"><AlertTriangle size={15} />另有 {undated} 条看过或弃看记录没有完成/结束日期，未显示在时间线中。</p>}
  </section>
}

import { AlertTriangle, CalendarDays, ChevronDown, Film, Star, Tv } from 'lucide-react'
import { statusMeta } from '../status'
import type { RecordItem } from '../types'

export function TextRecordRow({ record, onOpen }: { record: RecordItem; onOpen: () => void }) {
  const meta = statusMeta[record.status]
  const date = record.completedAt || record.createdAt || record.metadata?.releaseDate
  return <button className="record-row" onClick={onOpen}>
    <span className={`status-mark ${meta.className}`} title={meta.label}>{meta.symbol}</span>
    <span className="text-record-content">
      <span className="text-record-title">{record.title || '未命名记录'}{record.mediaRef && <span className="media-pill">{record.mediaRef.type === 'tv' ? <Tv size={13} /> : <Film size={13} />}{record.mediaRef.type}:{record.mediaRef.id}</span>}{record.warnings.length > 0 && <span className="warning-pill"><AlertTriangle size={13} />{record.warnings.length}</span>}</span>
      <span className="text-record-meta"><b className={meta.className}>{meta.label}</b>{record.progress && <span>{record.progress}</span>}{date && <span><CalendarDays size={13} />{date}</span>}{record.rating && <span className="rating"><Star size={13} fill="currentColor" />{record.rating}</span>}{record.tags.slice(0, 3).map((tag) => <span className="tag" key={tag}>+{tag}</span>)}{record.comment && <span className="text-comment">“{record.comment}”</span>}</span>
    </span>
    <span className="row-open"><ChevronDown size={17} /></span>
  </button>
}

export function PosterRecord({ record, onOpen }: { record: RecordItem; onOpen: () => void }) {
  const meta = statusMeta[record.status]
  return <button className="poster-record" onClick={onOpen}>
    <span className="poster-record-image">{record.metadata?.posterUrl ? <img src={record.metadata.posterUrl} alt="" /> : <span className="poster-placeholder"><Film size={30} /><small>{record.mediaRef ? '刷新 TMDB 详情' : '尚未匹配 TMDB'}</small></span>}<span className={`poster-status ${meta.className}`}>{meta.symbol}</span>{record.rating && <span className="poster-rating"><Star size={12} fill="currentColor" />{record.rating}</span>}</span>
    <span className="poster-record-title">{record.title || '未命名记录'}</span>
  </button>
}

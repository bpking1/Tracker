import { useState } from 'react'
import { AlertTriangle, Film, Link2, LoaderCircle, Pencil, Play, RefreshCw, Star, Trash2 } from 'lucide-react'
import { statusMeta } from '../status'
import type { PlayLink, RecordItem } from '../types'
import { Modal } from './Modal'

interface DetailModalProps {
  record: RecordItem
  onClose: () => void
  onEdit: () => void
  onMatch: () => void
  onRefresh: () => Promise<void>
  onDelete: () => void
  onSearchActor: (actor: string) => void
  onPlay: () => Promise<PlayLink>
}

export function DetailModal({ record, onClose, onEdit, onMatch, onRefresh, onDelete, onSearchActor, onPlay }: DetailModalProps) {
  const [refreshing, setRefreshing] = useState(false)
  const [playing, setPlaying] = useState(false)
  const [playError, setPlayError] = useState('')
  const meta = statusMeta[record.status]
  const refresh = async () => { setRefreshing(true); await onRefresh(); setRefreshing(false) }
  const play = async () => {
    const popup = window.open('about:blank', '_blank')
    if (popup) popup.opener = null
    setPlaying(true)
    setPlayError('')
    try {
      const link = await onPlay()
      const target = link.redirectedUrl || link.playUrl
      if (!target) throw new Error('Emby 没有返回播放地址')
      if (!popup) throw new Error('浏览器阻止了播放窗口，请允许此站点打开新窗口')
      popup.location.replace(target)
    } catch (cause) {
      popup?.close()
      setPlayError(cause instanceof Error ? cause.message : '无法获取 Emby 播放地址')
    } finally {
      setPlaying(false)
    }
  }
  return <Modal title="影片详情" onClose={onClose} wide>
    <div className="detail-layout">
      <div className="detail-poster">{record.metadata?.posterUrl ? <img src={record.metadata.posterUrl} alt={`${record.title}海报`} /> : <div><Film size={36} /><span>暂无海报</span></div>}</div>
      <div className="detail-main">
        <div className="detail-heading"><span className={`detail-status ${meta.className}`}>{meta.symbol}</span><div><h2>{record.title || '未命名记录'}</h2>{record.metadata?.originalTitle && record.metadata.originalTitle !== record.title && <p>{record.metadata.originalTitle}</p>}</div></div>
        <div className="detail-facts"><span className={meta.className}>{meta.label}</span>{record.mediaRef && <span>{record.mediaRef.type === 'tv' ? '剧集' : '电影'} · {record.mediaRef.type}:{record.mediaRef.id}</span>}{record.metadata?.releaseDate && <span>{record.metadata.releaseDate}</span>}{record.metadata && record.metadata.voteAverage > 0 && <span>TMDB {record.metadata.voteAverage.toFixed(1)}</span>}</div>
        {record.metadata?.overview && <p className="detail-overview">{record.metadata.overview}</p>}
        {record.metadata?.genres && record.metadata.genres.length > 0 && <div className="detail-section"><h3>类型题材</h3><div className="detail-tags">{record.metadata.genres.map((genre) => <span key={genre}>{genre}</span>)}</div></div>}
        {record.metadata?.cast && record.metadata.cast.length > 0 && <div className="detail-section"><h3>主要演员</h3><div className="cast-links">{record.metadata.cast.map((actor) => <button type="button" key={actor} onClick={() => onSearchActor(actor)} title={`搜索 ${actor} 的作品`}>{actor}</button>)}</div></div>}
        <div className="detail-section"><h3>我的记录</h3><div className="personal-facts">{record.rating && <span className="rating"><Star size={14} fill="currentColor" />{record.rating} / 5</span>}{record.progress && <span>进度 {record.progress}</span>}{record.createdAt && <span>创建于 {record.createdAt}</span>}{record.completedAt && <span>{record.status === 'dropped' ? '结束于' : '完成于'} {record.completedAt}</span>}</div>{record.tags.length > 0 && <div className="detail-tags">{record.tags.map((tag) => <span key={tag}>+{tag}</span>)}</div>}{record.comment && <blockquote>{record.comment}</blockquote>}</div>
        {record.warnings.length > 0 && <div className="detail-warning"><AlertTriangle size={16} /><span>{record.warnings.map((warning) => warning.message).join('；')}</span></div>}
      </div>
    </div>
    {playError && <p className="play-error"><AlertTriangle size={15} />{playError}</p>}
    <div className="detail-actions"><button className="danger-text-button" onClick={onDelete}><Trash2 size={16} />删除</button><div>{record.mediaRef && <button className="secondary-button play-button" onClick={() => void play()} disabled={playing} title={record.mediaRef.type === 'tv' ? '在 Emby 中打开剧集' : '从 Emby 播放'}>{playing ? <LoaderCircle className="spin" size={16} /> : <Play size={16} fill="currentColor" />}{playing ? '正在查找' : '播放'}</button>}<button className="secondary-button" onClick={onMatch}><Link2 size={16} />匹配 TMDB</button>{record.mediaRef && <button className="secondary-button" onClick={() => void refresh()} disabled={refreshing}>{refreshing ? <LoaderCircle className="spin" size={16} /> : <RefreshCw size={16} />}刷新详情</button>}<button className="primary-button" onClick={onEdit}><Pencil size={16} />编辑记录</button></div></div>
  </Modal>
}

import { useEffect, useRef, useState } from 'react'
import { AlertTriangle, ChevronDown, Copy, ExternalLink, Film, Link2, LoaderCircle, MonitorPlay, Pencil, Play, RefreshCw, Star, Trash2 } from 'lucide-react'
import { mpvHandlerProtocol, playableHttpUrl, potPlayerProtocol } from '../externalPlayers'
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
  onResolvePlayLink: () => Promise<PlayLink>
  onPlay: (link: PlayLink) => void
}

export function DetailModal({ record, onClose, onEdit, onMatch, onRefresh, onDelete, onSearchActor, onResolvePlayLink, onPlay }: DetailModalProps) {
  const [refreshing, setRefreshing] = useState(false)
  const [playing, setPlaying] = useState(false)
  const [playError, setPlayError] = useState('')
  const [playLink, setPlayLink] = useState<PlayLink | null>(null)
  const [playMenuOpen, setPlayMenuOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const playMenuRef = useRef<HTMLDivElement>(null)
  const meta = statusMeta[record.status]
  const refresh = async () => { setRefreshing(true); await onRefresh(); setRefreshing(false) }
  const loadPlayLink = async () => {
    if (playLink) return playLink
    setPlaying(true)
    setPlayError('')
    try {
      const next = await onResolvePlayLink()
      playableHttpUrl(next.redirectedUrl || next.playUrl)
      setPlayLink(next)
      return next
    } catch (cause) {
      setPlayError(cause instanceof Error ? cause.message : '无法获取 Emby 播放地址')
      return null
    } finally {
      setPlaying(false)
    }
  }
  const play = async () => {
    const link = await loadPlayLink()
    if (link) onPlay(link)
  }
  const togglePlayMenu = () => {
    const open = !playMenuOpen
    setPlayMenuOpen(open)
    setCopied(false)
    if (open && !playLink && !playing) void loadPlayLink()
  }
  const launchExternal = (protocol: string) => {
    setPlayMenuOpen(false)
    window.location.href = protocol
  }
  const copyPlayUrl = async () => {
    if (!playLink) return
    try {
      await copyText(playableHttpUrl(playLink.redirectedUrl || playLink.playUrl))
      setCopied(true)
    } catch {
      setPlayError('复制播放地址失败，请检查浏览器的剪贴板权限')
    }
  }
  useEffect(() => {
    if (!playMenuOpen) return
    const close = (event: PointerEvent) => {
      if (!playMenuRef.current?.contains(event.target as Node)) setPlayMenuOpen(false)
    }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [playMenuOpen])
  const playUrl = playLink ? playLink.redirectedUrl || playLink.playUrl : ''
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
    <div className="detail-actions"><button className="danger-text-button" onClick={onDelete}><Trash2 size={16} />删除</button><div>{record.mediaRef?.type === 'tm' && <div className="play-split" ref={playMenuRef}><button className="secondary-button play-button" onClick={() => void play()} disabled={playing} title="从 Emby 播放">{playing ? <LoaderCircle className="spin" size={16} /> : <Play size={16} fill="currentColor" />}{playing ? '正在查找' : '播放'}</button><button className="secondary-button play-menu-toggle" onClick={togglePlayMenu} aria-label="选择外部播放器" aria-haspopup="menu" aria-expanded={playMenuOpen}><ChevronDown size={15} /></button>{playMenuOpen && <div className="play-menu" role="menu">{playing ? <div className="play-menu-loading"><LoaderCircle className="spin" size={15} />正在获取播放地址</div> : playLink ? <><button role="menuitem" onClick={() => launchExternal(potPlayerProtocol(playUrl))}><MonitorPlay size={16} /><span>PotPlayer 播放</span></button><button role="menuitem" onClick={() => launchExternal(mpvHandlerProtocol(playUrl))}><ExternalLink size={16} /><span>mpv-handler 播放</span></button><button role="menuitem" onClick={() => void copyPlayUrl()}><Copy size={16} /><span>{copied ? '已复制播放地址' : '复制播放地址'}</span></button></> : <div className="play-menu-loading play-menu-failed">播放地址不可用</div>}</div>}</div>}<button className="secondary-button" onClick={onMatch}><Link2 size={16} />匹配 TMDB</button>{record.mediaRef && <button className="secondary-button" onClick={() => void refresh()} disabled={refreshing}>{refreshing ? <LoaderCircle className="spin" size={16} /> : <RefreshCw size={16} />}刷新详情</button>}<button className="primary-button" onClick={onEdit}><Pencil size={16} />编辑记录</button></div></div>
  </Modal>
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  const copied = document.execCommand('copy')
  textarea.remove()
  if (!copied) throw new Error('copy failed')
}

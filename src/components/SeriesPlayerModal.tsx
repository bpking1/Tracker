import { useEffect, useRef, useState } from 'react'
import { AlertTriangle, ChevronDown, Copy, ExternalLink, LoaderCircle, MonitorPlay, Play, RefreshCw } from 'lucide-react'
import { api } from '../api'
import { mpvHandlerProtocol, playableHttpUrl, potPlayerProtocol } from '../externalPlayers'
import { episodeCode, initialSeriesSelection } from '../seriesPlayback'
import type { PlexEpisode, PlexSeriesCatalog, PlayLink, RecordItem } from '../types'
import { Modal } from './Modal'

interface SeriesPlayerModalProps {
  record: RecordItem
  onClose: () => void
  onPlay: (link: PlayLink, episode: PlexEpisode) => void
}

export function SeriesPlayerModal({ record, onClose, onPlay }: SeriesPlayerModalProps) {
  const [catalog, setCatalog] = useState<PlexSeriesCatalog | null>(null)
  const [seasonNumber, setSeasonNumber] = useState<number | null>(null)
  const [episodeKey, setEpisodeKey] = useState('')
  const [loading, setLoading] = useState(true)
  const [catalogRefresh, setCatalogRefresh] = useState(0)
  const [refreshingCatalog, setRefreshingCatalog] = useState(false)
  const [resolving, setResolving] = useState(false)
  const [error, setError] = useState('')
  const [playLink, setPlayLink] = useState<{ episodeKey: string; link: PlayLink } | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let active = true
    const tmdbId = record.mediaRef?.type === 'tv' ? record.mediaRef.id : 0
    if (!tmdbId) {
      setError('这条记录没有有效的剧集 TMDB ID')
      setLoading(false)
      return
    }
    const refreshing = catalogRefresh > 0
    if (refreshing) setRefreshingCatalog(true)
    api.plexSeries(tmdbId, refreshing).then((next) => {
      if (!active) return
      if (refreshing) {
        setPlayLink(null)
        setMenuOpen(false)
        setCopied(false)
      }
      setCatalog(next)
      const selection = initialSeriesSelection(next, record.progress)
      setSeasonNumber(selection.seasonNumber)
      setEpisodeKey(selection.episodeKey)
    }).catch((cause: unknown) => {
      if (active) setError(messageOf(cause))
    }).finally(() => {
      if (active) {
        setLoading(false)
        setRefreshingCatalog(false)
      }
    })
    return () => { active = false }
  }, [record.mediaRef, record.progress, catalogRefresh])

  useEffect(() => {
    if (!menuOpen) return
    const close = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [menuOpen])

  const season = catalog?.seasons.find((item) => item.number === seasonNumber)
  const episode = season?.episodes.find((item) => item.ratingKey === episodeKey)

  const selectSeason = (nextSeason: number) => {
    const next = catalog?.seasons.find((item) => item.number === nextSeason)
    setSeasonNumber(nextSeason)
    setEpisodeKey(next?.episodes[0]?.ratingKey || '')
    setPlayLink(null)
    setMenuOpen(false)
    setCopied(false)
    setError('')
  }

  const selectEpisode = (nextEpisode: PlexEpisode) => {
    setEpisodeKey(nextEpisode.ratingKey)
    setPlayLink(null)
    setMenuOpen(false)
    setCopied(false)
    setError('')
  }

  const resolvePlayLink = async () => {
    if (!catalog || !episode) return null
    if (playLink?.episodeKey === episode.ratingKey) return playLink.link
    setResolving(true)
    setError('')
    try {
      const link = await api.plexEpisodePlayLink(catalog.serverId, catalog.seriesKey, episode.ratingKey)
      playableHttpUrl(link.redirectedUrl || link.playUrl)
      setPlayLink({ episodeKey: episode.ratingKey, link })
      return link
    } catch (cause) {
      setError(messageOf(cause))
      return null
    } finally {
      setResolving(false)
    }
  }

  const play = async () => {
    const link = await resolvePlayLink()
    if (link && episode) onPlay(link, episode)
  }

  const toggleMenu = () => {
    const open = !menuOpen
    setMenuOpen(open)
    setCopied(false)
    if (open && episode && playLink?.episodeKey !== episode.ratingKey && !resolving) void resolvePlayLink()
  }

  const launchExternal = (protocol: string) => {
    setMenuOpen(false)
    window.location.href = protocol
  }

  const copyPlayUrl = async () => {
    if (!playLink || playLink.episodeKey !== episode?.ratingKey) return
    try {
      await copyText(playableHttpUrl(playLink.link.redirectedUrl || playLink.link.playUrl))
      setCopied(true)
    } catch {
      setError('复制播放地址失败，请检查浏览器的剪贴板权限')
    }
  }

  const resolvedUrl = playLink && playLink.episodeKey === episode?.ratingKey
    ? playLink.link.redirectedUrl || playLink.link.playUrl
    : ''

  return <Modal title="选择剧集" onClose={onClose} wide>
    {loading ? <div className="series-loading"><LoaderCircle className="spin" size={20} />正在查找 Plex 剧集</div> : error && !catalog ? <div className="series-empty"><AlertTriangle size={24} /><p>{error}</p></div> : catalog && <div className="series-browser">
      <div className="series-summary"><div className="series-summary-copy"><strong>{catalog.seriesTitle || record.title}</strong><span>来自 {catalog.serverName}</span></div><div className="series-summary-actions"><span>{catalog.seasons.reduce((total, item) => total + item.episodes.length, 0)} 集</span><button className="icon-button" type="button" onClick={() => { setError(''); setCatalogRefresh((value) => value + 1) }} disabled={refreshingCatalog} aria-label="刷新 Plex 剧集目录" title="刷新 Plex 剧集目录">{refreshingCatalog ? <LoaderCircle className="spin" size={16} /> : <RefreshCw size={16} />}</button></div></div>
      <div className="season-tabs" role="tablist" aria-label="季">
        {catalog.seasons.map((item) => <button type="button" role="tab" aria-selected={item.number === seasonNumber} className={item.number === seasonNumber ? 'active' : ''} key={item.number} onClick={() => selectSeason(item.number)}>{item.number === 0 ? '特别篇' : `第 ${item.number} 季`}<span>{item.episodes.length}</span></button>)}
      </div>
      <div className="episode-list" role="listbox" aria-label={season?.title || '剧集列表'}>
        {season?.episodes.map((item) => <button type="button" role="option" aria-selected={item.ratingKey === episodeKey} className={item.ratingKey === episodeKey ? 'selected' : ''} key={item.ratingKey} onClick={() => selectEpisode(item)}>
          <span className="episode-number">E{padNumber(item.episodeNumber)}</span>
          <span className="episode-copy"><strong>{item.title || `第 ${item.episodeNumber} 集`}</strong><small>{[item.airDate, formatDuration(item.duration)].filter(Boolean).join(' · ')}</small></span>
          <Play size={16} fill={item.ratingKey === episodeKey ? 'currentColor' : 'none'} />
        </button>)}
      </div>
      {error && <p className="series-error"><AlertTriangle size={15} />{error}</p>}
      <div className="series-actions">
        <span>{episode ? `${episodeCode(episode)} ${episode.title || ''}` : '请选择一集'}</span>
        <div className="play-split" ref={menuRef}>
          <button className="primary-button play-button" onClick={() => void play()} disabled={!episode || resolving}>{resolving ? <LoaderCircle className="spin" size={16} /> : <Play size={16} fill="currentColor" />}{resolving ? '正在获取' : '播放'}</button>
          <button className="primary-button play-menu-toggle" onClick={toggleMenu} disabled={!episode} aria-label="选择外部播放器" aria-haspopup="menu" aria-expanded={menuOpen}><ChevronDown size={15} /></button>
          {menuOpen && <div className="play-menu" role="menu">{resolving ? <div className="play-menu-loading"><LoaderCircle className="spin" size={15} />正在获取播放地址</div> : resolvedUrl ? <><button role="menuitem" onClick={() => launchExternal(potPlayerProtocol(resolvedUrl))}><MonitorPlay size={16} /><span>PotPlayer 播放</span></button><button role="menuitem" onClick={() => launchExternal(mpvHandlerProtocol(resolvedUrl))}><ExternalLink size={16} /><span>mpv-handler 播放</span></button><button role="menuitem" onClick={() => void copyPlayUrl()}><Copy size={16} /><span>{copied ? '已复制播放地址' : '复制播放地址'}</span></button></> : <div className="play-menu-loading play-menu-failed">播放地址不可用</div>}</div>}
        </div>
      </div>
    </div>}
  </Modal>
}

function padNumber(value: number) {
  return String(value).padStart(2, '0')
}

function formatDuration(milliseconds: number) {
  if (milliseconds <= 0) return ''
  const minutes = Math.round(milliseconds / 60000)
  return `${minutes} 分钟`
}

function messageOf(cause: unknown) {
  return cause instanceof Error ? cause.message : '无法读取 Plex 剧集目录'
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

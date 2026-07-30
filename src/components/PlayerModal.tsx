import { useEffect, useRef, useState } from 'react'
import type Artplayer from 'artplayer'
import { AlertTriangle, Captions, CaptionsOff, LoaderCircle, X } from 'lucide-react'

interface PlayerModalProps {
  title: string
  url: string
  mediaId: string
  posterUrl?: string
  serverName: string
  onClose: () => void
}

export function PlayerModal({ title, url, mediaId, posterUrl, serverName, onClose }: PlayerModalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const playerRef = useRef<Artplayer | null>(null)
  const subtitleInputRef = useRef<HTMLInputElement>(null)
  const [error, setError] = useState('')
  const [playerReady, setPlayerReady] = useState(false)
  const [subtitleName, setSubtitleName] = useState('')
  const [subtitleLoading, setSubtitleLoading] = useState(false)

  useEffect(() => {
    if (!containerRef.current) return

    let player: Artplayer | null = null
    let active = true
    setPlayerReady(false)
    setSubtitleName('')
    setSubtitleLoading(false)
    setError('')
    void import('artplayer').then((artplayerModule) => {
      if (!active || !containerRef.current) return
      const ArtplayerConstructor = artplayerModule.default
      player = new ArtplayerConstructor({
        id: mediaId,
        container: containerRef.current,
        url,
        poster: posterUrl,
        autoplay: true,
        volume: 0.5,
        pip: true,
        miniProgressBar: true,
        autoPlayback: true,
        playbackRate: true,
        setting: true,
        subtitleOffset: true,
        hotkey: true,
        fullscreen: true,
        fullscreenWeb: true,
        mutex: true,
        theme: '#4f8a68',
      })
      playerRef.current = player
      setPlayerReady(true)
      player.on('error', (cause) => {
        const detail = cause instanceof Error && cause.message ? `：${cause.message}` : ''
        setError(`视频读取失败${detail}。请确认视频地址可以访问，并且浏览器原生支持该文件的容器和音视频编码。`)
      })
    }).catch((cause: unknown) => {
      if (!active) return
      const detail = cause instanceof Error && cause.message ? `：${cause.message}` : ''
      setError(`播放器初始化失败${detail}`)
    })

    return () => {
      active = false
      playerRef.current = null
      revokeSubtitleURL(player)
      player?.destroy(true)
    }
  }, [mediaId, posterUrl, url])

  const loadSubtitle = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const input = event.currentTarget
    const file = input.files?.[0]
    input.value = ''
    if (!file) return

    const type = subtitleType(file.name)
    if (!type) {
      setError('字幕格式不支持，请选择 SRT、VTT 或 ASS 文件。')
      return
    }
    const player = playerRef.current
    if (!player) {
      setError('播放器尚未初始化，请稍后重试。')
      return
    }

    const objectURL = URL.createObjectURL(file)
    setSubtitleLoading(true)
    try {
      await player.subtitle.switch(objectURL, { name: file.name, type, encoding: 'utf-8' })
      player.subtitle.show = true
      setSubtitleName(file.name)
      setError('')
    } catch (cause) {
      const detail = cause instanceof Error && cause.message ? `：${cause.message}` : ''
      setError(`字幕加载失败${detail}`)
    } finally {
      URL.revokeObjectURL(objectURL)
      setSubtitleLoading(false)
    }
  }

  const closeSubtitle = () => {
    const player = playerRef.current
    if (!player) return
    player.subtitle.show = false
    revokeSubtitleURL(player)
    setSubtitleName('')
    player.notice.show = '字幕已关闭'
  }

  return <div className="player-backdrop" role="presentation">
    <section className="player-dialog" role="dialog" aria-modal="true" aria-label={`播放 ${title}`}>
      <header>
        <div className="player-heading"><h2>{title}</h2><span>来自 {serverName}</span></div>
        <div className="player-tools">
          <input ref={subtitleInputRef} className="player-subtitle-input" type="file" accept=".srt,.vtt,.ass,text/vtt,application/x-subrip" onChange={(event) => void loadSubtitle(event)} />
          {subtitleName && <span className="player-subtitle-name" title={subtitleName}>{subtitleName}</span>}
          {subtitleName && <button className="player-tool-button" onClick={closeSubtitle} disabled={subtitleLoading} aria-label="关闭字幕" title="关闭字幕"><CaptionsOff size={19} /></button>}
          <button className="player-tool-button" onClick={() => subtitleInputRef.current?.click()} disabled={!playerReady || subtitleLoading} aria-label="选择本地字幕" title="选择本地字幕">{subtitleLoading ? <LoaderCircle className="spin" size={19} /> : <Captions size={20} />}</button>
          <button className="player-close" onClick={onClose} aria-label="关闭播放器" title="关闭播放器"><X size={20} /></button>
        </div>
      </header>
      <div className="player-stage" ref={containerRef} />
      {error && <p className="player-error"><AlertTriangle size={16} /><span>{error}</span></p>}
    </section>
  </div>
}

function subtitleType(fileName: string): 'srt' | 'vtt' | 'ass' | null {
  const extension = fileName.split('.').pop()?.toLowerCase()
  return extension === 'srt' || extension === 'vtt' || extension === 'ass' ? extension : null
}

function revokeSubtitleURL(player: Artplayer | null) {
  const subtitleURL = player?.subtitle.url
  if (subtitleURL?.startsWith('blob:')) URL.revokeObjectURL(subtitleURL)
}

import { useEffect, useRef, useState } from 'react'
import type Artplayer from 'artplayer'
import { AlertTriangle, X } from 'lucide-react'

interface PlayerModalProps {
  title: string
  url: string
  posterUrl?: string
  serverName: string
  onClose: () => void
}

export function PlayerModal({ title, url, posterUrl, serverName, onClose }: PlayerModalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!containerRef.current) return

    let player: Artplayer | null = null
    let active = true
    void Promise.all([import('artplayer'), import('artplayer-proxy-mediabunny')]).then(([artplayerModule, proxyModule]) => {
      if (!active || !containerRef.current) return
      const ArtplayerConstructor = artplayerModule.default
      const proxyMediabunny = proxyModule.default
      player = new ArtplayerConstructor({
        container: containerRef.current,
        url,
        poster: posterUrl,
        autoplay: true,
        volume: 0.7,
        playbackRate: true,
        setting: true,
        hotkey: true,
        fullscreen: true,
        fullscreenWeb: true,
        mutex: true,
        theme: '#4f8a68',
        proxy: proxyMediabunny({
          source: url,
          poster: posterUrl,
          autoplay: true,
          preflightRange: false,
        }),
      })
      player.on('error', (cause) => {
        const detail = cause instanceof Error && cause.message ? `：${cause.message}` : ''
        setError(`视频读取失败${detail}。请确认视频地址允许跨域访问，并且浏览器支持该视频的音视频编码。`)
      })
    }).catch((cause: unknown) => {
      if (!active) return
      const detail = cause instanceof Error && cause.message ? `：${cause.message}` : ''
      setError(`播放器初始化失败${detail}`)
    })

    return () => {
      active = false
      player?.destroy(true)
    }
  }, [posterUrl, url])

  return <div className="player-backdrop" role="presentation">
    <section className="player-dialog" role="dialog" aria-modal="true" aria-label={`播放 ${title}`}>
      <header>
        <div><h2>{title}</h2><span>来自 {serverName}</span></div>
        <button className="player-close" onClick={onClose} aria-label="关闭播放器"><X size={20} /></button>
      </header>
      <div className="player-stage" ref={containerRef} />
      {error && <p className="player-error"><AlertTriangle size={16} /><span>{error}</span></p>}
    </section>
  </div>
}

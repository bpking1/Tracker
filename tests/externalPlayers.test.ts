import { describe, expect, test } from 'bun:test'
import { mpvHandlerProtocol, playableHttpUrl, potPlayerProtocol } from '../src/externalPlayers'

describe('external player protocols', () => {
  test('builds a PotPlayer URL without changing the media URL', () => {
    const mediaUrl = 'https://media.example/video.mkv?token=a%2Fb&part=1'
    expect(potPlayerProtocol(mediaUrl)).toBe(`potplayer://${mediaUrl}`)
  })

  test('builds the URL-safe base64 protocol expected by mpv-handler', () => {
    expect(mpvHandlerProtocol('https://www.youtube.com/watch?v=Ggkn2f5e-IU'))
      .toBe('mpv-handler://play/aHR0cHM6Ly93d3cueW91dHViZS5jb20vd2F0Y2g_dj1HZ2tuMmY1ZS1JVQ')
  })

  test('encodes Unicode URLs as UTF-8 and rejects unsafe protocols', () => {
    const protocol = mpvHandlerProtocol('https://media.example/电影.mkv')
    const encoded = protocol.slice('mpv-handler://play/'.length).replaceAll('_', '/').replaceAll('-', '+')
    const padded = encoded.padEnd(Math.ceil(encoded.length / 4) * 4, '=')
    const decoded = new TextDecoder().decode(Uint8Array.from(atob(padded), (character) => character.charCodeAt(0)))
    expect(decoded).toBe('https://media.example/电影.mkv')
    expect(() => playableHttpUrl('file:///movie.mkv')).toThrow()
  })
})

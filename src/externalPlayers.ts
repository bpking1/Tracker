export function playableHttpUrl(raw: string): string {
  const value = raw.trim()
  const parsed = new URL(value)
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('外部播放器只支持 HTTP 或 HTTPS 播放地址')
  }
  return value
}

export function potPlayerProtocol(raw: string): string {
  return `potplayer://${playableHttpUrl(raw)}`
}

export function mpvHandlerProtocol(raw: string): string {
  return `mpv-handler://play/${base64Url(playableHttpUrl(raw))}`
}

function base64Url(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replaceAll('/', '_').replaceAll('+', '-').replaceAll('=', '')
}

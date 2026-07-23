import type { AppConfig, AutoMatchResult, MediaRef, PlayLink, RecordInput, RefreshMetadataResult, Snapshot, TmdbResult } from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = new Error(payload.error || `请求失败 (${response.status})`)
    Object.assign(error, { status: response.status, payload })
    throw error
  }
  return payload as T
}

export const api = {
  records: () => request<Snapshot>('/api/records'),
  create: (revision: string, record: RecordInput) =>
    request<Snapshot>('/api/records', { method: 'POST', body: JSON.stringify({ revision, ...record }) }),
  update: (revision: string, key: string, record: RecordInput) =>
    request<Snapshot>(`/api/records/${key}`, { method: 'PUT', body: JSON.stringify({ revision, ...record }) }),
  remove: (revision: string, key: string) =>
    request<Snapshot>(`/api/records/${key}`, { method: 'DELETE', body: JSON.stringify({ revision }) }),
  searchTmdb: (query: string, type = 'all') =>
    request<TmdbResult[]>(`/api/tmdb/search?q=${encodeURIComponent(query)}&type=${type}`),
  matchTmdb: (revision: string, key: string, type: 'tm' | 'tv', id: number) =>
    request<Snapshot>(`/api/records/${key}/tmdb-match`, {
      method: 'POST',
      body: JSON.stringify({ revision, type, id }),
    }),
  refreshTmdb: (revision: string, key: string) =>
    request<Snapshot>(`/api/records/${key}/tmdb-refresh`, {
      method: 'POST',
      body: JSON.stringify({ revision }),
    }),
  autoMatchTmdb: (revision: string) =>
    request<AutoMatchResult>('/api/tmdb/auto-match', {
      method: 'POST',
      body: JSON.stringify({ revision }),
    }),
  refreshMissingMetadata: (revision: string) =>
    request<RefreshMetadataResult>('/api/tmdb/refresh-missing', {
      method: 'POST',
      body: JSON.stringify({ revision }),
    }),
  playLink: (mediaRef: MediaRef) =>
    request<PlayLink>(`/api/play-link?type=${mediaRef.type}&q=${mediaRef.id}`),
  config: () => request<AppConfig>('/api/config'),
}

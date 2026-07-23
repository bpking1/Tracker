export type Status = 'planned' | 'watching' | 'watched' | 'dropped'

export interface MediaRef {
  type: 'tm' | 'tv'
  id: number
}

export interface ParseWarning {
  code: string
  message: string
}

export interface MediaMetadata {
  mediaRef: MediaRef
  title: string
  originalTitle: string
  releaseDate: string
  overview: string
  posterUrl: string
  genres: string[]
  cast: string[]
  voteAverage: number
  fetchedAt: string
}

export interface RecordItem {
  key: string
  status: Status
  title: string
  mediaRef: MediaRef | null
  completedAt: string | null
  createdAt: string | null
  rating: number | null
  progress: string | null
  tags: string[]
  comment: string | null
  rawLine: string
  lineNumber: number
  warnings: ParseWarning[]
  metadata?: MediaMetadata | null
  metadataState: 'unmatched' | 'missing' | 'invalid' | 'ready'
}

export type RecordInput = Omit<RecordItem, 'key' | 'rawLine' | 'lineNumber' | 'warnings' | 'metadata' | 'metadataState'>

export interface Snapshot {
  revision: string
  records: RecordItem[]
  fileWarnings: ParseWarning[]
}

export interface TmdbResult {
  id: number
  type: 'movie' | 'tv'
  title: string
  date: string
  overview: string
  posterPath: string
  voteAverage: number
}

export interface AppConfig {
  dataFile: string
}

export interface AutoMatchFailure {
  title: string
  error: string
}

export interface AutoMatchResult {
  snapshot: Snapshot
  total: number
  matched: number
  noResults: string[]
  failed: AutoMatchFailure[]
}

export interface RefreshMetadataResult {
  snapshot: Snapshot
  total: number
  refreshed: number
  failed: AutoMatchFailure[]
}

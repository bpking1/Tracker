import type { MediaRef, RecordItem, Status } from './types'

export type SortKey = 'file' | 'completed' | 'created' | 'rating'
export type MetadataFilter = 'all' | 'unmatched' | 'issue'
export type MediaTypeFilter = 'all' | 'tm' | 'tv'
export type TimelineStatusFilter = 'all' | 'watched' | 'dropped'

export interface LibraryFilters {
  query: string
  status: Status | 'all'
  metadata: MetadataFilter
  mediaType: MediaTypeFilter
  genre: string
  sort: SortKey
}

export interface TimelineFilters {
  query: string
  year: string
  status: TimelineStatusFilter
  genre: string
}

export interface TimelineYearGroup {
  year: string
  months: Array<{ month: string; items: RecordItem[] }>
}

export function recordMatchesQuery(record: RecordItem, query: string) {
  const normalized = query.trim().toLocaleLowerCase()
  if (!normalized) return true

  return [
    record.title,
    record.metadata?.title || '',
    record.metadata?.originalTitle || '',
    record.comment || '',
    ...record.tags,
    ...(record.metadata?.genres || []),
    ...(record.metadata?.cast || []),
  ].some((value) => value.toLocaleLowerCase().includes(normalized))
}

export function filterLibraryRecords(records: RecordItem[], filters: LibraryFilters) {
  const filtered = records.filter((record) => {
    if (filters.status !== 'all' && record.status !== filters.status) return false
    if (filters.metadata === 'unmatched' && record.mediaRef) return false
    if (filters.metadata === 'issue' && record.metadataState !== 'missing' && record.metadataState !== 'invalid') return false
    if (filters.mediaType !== 'all' && record.mediaRef?.type !== filters.mediaType) return false
    if (filters.genre !== 'all' && !record.metadata?.genres.includes(filters.genre)) return false
    return recordMatchesQuery(record, filters.query)
  })

  return [...filtered].sort((left, right) => {
    if (filters.sort === 'rating') return (right.rating || 0) - (left.rating || 0)
    if (filters.sort === 'created') return (right.createdAt || '').localeCompare(left.createdAt || '')
    if (filters.sort === 'completed') return (right.completedAt || '').localeCompare(left.completedAt || '')
    return left.lineNumber - right.lineNumber
  })
}

export function filterTimelineRecords(records: RecordItem[], filters: TimelineFilters) {
  return records
    .filter((record) => {
      if ((record.status !== 'watched' && record.status !== 'dropped') || !record.completedAt) return false
      if (filters.year !== 'all' && !record.completedAt.startsWith(filters.year)) return false
      if (filters.status !== 'all' && record.status !== filters.status) return false
      if (filters.genre !== 'all' && !record.metadata?.genres.includes(filters.genre)) return false
      return recordMatchesQuery(record, filters.query)
    })
    .sort((left, right) => right.completedAt!.localeCompare(left.completedAt!) || right.lineNumber - left.lineNumber)
}

export function groupTimelineRecords(records: RecordItem[]) {
  const yearGroups = new Map<string, Map<string, RecordItem[]>>()
  records.forEach((record) => {
    const [year, month] = record.completedAt!.split('.')
    if (!yearGroups.has(year)) yearGroups.set(year, new Map())
    const months = yearGroups.get(year)!
    if (!months.has(month)) months.set(month, [])
    months.get(month)!.push(record)
  })

  return Array.from(yearGroups, ([year, months]): TimelineYearGroup => ({
    year,
    months: Array.from(months, ([month, items]) => ({ month, items })),
  }))
}

export function sameMediaRef(left: MediaRef | null, right: MediaRef) {
  return left?.type === right.type && left.id === right.id
}

function mediaRefCounts(records: RecordItem[]) {
  const counts = new Map<string, { mediaRef: MediaRef; count: number }>()
  records.forEach((record) => {
    if (!record.mediaRef) return
    const key = `${record.mediaRef.type}:${record.mediaRef.id}`
    const current = counts.get(key)
    counts.set(key, { mediaRef: record.mediaRef, count: (current?.count || 0) + 1 })
  })
  return counts
}

export function newlyDuplicatedMediaRefs(before: RecordItem[], after: RecordItem[]) {
  const previous = mediaRefCounts(before)
  return Array.from(mediaRefCounts(after).entries())
    .filter(([key, value]) => value.count > 1 && (previous.get(key)?.count || 0) < 2)
    .map(([, value]) => value.mediaRef)
}

export function duplicateMessage(mediaRefs: MediaRef[]) {
  const refs = mediaRefs.slice(0, 3).map((mediaRef) => `${mediaRef.type}:${mediaRef.id}`).join('、')
  const remaining = mediaRefs.length > 3 ? ` 等 ${mediaRefs.length} 组` : ''
  return `片单中已存在相同 TMDB 记录（${refs}${remaining}），本次匹配仍已保存。`
}

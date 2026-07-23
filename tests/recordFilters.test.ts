import { describe, expect, test } from 'bun:test'
import {
  filterLibraryRecords,
  filterTimelineRecords,
  groupTimelineRecords,
  newlyDuplicatedMediaRefs,
  recordMatchesQuery,
} from '../src/recordFilters'
import type { MediaMetadata, RecordItem } from '../src/types'

function metadata(overrides: Partial<MediaMetadata> = {}): MediaMetadata {
  return {
    mediaRef: { type: 'tm', id: 1 },
    title: '标准标题',
    originalTitle: 'Original Title',
    releaseDate: '2024.01.01',
    overview: '',
    posterUrl: '',
    genres: ['剧情'],
    cast: ['演员甲'],
    voteAverage: 8,
    fetchedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function record(overrides: Partial<RecordItem> = {}): RecordItem {
  return {
    key: 'record-1',
    status: 'planned',
    title: '用户标题',
    mediaRef: { type: 'tm', id: 1 },
    completedAt: null,
    createdAt: '2026.01.01',
    rating: null,
    progress: null,
    tags: ['收藏'],
    comment: '个人短评',
    rawLine: '',
    lineNumber: 1,
    warnings: [],
    metadata: metadata(),
    metadataState: 'ready',
    ...overrides,
  }
}

describe('recordMatchesQuery', () => {
  test('matches every searchable text field', () => {
    const item = record()
    for (const query of ['用户标题', '标准标题', 'original title', '演员甲', '剧情', '收藏', '个人短评']) {
      expect(recordMatchesQuery(item, query)).toBe(true)
    }
    expect(recordMatchesQuery(item, '不存在')).toBe(false)
  })
})

describe('filterLibraryRecords', () => {
  const records = [
    record({ key: 'movie', status: 'watched', completedAt: '2025.01.01', rating: 5, lineNumber: 2 }),
    record({
      key: 'tv',
      title: '剧集',
      status: 'planned',
      mediaRef: { type: 'tv', id: 2 },
      rating: 3,
      lineNumber: 1,
      metadata: metadata({ mediaRef: { type: 'tv', id: 2 }, genres: ['科幻'] }),
    }),
    record({ key: 'unmatched', title: '未匹配', mediaRef: null, metadata: null, metadataState: 'unmatched', lineNumber: 3 }),
  ]

  test('combines status, media type and genre filters', () => {
    const result = filterLibraryRecords(records, {
      query: '',
      status: 'planned',
      metadata: 'all',
      mediaType: 'tv',
      genre: '科幻',
      sort: 'file',
    })
    expect(result.map((item) => item.key)).toEqual(['tv'])
  })

  test('filters unmatched records and sorts without mutating input', () => {
    const result = filterLibraryRecords(records, {
      query: '',
      status: 'all',
      metadata: 'unmatched',
      mediaType: 'all',
      genre: 'all',
      sort: 'rating',
    })
    expect(result.map((item) => item.key)).toEqual(['unmatched'])
    expect(records.map((item) => item.key)).toEqual(['movie', 'tv', 'unmatched'])
  })
})

describe('timeline helpers', () => {
  const records = [
    record({ key: 'older', status: 'watched', completedAt: '2025.01.03', lineNumber: 1 }),
    record({ key: 'newer', status: 'dropped', completedAt: '2025.02.04', lineNumber: 2 }),
    record({ key: 'planned', status: 'planned', completedAt: null, lineNumber: 3 }),
  ]

  test('filters dated watched records and keeps descending date order', () => {
    const result = filterTimelineRecords(records, { query: '', year: '2025', status: 'all', genre: 'all' })
    expect(result.map((item) => item.key)).toEqual(['newer', 'older'])
  })

  test('groups filtered records by year and month', () => {
    const filtered = filterTimelineRecords(records, { query: '', year: 'all', status: 'all', genre: 'all' })
    expect(groupTimelineRecords(filtered).map((group) => ({
      year: group.year,
      months: group.months.map((month) => month.month),
    }))).toEqual([{ year: '2025', months: ['02', '01'] }])
  })
})

describe('newlyDuplicatedMediaRefs', () => {
  test('reports only duplicate IDs introduced by the operation', () => {
    const existing = [
      record({ key: 'first', mediaRef: { type: 'tm', id: 1 } }),
      record({ key: 'unmatched', mediaRef: null }),
    ]
    const next = [
      existing[0],
      record({ key: 'matched', mediaRef: { type: 'tm', id: 1 } }),
    ]
    expect(newlyDuplicatedMediaRefs(existing, next)).toEqual([{ type: 'tm', id: 1 }])
    expect(newlyDuplicatedMediaRefs(next, next)).toEqual([])
  })
})

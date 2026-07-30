import { describe, expect, test } from 'bun:test'
import { episodeCode, initialSeriesSelection } from '../src/seriesPlayback'
import type { PlexSeriesCatalog } from '../src/types'

const catalog: PlexSeriesCatalog = {
  seriesTitle: '测试剧集',
  serverId: '0',
  serverName: 'Plex',
  seriesKey: '10',
  seasons: [
    { number: 0, title: '特别篇', episodes: [{ ratingKey: '1', seasonNumber: 0, episodeNumber: 1, title: '花絮', duration: 0, airDate: '' }] },
    { number: 2, title: '第 2 季', episodes: [{ ratingKey: '21', seasonNumber: 2, episodeNumber: 1, title: '第一集', duration: 0, airDate: '' }, { ratingKey: '22', seasonNumber: 2, episodeNumber: 2, title: '第二集', duration: 0, airDate: '' }] },
  ],
}

describe('series playback selection', () => {
  test('selects the recorded episode when progress is valid', () => {
    expect(initialSeriesSelection(catalog, 'S02E02')).toEqual({ seasonNumber: 2, episodeKey: '22' })
  })

  test('falls back to the first available episode', () => {
    expect(initialSeriesSelection(catalog, 'S09E01')).toEqual({ seasonNumber: 0, episodeKey: '1' })
  })

  test('builds a stable episode code including specials', () => {
    expect(episodeCode({ seasonNumber: 0, episodeNumber: 3 })).toBe('S00E03')
  })
})

import type { PlexEpisode, PlexSeriesCatalog } from './types'

export function initialSeriesSelection(catalog: PlexSeriesCatalog, progress: string | null) {
  const parsed = parseProgress(progress)
  const progressSeason = parsed ? catalog.seasons.find((season) => season.number === parsed.season) : undefined
  const season = progressSeason || catalog.seasons[0]
  const progressEpisode = progressSeason?.episodes.find((episode) => episode.episodeNumber === parsed?.episode)
  return {
    seasonNumber: season?.number ?? null,
    episodeKey: progressEpisode?.ratingKey || season?.episodes[0]?.ratingKey || '',
  }
}

export function episodeCode(episode: Pick<PlexEpisode, 'seasonNumber' | 'episodeNumber'>) {
  return `S${padNumber(episode.seasonNumber)}E${padNumber(episode.episodeNumber)}`
}

function parseProgress(progress: string | null) {
  const match = progress?.match(/S(\d+)E(\d+)/i)
  if (!match) return null
  return { season: Number(match[1]), episode: Number(match[2]) }
}

function padNumber(value: number) {
  return String(value).padStart(2, '0')
}

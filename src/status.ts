import type { Status } from './types'

export const statusMeta: Record<Status, { label: string; symbol: string; className: string }> = {
  planned: { label: '想看', symbol: '−', className: 'status-planned' },
  watching: { label: '在看', symbol: '›', className: 'status-watching' },
  watched: { label: '看过', symbol: '✓', className: 'status-watched' },
  dropped: { label: '弃看', symbol: '×', className: 'status-dropped' },
}

import type { Step, Severity } from '@/types'

export interface Lane {
    id: string // subject (e.g. "_chain", "node-1")
    label: string
    highlight?: boolean
}

export interface TimelineItem {
    id: string // diagnostic event_id
    laneId: string // subject
    time: Date
    severity: Severity
    state: 'open' | 'recovered'
    label?: string // short label for hover/click
}

export type TimelineEvent =
    | { type: 'item-click'; itemId: string; time: Date }
    | { type: 'cursor-change'; time: Date }
    | { type: 'pan'; from: Date; to: Date }

export interface TimelineRenderer {
    setLanes(lanes: Lane[]): void
    setItems(items: TimelineItem[]): void
    setStep(step: Step): void
    setCursor(at: Date | null): void
    on(handler: (e: TimelineEvent) => void): void
    destroy(): void
}

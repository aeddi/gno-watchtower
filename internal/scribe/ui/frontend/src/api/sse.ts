import type { ScribeEvent } from '@/types'

export interface StreamFilter {
    // Best-effort server-side filtering (scribe SSE supports a kinds CSV).
    // Client-side filtering is also applied by consumers as needed.
    kinds?: string[]
    subjects?: string[]
}

export class EventStream {
    private es: EventSource

    constructor(
        filter: StreamFilter,
        private onEvent: (e: ScribeEvent) => void
    ) {
        const params = new URLSearchParams()
        if (filter.kinds?.length) params.set('kinds', filter.kinds.join(','))
        if (filter.subjects?.length)
            params.set('subjects', filter.subjects.join(','))
        const qs = params.toString()
        const url = `/api/events/stream${qs ? `?${qs}` : ''}`
        this.es = new EventSource(url)
        this.es.onmessage = (e) => {
            try {
                const data = JSON.parse(e.data) as ScribeEvent
                this.onEvent(data)
            } catch {
                // Drop malformed payloads silently — server-side bug, not client's.
            }
        }
        this.es.onerror = () => {
            // EventSource auto-reconnects; just log for visibility.
            // eslint-disable-next-line no-console
            console.warn('SSE error; browser will reconnect')
        }
    }

    close() {
        this.es.close()
    }
}

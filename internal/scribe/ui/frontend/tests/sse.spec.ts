import { describe, it, expect, vi, beforeEach } from 'vitest'
import { EventStream } from '@/api/sse'

class StubEventSource {
    static last: StubEventSource
    url: string
    onmessage: ((e: MessageEvent) => void) | null = null
    onerror: ((e: Event) => void) | null = null
    readyState = 1
    constructor(url: string) {
        this.url = url
        StubEventSource.last = this
    }
    close() {
        this.readyState = 2
    }
    // helper for tests
    emit(data: any) {
        this.onmessage?.(
            new MessageEvent('message', { data: JSON.stringify(data) })
        )
    }
}

describe('EventStream', () => {
    beforeEach(() => {
        // @ts-expect-error - replacing the global for the test
        global.EventSource = StubEventSource
    })

    it('opens an EventSource at the right URL', () => {
        new EventStream({ kinds: ['diagnostic.bft_at_risk_v1'] }, () => {})
        expect(StubEventSource.last.url).toContain('/api/events/stream')
        expect(StubEventSource.last.url).toContain(
            'kinds=diagnostic.bft_at_risk_v1'
        )
    })

    it('parses messages and forwards to onEvent', () => {
        const events: any[] = []
        new EventStream({}, (e) => events.push(e))
        StubEventSource.last.emit({ event_id: '01J0', kind: 'x.y' })
        expect(events).toHaveLength(1)
        expect(events[0].event_id).toBe('01J0')
    })

    it('drops malformed JSON without throwing', () => {
        const events: any[] = []
        new EventStream({}, (e) => events.push(e))
        StubEventSource.last.onmessage?.(
            new MessageEvent('message', { data: 'not json' })
        )
        expect(events).toHaveLength(0)
    })

    it('close() closes the EventSource', () => {
        const stream = new EventStream({}, () => {})
        stream.close()
        expect(StubEventSource.last.readyState).toBe(2)
    })
})

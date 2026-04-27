import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useTimelineStore } from '@/stores/timeline'
import { storeToParams, paramsToStore } from '@/utils/url-state'

describe('url-state', () => {
    beforeEach(() => setActivePinia(createPinia()))

    it('storeToParams omits live-mode at', () => {
        const s = useTimelineStore()
        s.setSubjects(['node-1', 'node-3'])
        const p = storeToParams(s)
        expect(p.get('subject')).toBe('node-1,node-3')
        expect(p.get('at')).toBeNull()
    })

    it('storeToParams includes at when paused', () => {
        const s = useTimelineStore()
        s.setAt(new Date('2026-04-27T12:34:56Z'))
        const p = storeToParams(s)
        expect(p.get('at')).toBe('2026-04-27T12:34:56.000Z')
    })

    it('paramsToStore hydrates the store from URL', () => {
        const s = useTimelineStore()
        const p = new URLSearchParams(
            '?at=2026-04-27T10:00:00Z&subject=node-2&layout=force'
        )
        paramsToStore(p, s)
        expect(s.at?.toISOString()).toBe('2026-04-27T10:00:00.000Z')
        expect(s.subjects).toEqual(['node-2'])
        expect(s.graphLayout).toBe('force')
    })

    it('paramsToStore handles empty params', () => {
        const s = useTimelineStore()
        const p = new URLSearchParams('')
        paramsToStore(p, s)
        expect(s.at).toBeNull()
        expect(s.subjects).toEqual([])
    })
})

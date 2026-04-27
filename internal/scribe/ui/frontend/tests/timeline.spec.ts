import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useTimelineStore } from '@/stores/timeline'

describe('useTimelineStore', () => {
    beforeEach(() => setActivePinia(createPinia()))

    it('starts in live mode with no subjects', () => {
        const s = useTimelineStore()
        expect(s.at).toBeNull()
        expect(s.liveMode).toBe(true)
        expect(s.subjects).toEqual([])
        expect(s.step).toBe('30s')
        expect(s.graphLayout).toBe('circle')
    })

    it('setAt(date) disengages live mode', () => {
        const s = useTimelineStore()
        s.setAt(new Date('2026-04-27T12:34:56Z'))
        expect(s.liveMode).toBe(false)
        expect(s.at?.toISOString()).toBe('2026-04-27T12:34:56.000Z')
    })

    it('goLive() resumes live mode', () => {
        const s = useTimelineStore()
        s.setAt(new Date('2026-04-27T12:00:00Z'))
        s.goLive()
        expect(s.liveMode).toBe(true)
        expect(s.at).toBeNull()
    })

    it('toggleSubject adds and removes', () => {
        const s = useTimelineStore()
        s.toggleSubject('node-1')
        expect(s.subjects).toEqual(['node-1'])
        s.toggleSubject('node-2')
        expect(s.subjects).toEqual(['node-1', 'node-2'])
        s.toggleSubject('node-1')
        expect(s.subjects).toEqual(['node-2'])
    })

    it('setSubjects replaces selection', () => {
        const s = useTimelineStore()
        s.toggleSubject('node-1')
        s.setSubjects(['node-3'])
        expect(s.subjects).toEqual(['node-3'])
        s.setSubjects([])
        expect(s.subjects).toEqual([])
    })

    it('filters update independently', () => {
        const s = useTimelineStore()
        s.filters.severity = ['error', 'critical']
        s.filters.state = ['open']
        expect(s.filters.severity).toEqual(['error', 'critical'])
        expect(s.filters.state).toEqual(['open'])
    })
})

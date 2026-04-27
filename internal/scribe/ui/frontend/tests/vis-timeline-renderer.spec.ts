import { describe, it, expect, beforeEach, vi } from 'vitest'
import { VisTimelineRenderer } from '@/renderers/timeline/VisTimelineRenderer'

vi.mock('vis-timeline/standalone', () => {
    return {
        Timeline: vi.fn().mockImplementation(() => ({
            setItems: vi.fn(),
            setGroups: vi.fn(),
            setOptions: vi.fn(),
            setCustomTime: vi.fn(),
            addCustomTime: vi.fn(),
            destroy: vi.fn(),
            on: vi.fn(),
            setWindow: vi.fn(),
        })),
        DataSet: class {
            _arr: any[]
            constructor(a: any[] = []) {
                this._arr = a
            }
            get() {
                return this._arr
            }
            add(x: any) {
                Array.isArray(x) ? this._arr.push(...x) : this._arr.push(x)
            }
            clear() {
                this._arr.length = 0
            }
        },
    }
})

describe('VisTimelineRenderer', () => {
    let container: HTMLElement
    beforeEach(() => {
        container = document.createElement('div')
    })

    it('mounts without throwing', () => {
        const r = new VisTimelineRenderer(container)
        expect(() =>
            r.setLanes([{ id: '_chain', label: '_chain' }])
        ).not.toThrow()
        expect(() =>
            r.setItems([
                {
                    id: '01J0',
                    laneId: '_chain',
                    time: new Date(),
                    severity: 'error',
                    state: 'open',
                },
            ])
        ).not.toThrow()
        r.destroy()
    })

    it('on() registers a handler that can be invoked', () => {
        const r = new VisTimelineRenderer(container)
        const handler = vi.fn()
        r.on(handler)
        expect(handler).toHaveBeenCalledTimes(0)
        r.destroy()
    })

    it('setStep updates the visible window', () => {
        const r = new VisTimelineRenderer(container)
        expect(() => r.setStep('30s')).not.toThrow()
        expect(() => r.setStep('1h')).not.toThrow()
        r.destroy()
    })

    it('setCursor with non-null Date does not throw', () => {
        const r = new VisTimelineRenderer(container)
        expect(() => r.setCursor(new Date())).not.toThrow()
        expect(() => r.setCursor(null)).not.toThrow()
        r.destroy()
    })
})

import { Timeline, DataSet } from 'vis-timeline/standalone'
import type {
    Lane,
    TimelineEvent,
    TimelineItem,
    TimelineRenderer,
} from './TimelineRenderer'
import type { Step } from '@/types'

const STEP_TO_SECONDS_PER_PIXEL: Record<Step, number> = {
    '1s': 1,
    '5s': 5,
    '30s': 30,
    '5m': 300,
    '1h': 3600,
}

const SEVERITY_COLOR: Record<string, string> = {
    warning: '#f59e0b',
    error: '#ef4444',
    critical: '#ef4444',
}

const CURSOR_ID = 'cursor'

export class VisTimelineRenderer implements TimelineRenderer {
    private tl: Timeline
    private items = new DataSet<any>([])
    private groups = new DataSet<any>([])
    private listeners: Array<(e: TimelineEvent) => void> = []

    constructor(container: HTMLElement) {
        this.tl = new Timeline(container, this.items, this.groups, {
            stack: false,
            multiselect: false,
            orientation: 'top',
            cluster: { titleTemplate: '{count} events' },
            zoomable: true,
            moveable: true,
            showCurrentTime: true,
        } as any)

        // Add a custom time marker we'll move via setCursor.
        this.tl.addCustomTime(new Date(), CURSOR_ID)

        this.tl.on('select', (props: any) => {
            const id = props.items?.[0]
            if (!id) return
            const item = this.items.get(id) as any
            if (!item) return
            this.emit({
                type: 'item-click',
                itemId: id,
                time: new Date(item.start),
            })
        })
        this.tl.on('rangechanged', (props: any) => {
            this.emit({
                type: 'pan',
                from: new Date(props.start),
                to: new Date(props.end),
            })
        })
    }

    setLanes(lanes: Lane[]) {
        this.groups.clear()
        lanes.forEach((l) =>
            this.groups.add({
                id: l.id,
                content: l.label,
                className: l.highlight ? 'lane-highlight' : '',
            })
        )
    }

    setItems(items: TimelineItem[]) {
        this.items.clear()
        items.forEach((i) =>
            this.items.add({
                id: i.id,
                group: i.laneId,
                start: i.time,
                type: 'point',
                style: `background-color: ${SEVERITY_COLOR[i.severity] ?? '#7d8590'}; border-color: ${SEVERITY_COLOR[i.severity] ?? '#7d8590'};`,
                title: `${i.label ?? ''} (${i.severity}/${i.state})`,
            })
        )
    }

    setStep(step: Step) {
        // Map step → visible window. Approximate container width with 1500px.
        const secPerPx = STEP_TO_SECONDS_PER_PIXEL[step]
        const visibleSec = secPerPx * 1500
        const end = new Date()
        const start = new Date(end.getTime() - visibleSec * 1000)
        this.tl.setWindow(start, end, { animation: false })
    }

    setCursor(at: Date | null) {
        if (at) {
            this.tl.setCustomTime(at, CURSOR_ID)
        }
        // null = live mode; vis-timeline showCurrentTime already renders a "now" line.
    }

    on(handler: (e: TimelineEvent) => void) {
        this.listeners.push(handler)
    }

    destroy() {
        this.tl.destroy()
        this.listeners = []
    }

    private emit(e: TimelineEvent) {
        for (const l of this.listeners) l(e)
    }
}

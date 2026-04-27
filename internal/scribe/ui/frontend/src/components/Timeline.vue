<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useTimelineStore } from '@/stores/timeline'
import { api } from '@/api/client'
import { VisTimelineRenderer } from '@/renderers/timeline/VisTimelineRenderer'
import type {
    TimelineRenderer,
    TimelineItem,
    Lane,
} from '@/renderers/timeline/TimelineRenderer'
import type { ScribeEvent, Severity, DiagnosticState } from '@/types'

const store = useTimelineStore()
const container = ref<HTMLElement>()
let renderer: TimelineRenderer | null = null

async function reload() {
    if (!renderer) return
    try {
        const subjects = await api.listSubjects()
        const lanes: Lane[] = subjects.map((id) => ({
            id,
            label: id,
            highlight: store.subjects.includes(id),
        }))
        // Hide non-focused validator lanes if exactly one validator is focused.
        // _chain stays visible; multi-select keeps the selected validators.
        const visible =
            store.subjects.length === 1
                ? lanes.filter(
                      (l) => l.id === '_chain' || l.id === store.subjects[0]
                  )
                : lanes
        renderer.setLanes(visible)

        const r = await api.listEvents({
            kind: 'diagnostic.*',
            severity: store.filters.severity.length
                ? store.filters.severity
                : undefined,
            state:
                store.filters.state.length === 1
                    ? store.filters.state[0]
                    : undefined,
            to: store.at ?? undefined,
            limit: 1000,
        })
        const items: TimelineItem[] = r.events
            .filter((e: ScribeEvent) => visible.some((l) => l.id === e.subject))
            .map((e: ScribeEvent) => ({
                id: e.event_id,
                laneId: e.subject,
                time: new Date(e.time),
                severity: (e.severity ?? 'warning') as Severity,
                state: (e.state ?? 'open') as DiagnosticState,
                label: e.kind,
            }))
        renderer.setItems(items)
        renderer.setStep(store.step)
        renderer.setCursor(store.at)
    } catch {
        // API unavailable — leave the timeline empty.
    }
}

onMounted(() => {
    if (!container.value) return
    renderer = new VisTimelineRenderer(container.value)
    renderer.on((ev) => {
        if (ev.type === 'item-click') store.setAt(ev.time)
    })
    reload()
})

onBeforeUnmount(() => {
    renderer?.destroy()
    renderer = null
})

watch(
    [
        () => store.subjects,
        () => store.filters,
        () => store.at,
        () => store.step,
    ],
    reload,
    { deep: true }
)
watch(
    () => store.at,
    (at) => renderer?.setCursor(at)
)
</script>

<template>
    <div ref="container" class="timeline" />
</template>

<style scoped>
.timeline {
    width: 100%;
    height: 100%;
    min-height: 200px;
    background: #0d1117;
}
:deep(.lane-highlight) {
    background: rgba(88, 166, 255, 0.05);
    border-left: 2px solid #58a6ff;
}
</style>

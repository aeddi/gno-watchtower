<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useTimelineStore } from '@/stores/timeline'
import { api } from '@/api/client'
import { CytoscapeGraphRenderer } from '@/renderers/graph/CytoscapeGraphRenderer'
import type {
    GraphRenderer,
    GraphNode,
    GraphEdge,
} from '@/renderers/graph/GraphRenderer'
import type { Severity } from '@/types'

const store = useTimelineStore()
const container = ref<HTMLElement>()
let renderer: GraphRenderer | null = null

async function reload() {
    if (!renderer) return
    const subjects = (await api.listSubjects()).filter((s) => s !== '_chain')

    // Per-validator severity from open diagnostics at `at`.
    const openDiag = await api.listEvents({
        kind: 'diagnostic.*',
        state: 'open',
        to: store.at ?? undefined,
        limit: 200,
    })
    const sevByNode = new Map<string, GraphNode['severity']>()
    for (const e of openDiag.events) {
        if (!e.severity) continue
        const cur = sevByNode.get(e.subject)
        if (
            !cur ||
            e.severity === 'critical' ||
            (e.severity === 'error' && cur !== 'critical') ||
            (e.severity === 'warning' && cur === 'none')
        ) {
            sevByNode.set(e.subject, e.severity as Severity)
        }
    }

    // Per-validator behind_sentry — fetched from /api/state per subject (parallel).
    const behindSentryByNode = new Map<string, boolean>()
    await Promise.all(
        subjects.map(async (s) => {
            try {
                const st = await api.getState(s, store.at ?? undefined)
                const bs = st.structured?.behind_sentry
                if (typeof bs === 'boolean') behindSentryByNode.set(s, bs)
            } catch {
                /* leave undefined => no shield */
            }
        })
    )

    const nodes: GraphNode[] = subjects.map((id) => ({
        id,
        label: id,
        severity: sevByNode.get(id) ?? 'none',
        behindSentry: behindSentryByNode.get(id) ?? false,
    }))
    renderer.setNodes(nodes)

    // Edges from peer events (current snapshot at `at`).
    // v1: derive from latest peer_connected events for known validators.
    const peerEvents = await api.listEvents({
        kind: 'validator.peer_connected',
        to: store.at ?? undefined,
        limit: 500,
    })
    const edges: GraphEdge[] = []
    const seen = new Set<string>()
    for (const e of peerEvents.events) {
        const peer = ((e.payload as Record<string, unknown>)?.peer_id ??
            (e.payload as Record<string, unknown>)?.peer) as string | undefined
        if (!peer || !subjects.includes(peer)) continue
        const key = [e.subject, peer].sort().join('|')
        if (seen.has(key)) continue
        seen.add(key)
        edges.push({ source: e.subject, target: peer })
    }
    renderer.setEdges(edges)

    renderer.setLayout(store.graphLayout)
    renderer.setSelection(store.subjects)
}

onMounted(() => {
    if (!container.value) return
    renderer = new CytoscapeGraphRenderer(container.value)
    renderer.on((ev) => {
        if (ev.type === 'node-click') {
            if (ev.multi) store.toggleSubject(ev.id)
            else store.setSubjects([ev.id])
        } else if (ev.type === 'background-click') {
            store.setSubjects([])
        }
    })
    reload()
})

onBeforeUnmount(() => {
    renderer?.destroy()
    renderer = null
})

watch([() => store.at, () => store.graphLayout], reload, { deep: true })
watch(
    () => store.subjects,
    (ids) => renderer?.setSelection(ids)
)
</script>

<template>
    <div ref="container" class="graph" />
</template>

<style scoped>
.graph {
    width: 100%;
    height: 100%;
    min-height: 200px;
    background: #0d1117;
}
</style>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch, nextTick } from 'vue'
import { useTimelineStore } from '@/stores/timeline'
import { api } from '@/api/client'
import { CytoscapeGraphRenderer } from '@/renderers/graph/CytoscapeGraphRenderer'
import type {
    GraphRenderer,
    GraphNode,
    GraphEdge,
} from '@/renderers/graph/GraphRenderer'
import type { Severity } from '@/types'
import GraphLayoutSwitch from './GraphLayoutSwitch.vue'

const store = useTimelineStore()
const container = ref<HTMLElement>()
const lastNodes = ref<GraphNode[]>([])
const lastEdges = ref<GraphEdge[]>([])
let renderer: GraphRenderer | null = null

async function reload() {
    try {
        const subjects = (await api.listSubjects()).filter(
            (s) => s !== '_chain'
        )

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

        // Per-validator behind_sentry comes from `fast_scalars` (sample-derived),
        // not `structured` (event-projected). The store-side `behind_sentry`
        // column is populated by the sentinel metric handler when it lands;
        // until then it's NULL and the shield stays off.
        const behindSentryByNode = new Map<string, boolean>()
        await Promise.all(
            subjects.map(async (s) => {
                try {
                    const st = await api.getState(s, store.at ?? undefined)
                    const bs = st.fast_scalars?.behind_sentry
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

        // Edges from peer events (current snapshot at `at`).
        const peerEvents = await api.listEvents({
            kind: 'validator.peer_connected',
            to: store.at ?? undefined,
            limit: 500,
        })
        const edges: GraphEdge[] = []
        const seen = new Set<string>()
        for (const e of peerEvents.events) {
            const peer = ((e.payload as Record<string, unknown>)?.peer_id ??
                (e.payload as Record<string, unknown>)?.peer) as
                | string
                | undefined
            if (!peer || !subjects.includes(peer)) continue
            const key = [e.subject, peer].sort().join('|')
            if (seen.has(key)) continue
            seen.add(key)
            edges.push({ source: e.subject, target: peer })
        }

        lastNodes.value = nodes
        lastEdges.value = edges

        if (renderer) {
            renderer.setNodes(nodes)
            renderer.setEdges(edges)
            renderer.setLayout(store.graphLayout)
            renderer.setSelection(store.subjects)
        }
    } catch {
        // API unavailable — leave the graph empty.
    }
}

function mountRenderer() {
    if (!container.value) return
    renderer?.destroy()
    renderer = new CytoscapeGraphRenderer(container.value)
    renderer.on((ev) => {
        if (ev.type === 'node-click') {
            if (ev.multi) store.toggleSubject(ev.id)
            else store.setSubjects([ev.id])
        } else if (ev.type === 'background-click') {
            store.setSubjects([])
        }
    })
    // Push current data into the new renderer if we have it.
    if (lastNodes.value.length) {
        renderer.setNodes(lastNodes.value)
        renderer.setEdges(lastEdges.value)
        renderer.setLayout(store.graphLayout)
        renderer.setSelection(store.subjects)
    }
}

onMounted(async () => {
    if (store.graphLayout !== 'table') mountRenderer()
    await reload()
})

onBeforeUnmount(() => {
    renderer?.destroy()
    renderer = null
})

// Switching layout between table and graph: tear down or remount the renderer.
watch(
    () => store.graphLayout,
    async (next, prev) => {
        if (prev === 'table' && next !== 'table') {
            await nextTick()
            mountRenderer()
        } else if (next === 'table' && prev !== 'table') {
            renderer?.destroy()
            renderer = null
        } else if (renderer) {
            renderer.setLayout(next)
        }
    }
)

watch([() => store.at], reload, { deep: true })
watch(
    () => store.subjects,
    (ids) => renderer?.setSelection(ids)
)

function severityColor(s: GraphNode['severity']) {
    switch (s) {
        case 'critical':
        case 'error':
            return '#ef4444'
        case 'warning':
            return '#f59e0b'
        default:
            return '#3fb950'
    }
}

function onRowClick(e: MouseEvent, id: string) {
    if (e.metaKey || e.ctrlKey) store.toggleSubject(id)
    else store.setSubjects([id])
}
</script>

<template>
    <div class="wrapper">
        <header class="ctrls">
            <GraphLayoutSwitch />
        </header>
        <div v-if="store.graphLayout === 'table'" class="table-view">
            <table>
                <thead>
                    <tr>
                        <th>validator</th>
                        <th>severity</th>
                        <th>shield</th>
                    </tr>
                </thead>
                <tbody>
                    <tr
                        v-for="n in lastNodes"
                        :key="n.id"
                        :class="{ selected: store.subjects.includes(n.id) }"
                        @click="onRowClick($event, n.id)"
                    >
                        <td>{{ n.id }}</td>
                        <td :style="{ color: severityColor(n.severity) }">
                            {{ n.severity }}
                        </td>
                        <td>{{ n.behindSentry ? '🛡' : '' }}</td>
                    </tr>
                </tbody>
            </table>
        </div>
        <div v-else ref="container" class="graph" />
    </div>
</template>

<style scoped>
.wrapper {
    display: flex;
    flex-direction: column;
    height: 100%;
}
.ctrls {
    padding: 0.4rem;
    border-bottom: 1px solid #30363d;
}
.graph {
    flex: 1;
    background: #0d1117;
}
.table-view {
    flex: 1;
    overflow-y: auto;
    padding: 0.6rem;
    background: #0d1117;
}
.table-view table {
    width: 100%;
    border-collapse: collapse;
    font-family: ui-monospace, monospace;
    font-size: 0.8rem;
}
.table-view th,
.table-view td {
    text-align: left;
    padding: 0.3rem 0.6rem;
    border-bottom: 1px solid #30363d;
}
.table-view tr.selected {
    background: rgba(88, 166, 255, 0.12);
}
.table-view tr:hover {
    background: rgba(255, 255, 255, 0.04);
    cursor: pointer;
}
</style>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ScribeEvent } from '@/types'

const props = defineProps<{ event: ScribeEvent }>()
const emit = defineEmits<{ select: [time: string] }>()

const expanded = ref(false)

const severityColor = computed(() => {
    switch (props.event.severity) {
        case 'critical':
            return '#ef4444'
        case 'error':
            return '#ef4444'
        case 'warning':
            return '#f59e0b'
        default:
            return '#7d8590'
    }
})

const stateLabel = computed(() => props.event.state?.toUpperCase() ?? '—')

const summary = computed(() => {
    const p = props.event.payload as Record<string, unknown>
    if (typeof p?.p95_seconds === 'number')
        return `p95=${(p.p95_seconds as number).toFixed(1)}s`
    if (
        typeof p?.online_count === 'number' &&
        typeof p?.valset_size === 'number'
    ) {
        return `online=${p.online_count}/${p.valset_size}`
    }
    if (typeof p?.height === 'number') return `height=${p.height}`
    return ''
})

function toggle() {
    expanded.value = !expanded.value
    emit('select', props.event.time)
}
</script>

<template>
    <div class="diag" :style="{ borderLeftColor: severityColor }">
        <div data-testid="header" class="header" @click="toggle">
            <span class="kind">{{
                event.kind.replace(/^diagnostic\./, '')
            }}</span>
            <span class="state" :style="{ color: severityColor }">{{
                stateLabel
            }}</span>
            <span class="subject">{{ event.subject }}</span>
            <span class="time">{{
                new Date(event.time).toLocaleTimeString()
            }}</span>
            <span v-if="summary" class="summary">{{ summary }}</span>
        </div>

        <div v-if="expanded" data-testid="expanded" class="expanded">
            <div class="payload">
                <div v-for="(v, k) in event.payload" :key="k" class="kv">
                    <span class="key">{{ k }}:</span>
                    <span class="val">{{ v }}</span>
                </div>
            </div>

            <div
                v-if="event.provenance?.linked_signals?.length"
                class="signals"
            >
                <div class="label">linked signals:</div>
                <ul>
                    <li
                        v-for="(sig, i) in event.provenance.linked_signals"
                        :key="i"
                    >
                        <a :href="sig.url" target="_blank" rel="noopener"
                            >{{ sig.type }}: {{ sig.query }}</a
                        >
                    </li>
                </ul>
            </div>

            <div v-if="event.provenance?.doc_ref" class="doc">
                <a
                    :href="event.provenance.doc_ref"
                    target="_blank"
                    rel="noopener"
                    >📖 {{ event.provenance.doc_ref }}</a
                >
            </div>

            <div
                v-if="event.provenance?.source_event_ids?.length"
                class="sources"
            >
                <div class="label">source events:</div>
                <ul>
                    <li
                        v-for="id in event.provenance.source_event_ids"
                        :key="id"
                    >
                        <code>{{ id }}</code>
                    </li>
                </ul>
            </div>

            <div v-if="event.recovers" class="paired">
                <span class="label">paired open event:</span>
                <code>{{ event.recovers }}</code>
            </div>
        </div>
    </div>
</template>

<style scoped>
.diag {
    padding: 0.6rem 0.8rem;
    margin-bottom: 0.4rem;
    background: #161b22;
    border-left: 3px solid;
    cursor: pointer;
}
.header {
    display: flex;
    gap: 0.6rem;
    align-items: center;
    flex-wrap: wrap;
}
.kind {
    font-weight: 600;
}
.state {
    font-size: 0.7rem;
    padding: 1px 6px;
    border-radius: 3px;
    background: rgba(255, 255, 255, 0.06);
}
.subject {
    color: #7d8590;
    font-family: ui-monospace, monospace;
}
.time {
    color: #7d8590;
    margin-left: auto;
}
.summary {
    color: #9da5b4;
    flex-basis: 100%;
    padding-left: 0;
}
.expanded {
    margin-top: 0.6rem;
    padding-left: 0.4rem;
    border-top: 1px solid #30363d;
    padding-top: 0.6rem;
}
.kv {
    font-family: ui-monospace, monospace;
    font-size: 0.85rem;
}
.key {
    color: #7d8590;
    margin-right: 0.4rem;
}
.signals,
.sources,
.doc,
.paired {
    margin-top: 0.5rem;
}
.label {
    color: #7d8590;
    font-size: 0.7rem;
}
.signals ul,
.sources ul {
    margin: 0.3rem 0 0 1.2rem;
    padding: 0;
}
a {
    color: #58a6ff;
    text-decoration: none;
}
a:hover {
    text-decoration: underline;
}
</style>

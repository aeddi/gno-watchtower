<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NSpace } from 'naive-ui'
import { useTimelineStore } from '@/stores/timeline'

const store = useTimelineStore()
const isLive = computed(() => store.liveMode)

function copyPermalink() {
    navigator.clipboard.writeText(window.location.href)
}
</script>

<template>
    <header class="topbar">
        <span class="brand">scribe</span>
        <span class="status">
            <span v-if="isLive" class="live-dot" />
            <span v-if="isLive">live</span>
            <span v-else>paused @ {{ store.at?.toLocaleString() }}</span>
        </span>
        <n-space size="small">
            <n-button v-if="!isLive" size="small" @click="store.goLive()"
                >go live</n-button
            >
            <n-button size="small" tertiary @click="copyPermalink"
                >copy permalink</n-button
            >
        </n-space>
    </header>
</template>

<style scoped>
.topbar {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.6rem 1rem;
    border-bottom: 1px solid #30363d;
}
.brand {
    font-weight: 600;
}
.status {
    display: inline-flex;
    gap: 0.4rem;
    align-items: center;
    color: #7d8590;
}
.live-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #3fb950;
}
.topbar > :last-child {
    margin-left: auto;
}
</style>

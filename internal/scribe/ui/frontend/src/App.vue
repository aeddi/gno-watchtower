<script setup lang="ts">
import { computed } from 'vue'
import { NConfigProvider, darkTheme, lightTheme } from 'naive-ui'
import { useTimelineStore } from '@/stores/timeline'
import TopBar from '@/components/TopBar.vue'
import NetworkGraph from '@/components/NetworkGraph.vue'
import StatePanel from '@/components/StatePanel.vue'
import Minimap from '@/components/Minimap.vue'
import Timeline from '@/components/Timeline.vue'
import DiagnosticPane from '@/components/DiagnosticPane.vue'

const store = useTimelineStore()

const theme = computed(() => {
    if (store.theme === 'light') return lightTheme
    if (store.theme === 'dark') return darkTheme
    // system
    return matchMedia('(prefers-color-scheme: light)').matches
        ? lightTheme
        : darkTheme
})
</script>

<template>
    <n-config-provider :theme="theme">
        <main class="app">
            <TopBar />
            <section class="top">
                <div class="graph"><NetworkGraph /></div>
                <div class="state"><StatePanel /></div>
            </section>
            <section class="middle">
                <Minimap />
                <Timeline />
            </section>
            <section class="bottom">
                <DiagnosticPane />
            </section>
        </main>
    </n-config-provider>
</template>

<style>
:root {
    color-scheme: dark;
}
body {
    margin: 0;
    font-family: ui-sans-serif, system-ui, sans-serif;
    background: #0d1117;
    color: #c9d1d9;
    height: 100vh;
    overflow: hidden;
}
.app {
    height: 100vh;
    display: grid;
    grid-template-rows: auto 40fr 25fr 35fr;
}
.top {
    display: grid;
    grid-template-columns: 3fr 1fr;
    border-bottom: 1px solid #30363d;
    min-height: 0;
}
.graph,
.state {
    min-height: 0;
    overflow: hidden;
}
.state {
    border-left: 1px solid #30363d;
}
.middle {
    display: flex;
    flex-direction: column;
    border-bottom: 1px solid #30363d;
    min-height: 0;
}
.bottom {
    min-height: 0;
    overflow: hidden;
}
</style>

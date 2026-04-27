<script setup lang="ts">
import { NConfigProvider, darkTheme } from 'naive-ui'
import { computed } from 'vue'
import DiagnosticPane from '@/components/DiagnosticPane.vue'

const theme = computed(() => darkTheme)
</script>

<template>
    <n-config-provider :theme="theme">
        <main class="root">
            <h1>scribe</h1>
            <DiagnosticPane class="pane" />
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
.root {
    padding: 1rem;
    height: 100vh;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
}
.root h1 {
    margin: 0 0 0.6rem 0;
    font-size: 1.1rem;
    color: #7d8590;
    font-weight: 500;
}
.pane {
    flex: 1;
    min-height: 0;
}
</style>

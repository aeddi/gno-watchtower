<script setup lang="ts">
import { NConfigProvider, darkTheme } from 'naive-ui'
import { computed } from 'vue'

// v1 default: dark theme. Theme switcher lands in Phase 10.
const theme = computed(() => darkTheme)
</script>

<template>
    <n-config-provider :theme="theme">
        <main class="root">
            <h1>scribe timeline UI</h1>
            <p>scaffold up. region components land in later phases.</p>
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
}
.root {
    padding: 1.5rem;
}
</style>

<script setup lang="ts">
import { computed } from 'vue';
import type { HandlingLevel } from '../../types/handling-advice';
const props = defineProps<{ level: HandlingLevel | string }>();
const label = computed<Record<string, string>>(() => ({
  observe: '保持观察',
  restrict: '限行处置',
  urgent: '立即处置',
}));
const tone = computed(() => ({ observe: 'success', restrict: 'warning', urgent: 'danger' }[props.level] || 'neutral'));
</script>
<template><span :class="`status status--${tone}`">{{ label[level] || level }}</span></template>

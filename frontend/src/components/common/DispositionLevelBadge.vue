<script setup lang="ts">
import { computed } from 'vue';
import { DISPOSITION_LEVEL_LABELS, type DispositionLevel } from '../../types/status';

const props = defineProps<{ level: string }>();
const label = computed(() => DISPOSITION_LEVEL_LABELS[props.level as DispositionLevel] || props.level);
const tone = computed(() => {
  if (props.level === 'urgent') return 'danger';
  if (props.level === 'restrict') return 'warning';
  return 'success';
});
</script>
<template><span :class="`disposition disposition--${tone}`">{{ label }}</span></template>

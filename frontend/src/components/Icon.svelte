<script lang="ts">
  import { ICONS } from '../lib/icons'
  let { name, size = 16, fill = 'currentColor' }: { name: string; size?: number; fill?: string } = $props()
</script>

<svg width={size} height={size} viewBox="0 -960 960 960" aria-hidden="true" style="flex:none;display:block"><path d={ICONS[name] ?? ''} {fill} /></svg>

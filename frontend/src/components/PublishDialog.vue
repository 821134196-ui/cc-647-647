<template>
  <div class="modal-mask" @click.self="$emit('close')">
    <div class="card modal">
      <h3>发布新版公示榜单</h3>
      <div v-if="pendingCount" class="banner">
        当前项目仍有 <strong>{{ pendingCount }}</strong> 条泳道处于待裁定状态，系统将拒绝发布。
        请先在“泳道原始数据与复核”中完成全部漏记/争议裁定（撤回也算终局）。
      </div>
      <div v-else class="banner info">
        所有泳道均已有终局结果。发布后将生成<strong>新版本</strong>，旧榜单完整保留、原始计时读数不变，观众即可看到带更正标注的新榜单。
      </div>

      <div style="margin:12px 0">
        <label>更正原因（将随榜单公示并写入历史）</label>
        <textarea v-model="note" rows="3"
          placeholder="如：第4道触板漏记，经三表手记与录像复核，采用手动成绩52.94"></textarea>
      </div>

      <div class="muted small" style="margin-bottom:14px">
        系统会自动比对上一版，生成人读更正摘要并逐行标注“本版更正”（成绩/名次/状态变化）。
      </div>

      <div class="row" style="justify-content:flex-end;gap:10px">
        <button @click="$emit('close')">取消</button>
        <button class="primary" :disabled="pendingCount > 0 || publishing" @click="publish">
          {{ publishing ? '发布中…' : '确认发布' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../api'

const props = defineProps({ eventId: Number, pendingCount: Number })
const emit = defineEmits(['close', 'published'])

const note = ref('')
const publishing = ref(false)

async function publish() {
  publishing.value = true
  try {
    const res = await api(`/events/${props.eventId}/publish`, {
      method: 'POST',
      body: { change_note: note.value || '常规公示' }
    })
    emit('published', res)
  } catch (e) {
    alert('发布失败：' + e.message)
  } finally {
    publishing.value = false
  }
}
</script>

<template>
  <div class="modal-mask" @click.self="$emit('close')">
    <div class="card modal">
      <h3>新建比赛项目</h3>
      <div style="margin:10px 0">
        <label>项目名称</label>
        <input v-model="form.name" placeholder="如 女子100米仰泳 决赛" />
      </div>
      <div class="row wrap" style="gap:10px">
        <div class="grow">
          <label>距离（米）</label>
          <input v-model.number="form.distance" type="number" />
        </div>
        <div class="grow">
          <label>泳姿</label>
          <select v-model="form.stroke">
            <option>自由泳</option><option>蛙泳</option><option>仰泳</option>
            <option>蝶泳</option><option>混合泳</option>
          </select>
        </div>
        <div class="grow">
          <label>轮次</label>
          <select v-model="form.round">
            <option>预赛</option><option>半决赛</option><option>决赛</option>
          </select>
        </div>
      </div>
      <div style="margin:12px 0">
        <label>泳道数（报名名单）</label>
        <input v-model.number="laneCount" type="number" min="1" max="10" />
      </div>
      <div class="row" style="justify-content:flex-end;gap:10px">
        <button @click="$emit('close')">取消</button>
        <button class="primary" @click="submit">创建并编排泳道</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../api'

const props = defineProps({ meetId: Number })
const emit = defineEmits(['close', 'created'])

const form = ref({ name: '', distance: 50, stroke: '自由泳', round: '决赛' })
const laneCount = ref(8)

async function submit() {
  if (!form.value.name.trim()) return alert('请填写项目名称')
  try {
    const ev = await api(`/meets/${props.meetId}/events`, { method: 'POST', body: form.value })
    const lanes = []
    for (let i = 1; i <= laneCount.value; i++) {
      lanes.push({ lane_no: i, swimmer_name: `待报名选手${i}`, team: '待定' })
    }
    if (lanes.length) {
      await api(`/events/${ev.id}/lanes`, { method: 'POST', body: { lanes } })
    }
    emit('created', ev)
  } catch (e) { alert('创建失败：' + e.message) }
}
</script>

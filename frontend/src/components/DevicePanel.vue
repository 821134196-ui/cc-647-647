<template>
  <div class="card" style="margin-bottom:14px;border-color:#7e22ce">
    <div class="row" style="justify-content:space-between">
      <strong style="color:#d8b4fe">📟 电子计时设备 · 本地模拟接口</strong>
      <span class="muted small">POST /api/device/readings（读数只追加，不覆盖）</span>
    </div>

    <div v-if="!eventId" class="muted" style="margin-top:10px">请先选择比赛项目。</div>
    <template v-else>
      <div class="row wrap" style="gap:10px;margin-top:12px">
        <div style="width:110px">
          <label>泳道号</label>
          <input v-model.number="laneNo" type="number" min="1" max="10" />
        </div>
        <div style="width:150px">
          <label>读数节点</label>
          <select v-model="splitIndex">
            <option :value="0">触壁总成绩</option>
            <option :value="1">50m 分段</option>
            <option :value="2">100m 分段</option>
            <option :value="3">150m 分段</option>
          </select>
        </div>
        <div style="width:160px">
          <label>成绩（漏记时留空）</label>
          <input v-model="time" class="mono" :disabled="missed" placeholder="如 52.35" />
        </div>
        <div style="width:150px">
          <label>设备来源</label>
          <select v-model="source">
            <option value="pad">pad 触板</option>
            <option value="button">button 备用按钮</option>
            <option value="watch">watch 设备表</option>
          </select>
        </div>
        <div class="grow">
          <label>设备原始报文</label>
          <input v-model="deviceRaw" class="mono" placeholder="PAD-TOUCH-OK / PAD-TOUCH-TIMEOUT" />
        </div>
        <div style="padding-top:20px;display:flex;gap:8px">
          <label style="margin:0;display:flex;align-items:center;gap:6px;white-space:nowrap">
            <input type="checkbox" v-model="missed" style="width:auto" /> 漏记
          </label>
        </div>
      </div>
      <div class="row" style="gap:10px;margin-top:14px">
        <button class="primary" @click="submit(false)">上报读数</button>
        <button class="danger" @click="submit(true)">模拟漏记上报（自动立案）</button>
        <button class="sm ghost" @click="simulateRace">一键模拟整场（含第4道漏记）</button>
      </div>
      <div class="muted small" style="margin-top:8px">
        漏记上报后系统会自动创建 open 复核案件；之后再补传有效读数也只新增记录，历史漏记读数保留可查。
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api, parseTimeInput } from '../api'

const props = defineProps({ eventId: Number })
const emit = defineEmits(['changed'])

const laneNo = ref(4)
const splitIndex = ref(0)
const time = ref('')
const source = ref('pad')
const deviceRaw = ref('PAD-TOUCH-OK')
const missed = ref(false)

async function postReading(payload) {
  return api('/device/readings', { method: 'POST', body: payload })
}

async function submit(forceMissed) {
  const isMissed = forceMissed || missed.value
  const payload = {
    event_id: props.eventId,
    lane_no: laneNo.value,
    split_index: splitIndex.value,
    split_label: splitIndex.value === 0 ? '触壁' : ['50m', '100m', '150m'][splitIndex.value - 1],
    source: source.value,
    device_raw: isMissed ? (deviceRaw.value || 'PAD-TOUCH-TIMEOUT') : deviceRaw.value,
    missed: isMissed
  }
  if (!isMissed) {
    const ms = parseTimeInput(time.value)
    if (ms == null) return alert('请填写有效成绩，如 52.35；或勾选漏记')
    payload.time_ms = ms
  }
  try {
    const res = await postReading(payload)
    alert(isMissed
      ? `已上报漏记${res.auto_case_opened ? '，系统已自动立案复核' : '（已有进行中案件）'}`
      : '读数已记录')
    emit('changed')
  } catch (e) { alert('上报失败：' + e.message) }
}

// 便捷模拟：向当前项目的 8 条泳道各上报一个成绩，第4道漏记
async function simulateRace() {
  const times = [52350, 51880, 53120, null, 52760, 51880, 54010, 55230]
  for (let lane = 1; lane <= 8; lane++) {
    const t = times[lane - 1]
    const payload = {
      event_id: props.eventId, lane_no: lane, split_index: 0,
      split_label: '触壁', source: 'pad',
      device_raw: t == null ? 'PAD-TOUCH-TIMEOUT' : 'PAD-TOUCH-OK',
      missed: t == null
    }
    if (t != null) payload.time_ms = t
    try { await postReading(payload) } catch (e) { /* 泳道可能不存在，忽略 */ }
  }
  alert('模拟完成：8条泳道已上报，第4道漏记并自动立案（重复运行会追加读数，历史不覆盖）')
  emit('changed')
}
</script>

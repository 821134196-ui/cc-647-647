<template>
  <div>
    <div class="card" style="margin-bottom:14px">
      <div class="row" style="justify-content:space-between;align-items:flex-start" v-if="detail">
        <div>
          <h2 style="margin:0 0 4px">{{ detail.event.name }}</h2>
          <div class="muted small">
            {{ detail.event.distance }}米 {{ detail.event.stroke }} · {{ detail.event.round }}
            ｜共 {{ detail.lanes.length }} 条泳道 ｜数据时间 {{ detail.server_time }}
          </div>
        </div>
        <div class="row" style="gap:8px">
          <button v-if="isChief" class="primary" @click="publishDialog = true">发布新版榜单</button>
          <button class="sm" @click="reload">刷新</button>
        </div>
      </div>
      <div v-if="current" class="banner info" style="margin-top:12px;margin-bottom:0">
        <div class="row" style="justify-content:space-between;gap:10px">
          <strong>当前公示版：第 {{ current.version }} 版</strong>
          <span class="muted small">{{ fmtTime(current.created_at) }} · {{ current.publisher_name }} 发布</span>
        </div>
        <div class="small" v-if="current.change_note">更正原因：{{ current.change_note }}</div>
        <div class="small">更正内容：{{ current.correction || '首版公示' }}</div>
      </div>
      <div v-else class="banner" style="margin-top:12px;margin-bottom:0">
        该项目尚未发布公示榜单。裁判可在下方查看原始读数与实时排名，发布须由总裁判完成。
      </div>
    </div>

    <div class="tabs">
      <button :class="{ active: tab === 'board' }" @click="tab = 'board'">🏊 公示榜单</button>
      <button :class="{ active: tab === 'lanes' }" @click="tab = 'lanes'">
        📟 泳道原始数据与复核
        <span v-if="openCaseCount" class="tag open" style="margin-left:6px">{{ openCaseCount }} 待裁定</span>
      </button>
      <button :class="{ active: tab === 'live' }" @click="tab = 'live'">⏱ 裁判席实时排名</button>
      <button :class="{ active: tab === 'history' }" @click="tab = 'history'; loadVersions()">🗂 历史版本</button>
    </div>

    <!-- 公示榜单 -->
    <div v-if="tab === 'board'" class="card">
      <table v-if="current">
        <thead>
          <tr>
            <th style="width:70px">名次</th><th style="width:60px">泳道</th>
            <th>运动员</th><th>代表队</th><th>成绩</th><th>依据</th><th>状态</th><th>更正</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in current.entries" :key="e.id">
            <td><span class="rank-badge" :class="'rank-' + (e.rank||'')">{{ e.display_rank }}</span></td>
            <td class="mono">{{ e.lane_no }}</td>
            <td>{{ e.swimmer_name }}</td>
            <td class="muted">{{ e.team }}</td>
            <td class="mono" style="font-size:15px">{{ e.time_text || '—' }}</td>
            <td>
              <span v-if="e.source" class="tag" :class="e.source">{{ sourceNames[e.source] || e.source }}</span>
              <span v-else class="muted small">—</span>
            </td>
            <td>
              <span v-if="e.special_state === 'tie'" class="tag tie">并列</span>
              <span v-else-if="e.special_state === 'swimoff'" class="tag swimoff">重赛</span>
              <span v-else-if="e.special_state === 'withdrawn'" class="tag withdrawn">撤回</span>
              <span v-else class="muted small">正常</span>
            </td>
            <td>
              <span v-if="e.corrected" class="tag corrected">本版更正</span>
              <span v-else class="muted small">—</span>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="muted">暂无已发布榜单。</div>
    </div>

    <!-- 泳道原始数据与复核 -->
    <div v-if="tab === 'lanes'">
      <LaneCard
        v-for="d in detail?.lanes || []" :key="d.lane.id"
        :detail="d" :user="user" @toast="(m,k)=>$emit('toast',m,k)" @changed="reload"
      />
    </div>

    <!-- 实时排名（未发布） -->
    <div v-if="tab === 'live'" class="card">
      <div class="muted small" style="margin-bottom:8px">
        实时排依据原始读数与已裁定结果即时计算，仅裁判内部参考，不对观众公示。
      </div>
      <table>
        <thead><tr><th style="width:70px">名次</th><th style="width:60px">泳道</th><th>运动员</th><th>成绩</th><th>依据</th><th>状态</th></tr></thead>
        <tbody>
          <tr v-for="e in liveEntries" :key="e.lane_id">
            <td><span class="rank-badge" :class="'rank-' + (e.rank||'')">{{ e.display_rank }}</span></td>
            <td class="mono">{{ e.lane_no }}</td>
            <td>{{ e.swimmer_name }}</td>
            <td class="mono">{{ e.time_text || '—' }}</td>
            <td><span v-if="e.source" class="tag" :class="e.source">{{ sourceNames[e.source] }}</span></td>
            <td>
              <span v-if="e.special_state === 'withdrawn'" class="tag withdrawn">撤回</span>
              <span v-else-if="e.special_state === 'tie'" class="tag tie">并列</span>
              <span v-else-if="e.special_state === 'swimoff'" class="tag swimoff">重赛</span>
              <span v-else-if="e.rank === 0" class="tag open">待裁定</span>
              <span v-else class="muted small">正常</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 历史版本 -->
    <div v-if="tab === 'history'" class="card">
      <div v-if="!versions.length" class="muted">暂无历史版本。</div>
      <div v-for="b in versions" :key="b.id" class="card" style="margin-bottom:10px;padding:12px"
           :style="b.is_current ? {borderColor:'var(--accent-2)'} : {}">
        <div class="row" style="justify-content:space-between">
          <div class="row" style="gap:8px">
            <strong>第 {{ b.version }} 版</strong>
            <span v-if="b.is_current" class="tag electronic">当前公示</span>
            <span class="muted small">{{ fmtTime(b.created_at) }} · {{ b.publisher_name }}</span>
          </div>
          <button class="sm" @click="viewVersion(b.version)">查看该版原始快照</button>
        </div>
        <div class="small muted" style="margin-top:6px" v-if="b.change_note">原因：{{ b.change_note }}</div>
        <div class="small muted">摘要：{{ b.correction }}（{{ b.entries_count }} 条）</div>

        <table v-if="viewingVersion === b.version" style="margin-top:10px">
          <thead><tr><th style="width:70px">名次</th><th style="width:60px">泳道</th><th>运动员</th><th>成绩</th><th>依据</th><th>状态</th><th>更正</th></tr></thead>
          <tbody>
            <tr v-for="e in versionEntries" :key="e.id">
              <td><span class="rank-badge">{{ e.display_rank }}</span></td>
              <td class="mono">{{ e.lane_no }}</td>
              <td>{{ e.swimmer_name }}</td>
              <td class="mono">{{ e.time_text || '—' }}</td>
              <td><span v-if="e.source" class="tag" :class="e.source">{{ sourceNames[e.source] }}</span></td>
              <td>
                <span v-if="e.special_state === 'tie'" class="tag tie">并列</span>
                <span v-else-if="e.special_state === 'swimoff'" class="tag swimoff">重赛</span>
                <span v-else-if="e.special_state === 'withdrawn'" class="tag withdrawn">撤回</span>
              </td>
              <td><span v-if="e.corrected" class="tag corrected">更正</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <PublishDialog
      v-if="publishDialog"
      :event-id="eventId" :pending-count="openCaseCount"
      @close="publishDialog = false" @published="onPublished"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, SOURCE_NAMES } from '../api'
import LaneCard from './LaneCard.vue'
import PublishDialog from './PublishDialog.vue'

const props = defineProps({ eventId: Number, user: Object })
const emit = defineEmits(['toast'])

const detail = ref(null)
const liveEntries = ref([])
const versions = ref([])
const versionEntries = ref([])
const viewingVersion = ref(null)
const publishDialog = ref(false)
const tab = ref('board')

const current = computed(() => detail.value?.current_board || null)
const openCaseCount = computed(() =>
  (detail.value?.lanes || []).filter(d => d.cases.some(c => c.status === 'open')).length
)
const sourceNames = SOURCE_NAMES

function fmtTime(t) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}

async function reload() {
  detail.value = await api(`/events/${props.eventId}`)
  if (tab.value === 'live') await loadLive()
}

async function loadLive() {
  const res = await api(`/events/${props.eventId}/live`)
  liveEntries.value = res.entries
}

async function loadVersions() {
  versions.value = await api(`/events/${props.eventId}/boards`)
}

async function viewVersion(v) {
  if (viewingVersion.value === v) {
    viewingVersion.value = null
    return
  }
  const board = await api(`/events/${props.eventId}/boards/${v}`)
  viewingVersion.value = v
  versionEntries.value = board.entries
}

async function onPublished() {
  publishDialog.value = false
  tab.value = 'board'
  await reload()
  emit('toast', '新版榜单已发布并公示，旧版已归档')
}

onMounted(reload)
defineExpose({ reload })
</script>

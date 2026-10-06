<template>
  <div>
    <div class="panel">
      <div class="row">
        <div>
          <div style="font-size:17px;font-weight:700">{{ race.event_name }} · 榜单版本</div>
          <div class="muted">{{ race.code }} · {{ race.round }} · 发布新版本只新增快照，旧版本永久可查</div>
        </div>
        <span class="spacer"></span>
        <button class="ghost" @click="reload">🔄 刷新</button>
        <button v-if="user.role === 'chief'" @click="showPublish = !showPublish">📢 发布{{ versions.length ? '更正' : '新版' }}榜单</button>
      </div>
      <div v-if="banner" class="banner" :class="banner.type" style="margin-top:10px">{{ banner.text }}</div>

      <div v-if="showPublish && user.role === 'chief'" style="margin-top:12px">
        <label>更正原因{{ versions.length ? '（必填，将与更正时间一起公示）' : '' }}</label>
        <input v-if="!versions.length" v-model="reason" placeholder="初次发布可留空" />
        <textarea v-else v-model="reason"
          placeholder="例：3 道电子触板漏记，依总裁判裁定 #2 采用手动成绩 55.67；5/6 道并列已重赛决出名次"></textarea>
        <div class="row" style="margin-top:8px">
          <button @click="publish">确认发布第 {{ versions.length + 1 }} 版</button>
          <button class="ghost" @click="showPublish = false">取消</button>
        </div>
      </div>
    </div>

    <div class="panel" v-if="!versions.length">
      <span class="muted">该项目尚未发布榜单。{{ user.role === 'chief' ? '由总裁判发布初版。' : '请等待总裁判发布。' }}</span>
    </div>

    <div class="panel" v-if="versions.length && !selectedId">
      <h3>版本列表</h3>
      <div
        v-for="b in versions" :key="b.id"
        class="history-item" :class="{ current: b.status === 'CURRENT' }"
        @click="openBoard(b.id)"
      >
        <div class="ver-big">V{{ b.version_no }}</div>
        <div>
          <div :class="b.status === 'CURRENT' ? 'badge-current' : 'badge-super'">
            {{ b.status === 'CURRENT' ? '● 当前公示版' : '已被更正（历史版本）' }}
          </div>
          <div class="muted" style="font-size:12px">
            发布时间 {{ fmtTime(b.created_at) }} · {{ b.publisher_name }}
          </div>
          <div v-if="b.version_no > 1" style="font-size:12px;margin-top:2px">
            <span class="chip missed">更正原因</span>{{ b.correction_reason }}
          </div>
        </div>
      </div>
    </div>

    <div v-if="current">
      <div class="panel">
        <div class="board-head">
          <div class="ver-big">第 {{ current.board.version_no }} 版榜单</div>
          <span :class="current.board.status === 'CURRENT' ? 'badge-current' : 'badge-super'">
            {{ current.board.status === 'CURRENT' ? '● 当前公示中' : '历史版本（已被更正）' }}
          </span>
          <span class="spacer"></span>
          <button class="ghost tiny" @click="selectedId = null">返回版本列表</button>
          <button class="ghost tiny" v-if="current.previous" @click="openBoard(current.previous.id)">← 上一版 V{{ current.previous.version_no }}</button>
          <button class="ghost tiny" v-if="current.next" @click="openBoard(current.next.id)">下一版 V{{ current.next.version_no }} →</button>
        </div>
        <div class="muted" style="margin-top:6px;font-size:12px">
          发布时间：{{ fmtTime(current.board.created_at) }} · 发布人：{{ current.board.publisher_name }}
          <template v-if="current.board.version_no > 1">
            <br />更正原因：<b style="color:var(--warn)">{{ current.board.correction_reason }}</b>
          </template>
        </div>
      </div>

      <div class="panel">
        <table>
          <thead>
            <tr>
              <th style="width:60px">名次</th>
              <th style="width:70px">泳道</th>
              <th>运动员 / 代表队</th>
              <th style="width:130px">成绩</th>
              <th style="width:100px">成绩来源</th>
              <th>标记 / 说明</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="e in current.board.entries" :key="e.id">
              <td class="rank-cell">{{ e.rank != null ? e.rank : '—' }}</td>
              <td>{{ e.lane_no }}</td>
              <td><b>{{ e.swimmer_name }}</b> <span class="muted">{{ e.team }}</span></td>
              <td class="time-mono num">{{ e.display_time || '—' }}</td>
              <td>
                <span v-if="e.source === 'ELECTRONIC'" class="chip">电子计时</span>
                <span v-else-if="e.source === 'MANUAL'" class="chip missed">手动计时</span>
                <span v-else-if="e.source === 'SWIMOFF'" class="chip swimoff">重赛成绩</span>
                <span v-else class="muted">无</span>
              </td>
              <td>
                <template v-for="f in (e.flags || '').split(',').filter(Boolean)" :key="f">
                  <span class="chip" :class="flagClass(f)">{{ flagText(f) }}</span>
                </template>
                <span class="muted" style="font-size:11px">{{ e.note }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { api } from '../api'

const props = defineProps({
  race: Object,
  user: Object
})

const versions = ref([])
const current = ref(null)
const selectedId = ref(null)
const showPublish = ref(false)
const reason = ref('')
const banner = ref(null)

function flash(text, type = 'ok') {
  banner.value = { text, type }
  setTimeout(() => (banner.value = null), 5000)
}
function fmtTime(t) {
  return (t || '').replace('T', ' ').slice(0, 19)
}
function flagText(f) {
  return { TIE: '并列', SWIMOFF: '重赛', WITHDRAWN: '撤回', MISSED: '漏记' }[f] || f
}
function flagClass(f) {
  return { TIE: 'tie', SWIMOFF: 'swimoff', WITHDRAWN: 'withdrawn', MISSED: 'missed' }[f] || ''
}

async function reload() {
  versions.value = await api.get(`/api/races/${props.race.id}/boards`)
  if (selectedId.value) await openBoard(selectedId.value)
}

async function openBoard(id) {
  selectedId.value = id
  current.value = await api.get(`/api/boards/${id}`)
}

async function publish() {
  if (versions.value.length && !reason.value.trim()) {
    flash('更正发布必须填写更正原因', 'err')
    return
  }
  try {
    const r = await api.post(`/api/races/${props.race.id}/boards`, {
      correction_reason: reason.value
    })
    flash(r.message)
    reason.value = ''
    showPublish.value = false
    await reload()
  } catch (e) {
    flash(e.message, 'err')
  }
}

onMounted(reload)
watch(() => props.race.id, () => {
  selectedId.value = null
  current.value = null
  showPublish.value = false
  reason.value = ''
  reload()
})
</script>

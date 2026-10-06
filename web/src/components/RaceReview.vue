<template>
  <div>
    <div class="panel">
      <div class="row">
        <div>
          <div style="font-size:17px;font-weight:700">
            {{ race.event_name }}
            <span class="chip" v-if="race.round === '重赛'">重赛</span>
          </div>
          <div class="muted">{{ race.code }} · {{ race.distance }}米 {{ race.stroke }} · {{ race.round }}</div>
        </div>
        <span class="spacer"></span>
        <button class="ghost" @click="reload">🔄 刷新</button>
        <button v-if="isClerk" @click="importReadings" :disabled="busy">
          ⬇ 从计时设备导入读数（新批次）
        </button>
        <button v-if="isClerk" class="ghost" @click="showDevice = !showDevice">📡 设备会话</button>
      </div>
      <div v-if="banner" class="banner" :class="banner.type" style="margin-top:10px">{{ banner.text }}</div>

      <div v-if="showDevice" class="panel" style="margin-top:10px;background:#f8fafc">
        <h3>本地模拟计时设备会话</h3>
        <table v-if="sessions.length">
          <thead><tr><th>场次</th><th>泳道数</th><th>漏记泳道</th><th>并列成绩</th><th>已补发</th></tr></thead>
          <tbody>
            <tr v-for="s in sessions" :key="s.race_code">
              <td class="time-mono">{{ s.race_code }}</td>
              <td>{{ s.lanes }}</td>
              <td><span :class="s.missed_lanes ? 'chip missed' : 'chip'">{{ s.missed_lanes }}</span></td>
              <td>
                <span v-for="(cnt, t) in s.ties" :key="t" class="chip tie">{{ t }} ×{{ cnt }}</span>
                <span v-if="!Object.keys(s.ties).length" class="muted">无</span>
              </td>
              <td>{{ s.retransmits }}</td>
            </tr>
          </tbody>
        </table>
        <div class="muted" style="font-size:12px;margin-top:6px">
          设备与成绩库独立：漏记恢复需先让设备补发（泳道行内操作），再重新"导入读数"生成新批次，旧批次读数仍保留。
        </div>
      </div>
    </div>

    <!-- 泳道成绩表 -->
    <div class="panel">
      <h3>泳道原始读数与当前解析</h3>
      <table>
        <thead>
          <tr>
            <th style="width:130px">泳道 / 运动员</th>
            <th style="width:210px">电子原始读数（最新批次）</th>
            <th style="width:120px">裁判手记</th>
            <th style="width:130px">当前成绩 / 名次</th>
            <th>状态 / 复核与裁定</th>
            <th style="width:250px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in detail.lanes" :key="l.entry.id">
            <td>
              <div><b>{{ l.entry.lane_no }} 道</b> · {{ l.entry.swimmer.name }}</div>
              <div class="muted" style="font-size:11px">{{ l.entry.swimmer.team }}</div>
            </td>
            <td>
              <template v-if="l.latest_reading">
                <div class="row" style="gap:6px">
                  <span class="chip" :class="readingChip(l.latest_reading.status)">
                    {{ readingText(l.latest_reading.status) }}
                  </span>
                  <span class="muted" style="font-size:11px">{{ l.latest_reading.device_id }} · 第{{ l.latest_reading.import_batch }}批</span>
                </div>
                <div class="time-mono num" style="margin-top:4px">
                  触壁：{{ l.latest_reading.finish_time != null ? fmt(l.latest_reading.finish_time) : '— 漏记 —' }}
                </div>
                <div class="muted num" style="font-size:11px">分段：{{ (l.latest_reading.splits||[]).map(fmt).join(' / ') || '—' }}</div>
                <button class="ghost tiny" style="margin-top:4px" @click="showRaw(l.entry.id)">
                  {{ rawOpen === l.entry.id ? '收起历史批次' : '查看全部历史批次' }}
                </button>
              </template>
              <span v-else class="muted">尚未导入</span>
            </td>
            <td>
              <div v-if="l.manual_time != null" class="time-mono num">{{ fmt(l.manual_time) }}</div>
              <span v-else class="muted">—</span>
            </td>
            <td>
              <div class="row" style="gap:6px;align-items:center">
                <span class="rank-cell">{{ l.rank != null ? l.rank : '' }}</span>
                <span v-if="l.resolved_seconds != null" class="time-mono num">{{ l.display_time }}</span>
                <span v-else class="muted">无成绩</span>
              </div>
              <div style="margin-top:4px">
                <span v-if="l.source === 'ELECTRONIC'" class="chip">电子</span>
                <span v-else-if="l.source === 'MANUAL'" class="chip missed">手动</span>
                <span v-else-if="l.source === 'SWIMOFF'" class="chip swimoff">重赛</span>
                <span v-for="f in l.flags" :key="f" class="chip" :class="flagClass(f)">{{ flagText(f) }}</span>
              </div>
              <div v-if="l.note" class="muted" style="font-size:11px;margin-top:3px">{{ l.note }}</div>
            </td>
            <td>
              <template v-if="l.entry.status === 'WITHDRAWN'">
                <span class="chip withdrawn">已撤回</span>
                <div class="muted" style="font-size:11px">{{ l.entry.withdraw_reason }}</div>
              </template>
              <div v-for="cs in casesOfLane(l.entry.id)" :key="cs.id" class="case-box" :class="cs.status.toLowerCase()">
                <div>
                  <b>#{{ cs.id }} {{ reasonText(cs.reason_type) }}</b>
                  <span class="chip" :class="cs.status === 'OPEN' ? 'open' : 'closed'">
                    {{ cs.status === 'OPEN' ? '待裁定' : '已裁定' }}
                  </span>
                </div>
                <div>{{ cs.summary }}</div>
                <div v-for="r in cs.rulings" :key="r.id" style="margin-top:3px">
                  <span class="chip" :class="decisionClass(r.decision)">{{ decisionText(r.decision) }}</span>
                  <span class="muted">总裁判 {{ r.decider_name }}：{{ r.rationale }}</span>
                </div>
              </div>
              <span v-if="l.entry.status !== 'WITHDRAWN' && !casesOfLane(l.entry.id).length" class="muted">—</span>
            </td>
            <td>
              <div class="row" style="gap:6px">
                <button class="ghost tiny" @click="openAction(l, 'note')">
                  {{ isClerk ? '补材料' : '手记/材料' }}
                </button>
                <button v-if="isJudge" class="ghost tiny"
                  :disabled="!!openCaseOfLane(l.entry.id) || l.entry.status === 'WITHDRAWN'"
                  @click="openAction(l, l.latest_reading && l.latest_reading.status !== 'OK' ? 'caseMissed' : 'caseDispute')">
                  发起复核
                </button>
                <button v-if="isClerk && l.latest_reading && l.latest_reading.status !== 'OK'"
                  class="ghost tiny" @click="openAction(l, 'retransmit')">设备补发</button>
                <button v-if="isChief && l.entry.status !== 'WITHDRAWN'" class="ghost tiny" @click="openAction(l, 'withdraw')">撤回</button>
                <button v-if="isChief && openCaseOfLane(l.entry.id)" class="tiny" @click="openAction(l, 'ruling')">总裁判裁定</button>
              </div>
            </td>
          </tr>
          <tr v-for="l in detail.lanes" :key="'raw-' + l.entry.id" v-show="rawOpen === l.entry.id">
            <td colspan="6" style="background:#f8fafc">
              <b>泳道 {{ l.entry.lane_no }} 的全部历史原始读数（只读，不覆盖）：</b>
              <table style="margin-top:6px">
                <thead><tr><th>批次</th><th>设备</th><th>状态</th><th>分段</th><th>触壁时间</th><th>记录时间</th></tr></thead>
                <tbody>
                  <tr v-for="r in rawByLane[l.entry.id] || []" :key="r.id">
                    <td>第 {{ r.import_batch }} 批</td>
                    <td>{{ r.device_id }}</td>
                    <td><span class="chip" :class="readingChip(r.status)">{{ readingText(r.status) }}</span></td>
                    <td class="num">{{ (r.splits||[]).map(fmt).join(' / ') }}</td>
                    <td class="num time-mono">{{ r.finish_time != null ? fmt(r.finish_time) : '—' }}</td>
                    <td class="muted">{{ batches[r.import_batch] }}</td>
                  </tr>
                </tbody>
              </table>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 项目级操作：并列复核（裁判） -->
    <div class="panel" v-if="tieLanes.length >= 2">
      <h3>并列情况</h3>
      <div>
        检测到并列：
        <span v-for="l in tieLanes" :key="l.entry.id" class="chip tie">
          {{ l.entry.lane_no }}道 {{ l.entry.swimmer.name }} {{ l.display_time }}
        </span>
        <button v-if="isJudge && !openTieCase" class="tiny" style="margin-left:10px" @click="openTieCaseForm = true">发起并列复核（建议重赛）</button>
        <button v-if="isChief && openTieCase" class="tiny" style="margin-left:10px" @click="tieRuling">总裁判裁定：重赛</button>
        <span v-if="!isJudge && !isChief" class="muted" style="margin-left:10px">由裁判发起、总裁判裁定</span>
      </div>
      <div v-if="openTieCaseForm" class="inline-form">
        <div style="flex:3">
          <label>复核说明</label>
          <input v-model="tieSummary" placeholder="例：5、6 道电子成绩相同，按规程需安排重赛" />
        </div>
        <button @click="submitTieCase" style="margin-top:16px">提交复核</button>
        <button class="ghost" style="margin-top:16px" @click="openTieCaseForm = false">取消</button>
      </div>
    </div>

    <!-- 复核案件总览 -->
    <div class="panel" v-if="detail.cases && detail.cases.length">
      <h3>复核案件与裁定记录</h3>
      <div v-for="cs in detail.cases" :key="cs.id" class="case-box" :class="cs.status.toLowerCase()">
        <div>
          <b>案件 #{{ cs.id }}</b> · {{ reasonText(cs.reason_type) }}
          <span class="chip" :class="cs.status === 'OPEN' ? 'open' : 'closed'">
            {{ cs.status === 'OPEN' ? '待裁定' : '已闭环' }}
          </span>
          <span class="muted">发起人：{{ cs.opener_name }}</span>
        </div>
        <div style="margin:3px 0">{{ cs.summary }}</div>
        <div v-for="r in cs.rulings" :key="r.id">
          <span class="chip" :class="decisionClass(r.decision)">{{ decisionText(r.decision) }}</span>
          <span class="muted">{{ r.decider_name }} 依据：{{ r.rationale }}</span>
        </div>
      </div>
    </div>

    <!-- 弹出操作面板 -->
    <div class="panel" v-if="action" style="border:2px solid var(--brand)">
      <div class="row">
        <h3 style="margin:0">{{ actionTitle }}</h3>
        <span class="spacer"></span>
        <button class="ghost tiny" @click="action = null">✕ 关闭</button>
      </div>

      <template v-if="action.type === 'note'">
        <template v-if="!isClerk">
          <label>手记触壁成绩（秒，仅裁判/总裁判可作为成绩依据）</label>
          <input v-model.number="form.manual_finish" type="number" step="0.01" placeholder="例：55.67" style="max-width:240px" />
        </template>
        <div style="height:8px"></div>
        <label>{{ isClerk ? '补充材料（录像/边道手记等文字说明，不改变名次）' : '手记说明 / 改判依据' }}</label>
        <textarea v-model="form.content" placeholder="例：终点录像显示触壁时刻 32.41；三名边道手记分别为 55.66 / 55.68 / 55.67"></textarea>
        <div class="row" style="margin-top:8px">
          <button @click="submitNote">提交</button>
        </div>
        <div v-for="n in laneNotes" :key="n.id" class="note-box">
          <div class="meta">{{ n.author_name }}（{{ roleText(n.author_role) }}）· {{ fmtTime(n.created_at) }}</div>
          <div>{{ n.content }} <span v-if="n.manual_finish != null" class="chip missed">手记 {{ fmt(n.manual_finish) }}</span></div>
        </div>
      </template>

      <template v-else-if="action.type === 'caseMissed' || action.type === 'caseDispute'">
        <div class="kv">
          <div class="k">复核类型</div>
          <div>{{ action.type === 'caseMissed' ? '计时器漏记' : '成绩争议' }}</div>
        </div>
        <div style="height:8px"></div>
        <label>复核说明</label>
        <textarea v-model="form.summary" :placeholder="action.type === 'caseMissed'
          ? '例：3 道触板无响应，仅有 50 米分段 26.88；边道手记 55.67，录像吻合'
          : '例：电子 1:51.50 与手记 1:50.90 差 0.6 秒，疑似抢按/触板异常'"></textarea>
        <div class="row" style="margin-top:8px">
          <button @click="submitCase(action.type === 'caseMissed' ? 'MISSED_TOUCH' : 'TIME_DISPUTE')">提交复核案件</button>
        </div>
      </template>

      <template v-else-if="action.type === 'retransmit'">
        <div class="muted" style="margin-bottom:8px">
          通知设备启用备用计时模块补发 {{ action.lane.entry.lane_no }} 道触壁报文。补发后须重新"导入读数"。
        </div>
        <label>补发触壁时间（秒）</label>
        <input v-model.number="form.finish_time" type="number" step="0.01" placeholder="例：55.67" style="max-width:240px" />
        <div class="row" style="margin-top:8px"><button @click="submitRetransmit">通知设备补发</button></div>
      </template>

      <template v-else-if="action.type === 'ruling'">
        <div class="kv">
          <div class="k">案件</div>
          <div>#{{ openCaseOfLane(action.lane.entry.id).id }} · {{ reasonText(openCaseOfLane(action.lane.entry.id).reason_type) }}</div>
        </div>
        <div style="height:10px"></div>
        <label>裁定结论</label>
        <div class="row" style="gap:8px">
          <button :class="form.decision === 'USE_ELECTRONIC' ? '' : 'ghost'"
            @click="form.decision = 'USE_ELECTRONIC'">采用电子成绩</button>
          <button :class="form.decision === 'USE_MANUAL' ? '' : 'ghost'"
            @click="form.decision = 'USE_MANUAL'">采用手动成绩</button>
          <button :class="form.decision === 'USE_SWIMOFF' ? '' : 'ghost'"
            @click="form.decision = 'USE_SWIMOFF'">安排重赛</button>
        </div>
        <div v-if="form.decision === 'USE_MANUAL'" style="margin-top:10px">
          <label>手动成绩（留空则采用该泳道最新裁判手记）</label>
          <input v-model.number="form.manual_finish" type="number" step="0.01" :placeholder="action.lane.manual_time != null ? '最新手记 ' + fmt(action.lane.manual_time) : '无手记，需填写'" style="max-width:240px" />
        </div>
        <div style="height:10px"></div>
        <label>改判依据（必填，将随榜单公示）</label>
        <textarea v-model="form.rationale" placeholder="例：录像与三块边道秒表一致，电子触板漏记，依规程 SW11.4 采用手动成绩"></textarea>
        <div class="row" style="margin-top:8px">
          <button @click="submitRuling">确认终局裁定</button>
        </div>
      </template>

      <template v-else-if="action.type === 'withdraw'">
        <label>撤回原因（必填）</label>
        <textarea v-model="form.reason" placeholder="例：赛前点名未到 / 运动员身体原因弃权"></textarea>
        <div class="row" style="margin-top:8px"><button class="danger" @click="submitWithdraw">确认撤回</button></div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '../api'

const props = defineProps({
  race: Object,
  user: Object
})
defineEmits(['changed'])

const detail = ref({ lanes: [], cases: [] })
const rawByLane = ref({})
const batches = ref({})
const sessions = ref([])
const showDevice = ref(false)
const banner = ref(null)
const busy = ref(false)
const rawOpen = ref(null)
const action = ref(null)
const form = ref({})
const openTieCaseForm = ref(false)
const tieSummary = ref('')

const isClerk = computed(() => props.user.role === 'clerk')
const isJudge = computed(() => props.user.role === 'judge' || props.user.role === 'chief')
const isChief = computed(() => props.user.role === 'chief')

const tieLanes = computed(() =>
  (detail.value.lanes || []).filter(l => (l.flags || []).includes('TIE'))
)
const openTieCase = computed(() =>
  (detail.value.cases || []).find(c => c.reason_type === 'TIE_RESOLVE' && c.status === 'OPEN')
)

function flash(text, type = 'ok') {
  banner.value = { text, type }
  setTimeout(() => (banner.value = null), 5000)
}

function fmt(v) {
  return Number(v).toFixed(2)
}
function fmtTime(t) {
  return (t || '').replace('T', ' ').slice(0, 16)
}
function readingText(s) {
  return { OK: '正常', MISSED: '漏记', PARTIAL: '部分分段' }[s] || s
}
function readingChip(s) {
  return s === 'OK' ? '' : 'missed'
}
function flagText(f) {
  return { TIE: '并列', SWIMOFF: '重赛决出名次', WITHDRAWN: '撤回', MISSED: '漏记未决' }[f] || f
}
function flagClass(f) {
  return { TIE: 'tie', SWIMOFF: 'swimoff', WITHDRAWN: 'withdrawn', MISSED: 'missed' }[f] || ''
}
function reasonText(r) {
  return { MISSED_TOUCH: '计时器漏记', TIME_DISPUTE: '成绩争议', TIE_RESOLVE: '并列处理', WITHDRAWAL: '撤回' }[r] || r
}
function decisionText(d) {
  return { USE_ELECTRONIC: '采用电子成绩', USE_MANUAL: '采用手动成绩', USE_SWIMOFF: '安排/采用重赛' }[d] || d
}
function decisionClass(d) {
  return { USE_ELECTRONIC: '', USE_MANUAL: 'missed', USE_SWIMOFF: 'swimoff' }[d] || ''
}
function roleText(r) {
  return { clerk: '录入员', judge: '裁判', chief: '总裁判' }[r] || r
}

function casesOfLane(laneId) {
  return (detail.value.cases || []).filter(c => c.lane_entry_id === laneId)
}
function openCaseOfLane(laneId) {
  return casesOfLane(laneId).find(c => c.status === 'OPEN')
}

const actionTitle = computed(() => {
  if (!action.value) return ''
  const l = action.value.lane
  const who = `${l.entry.lane_no}道 ${l.entry.swimmer.name}`
  return {
    note: `手记 / 补充材料 — ${who}`,
    caseMissed: `发起漏记复核 — ${who}`,
    caseDispute: `发起成绩争议复核 — ${who}`,
    retransmit: `请求设备补发 — ${who}`,
    ruling: `总裁判裁定 — ${who}`,
    withdraw: `撤回 — ${who}`
  }[action.value.type]
})

const laneNotes = computed(() => {
  if (!action.value) return []
  const id = action.value.lane.entry.id
  return (detail.value.notes || []).filter(n => n.lane_entry_id === id).slice().reverse()
})

async function reload() {
  detail.value = await api.get(`/api/races/${props.race.id}`)
  if (showDevice.value || isClerk.value) {
    try { sessions.value = await api.get('/api/device/sessions') } catch { /* ignore */ }
  }
  // 保持已展开的历史批次
  if (rawOpen.value) await loadRaw(rawOpen.value)
}

async function loadRaw(laneId) {
  const r = await api.get(`/api/races/${props.race.id}/readings`)
  batches.value = r.batches
  const by = {}
  for (const x of r.readings) (by[x.lane_entry_id] ||= []).push(x)
  rawByLane.value = by
}

async function showRaw(laneId) {
  if (rawOpen.value === laneId) {
    rawOpen.value = null
    return
  }
  await loadRaw(laneId)
  rawOpen.value = laneId
}

async function importReadings() {
  busy.value = true
  try {
    const r = await api.post(`/api/races/${props.race.id}/readings/import`, {})
    flash(r.message || '导入完成')
    await reload()
  } catch (e) {
    flash(e.message, 'err')
  } finally {
    busy.value = false
  }
}

function openAction(lane, type) {
  action.value = { lane, type }
  form.value = { decision: 'USE_MANUAL' }
}

async function submitNote() {
  try {
    const body = { content: form.value.content || '' }
    if (!isClerk.value && form.value.manual_finish) body.manual_finish = form.value.manual_finish
    if (!body.content && body.manual_finish == null) {
      flash('请填写材料内容或手记成绩', 'err'); return
    }
    await api.post(`/api/races/${props.race.id}/lanes/${action.value.lane.entry.id}/notes`, body)
    flash('已记录' + (isClerk.value ? '补充材料（不改变名次）' : '手记/材料'))
    action.value = null
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

async function submitCase(reason) {
  try {
    await api.post(`/api/races/${props.race.id}/cases`, {
      lane_entry_id: action.value.lane.entry.id,
      reason_type: reason,
      summary: form.value.summary
    })
    flash('复核案件已提交，等待总裁判裁定')
    action.value = null
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

async function submitRetransmit() {
  if (!form.value.finish_time) { flash('请填写补发触壁时间', 'err'); return }
  try {
    const r = await api.post(`/api/device/sessions/${props.race.code}/retransmit`, {
      lane: action.value.lane.entry.lane_no,
      finish_time: form.value.finish_time
    })
    flash(r.message || '设备已补发')
    action.value = null
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

async function submitRuling() {
  const cs = openCaseOfLane(action.value.lane.entry.id)
  try {
    const body = { decision: form.value.decision, rationale: form.value.rationale }
    if (form.value.decision === 'USE_MANUAL' && form.value.manual_finish) {
      body.manual_finish = form.value.manual_finish
    }
    const r = await api.post(`/api/cases/${cs.id}/rulings`, body)
    flash(r.message || '裁定已记录')
    action.value = null
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

async function submitWithdraw() {
  try {
    const r = await api.post(`/api/races/${props.race.id}/lanes/${action.value.lane.entry.id}/withdraw`, {
      reason: form.value.reason
    })
    flash(r.message || '已撤回')
    action.value = null
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

async function submitTieCase() {
  if (!tieSummary.value) { flash('请填写复核说明', 'err'); return }
  try {
    await api.post(`/api/races/${props.race.id}/cases`, {
      lane_entry_id: 0, reason_type: 'TIE_RESOLVE', summary: tieSummary.value
    })
    openTieCaseForm.value = false
    tieSummary.value = ''
    flash('并列复核案件已提交')
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

async function tieRuling() {
  const laneIds = tieLanes.value.map(l => l.entry.id)
  const rationale = `并列成绩 ${tieLanes.value[0].display_time}，依规程安排重赛，以重赛成绩决定名次`
  try {
    const r = await api.post(`/api/cases/${openTieCase.value.id}/rulings`, {
      decision: 'USE_SWIMOFF', rationale, lane_entry_ids: laneIds
    })
    flash(r.message + '；左侧已生成「重赛」项目，导入重赛读数后发布更正榜单')
    await reload()
  } catch (e) { flash(e.message, 'err') }
}

onMounted(reload)
watch(() => props.race.id, () => {
  rawOpen.value = null
  action.value = null
  reload()
})
</script>

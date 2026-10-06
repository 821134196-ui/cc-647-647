<template>
  <div class="card" style="margin-bottom:14px">
    <div class="row" style="justify-content:space-between;align-items:flex-start">
      <div class="row" style="gap:12px;align-items:center">
        <div class="rank-badge" style="min-width:46px;font-size:16px">第{{ detail.lane.lane_no }}道</div>
        <div>
          <strong style="font-size:15px">{{ detail.lane.swimmer_name }}</strong>
          <span class="muted small" style="margin-left:8px">{{ detail.lane.team }}</span>
        </div>
        <span v-if="activeCase" class="tag" :class="activeCase.status">
          {{ activeCase.status === 'open' ? '复核中' : '已裁定' }}
        </span>
      </div>
      <div class="row wrap" style="gap:6px;justify-content:flex-end">
        <button v-if="canContribute" class="sm" @click="showManual = !showManual">手记/材料</button>
        <button v-if="canOpenCase" class="sm" @click="showCaseForm = !showCaseForm">立案复核</button>
        <button v-if="isChief && activeCase?.status === 'decided'" class="sm" @click="reopen">重新开启</button>
        <button v-if="isChief && activeCase?.status === 'open'" class="sm primary" @click="showDecision = !showDecision">
          总裁判裁定
        </button>
      </div>
    </div>

    <!-- 电子原始读数 -->
    <div class="section-title">电子计时原始读数<span class="muted small">（只读，仅追加）</span></div>
    <table>
      <thead><tr><th style="width:90px">节点</th><th style="width:110px">读数</th><th style="width:80px">来源</th><th>设备原始报文</th></tr></thead>
      <tbody>
        <tr v-for="r in detail.splits" :key="r.id">
          <td>{{ r.split_label }}</td>
          <td class="mono">{{ r.time_ms != null ? fmt(r.time_ms) : '漏记' }}</td>
          <td class="muted small">{{ r.source }}</td>
          <td class="mono muted small">{{ r.device_raw }}</td>
        </tr>
        <tr v-for="r in detail.all_touches" :key="r.id" :class="{ 'missed-row': r.missed }">
          <td>{{ r.split_label }}<span class="muted small"> #{{ r.id }}</span></td>
          <td class="mono" :style="r.missed ? 'color:var(--bad);font-weight:600' : ''">
            {{ r.missed ? '漏记 ✕' : fmt(r.time_ms) }}
          </td>
          <td class="muted small">{{ r.source }}</td>
          <td class="mono muted small">{{ r.device_raw }}</td>
        </tr>
      </tbody>
    </table>

    <!-- 手记与补充材料 -->
    <div class="section-title">手记与补充材料<span class="muted small">（任何岗位提交均不改变名次）</span></div>
    <div v-if="!detail.manuals.length" class="muted small">暂无手记。</div>
    <div v-for="m in detail.manuals" :key="m.id" class="card" style="padding:9px 12px;margin-bottom:6px;background:var(--panel-2)">
      <div class="row" style="justify-content:space-between">
        <div class="row" style="gap:8px">
          <strong class="mono">{{ fmt(m.time_ms) }}</strong>
          <span class="tag" :class="m.author_role">{{ roleNames[m.author_role] }} {{ m.author_name }}</span>
          <span class="muted small">{{ m.split_label }}</span>
        </div>
        <span class="muted small">{{ fmtTime(m.created_at) }}</span>
      </div>
      <div class="small" style="margin-top:4px">{{ m.note }}</div>
    </div>

    <!-- 复核案件 -->
    <template v-if="activeCase">
      <div class="section-title">
        复核案件 #{{ activeCase.id }} · {{ activeCase.reason }}
      </div>
      <div class="card small" style="padding:10px 14px;background:var(--panel-2);white-space:pre-wrap">{{ activeCase.description }}</div>

      <div v-if="activeCase.status === 'decided'" class="banner info" style="margin-top:10px">
        <div class="row" style="gap:8px;flex-wrap:wrap">
          <span class="tag" :class="activeCase.decision">{{ decisionNames[activeCase.decision] }}</span>
          <strong v-if="activeCase.decided_time_ms != null" class="mono">
            裁定成绩 {{ fmt(activeCase.decided_time_ms) }}
          </strong>
          <strong v-else>成绩撤回，不参与排名</strong>
          <span class="muted small">总裁判 {{ activeCase.referee_name }} · {{ fmtTime(activeCase.decided_at) }}</span>
        </div>
        <div class="small" v-if="activeCase.decision_note">改判依据：{{ activeCase.decision_note }}</div>
      </div>
    </template>

    <!-- 提交手记 -->
    <div v-if="showManual" class="card" style="margin-top:12px;padding:14px;background:#0c1d29">
      <div class="row wrap" style="gap:10px">
        <div style="width:140px">
          <label>手记成绩</label>
          <input v-model="manualTime" placeholder="如 52.94 或 1:02.34" class="mono" />
        </div>
        <div class="grow">
          <label>情况说明</label>
          <input v-model="manualNote" placeholder="如表计读数、触壁观察、录像情况等" />
        </div>
        <button class="primary" style="margin-top:16px" @click="submitManual">提交手记</button>
      </div>
    </div>

    <!-- 立案 -->
    <div v-if="showCaseForm" class="card" style="margin-top:12px;padding:14px;background:#0c1d29">
      <label>争议类型</label>
      <select v-model="caseReason" style="margin-bottom:10px">
        <option>电子计时漏记</option>
        <option>成绩争议</option>
        <option>并列复核</option>
        <option>犯规核查</option>
        <option>其他</option>
      </select>
      <label>改判依据 / 现场情况</label>
      <textarea v-model="caseDesc" rows="3" placeholder="描述读数异常、观察结果、与手记的差异等"></textarea>
      <div style="margin-top:10px;text-align:right">
        <button class="primary" @click="submitCase">提交立案</button>
      </div>
    </div>

    <!-- 补充材料 -->
    <div v-if="activeCase" class="row" style="margin-top:10px;gap:8px">
      <input v-model="appendText" class="grow" placeholder="向该案件补充材料（录像、旁证、手记说明…）" />
      <button v-if="canContribute" @click="submitAppend">补充</button>
    </div>

    <!-- 总裁判裁定 -->
    <div v-if="showDecision && isChief && activeCase" class="card" style="margin-top:12px;padding:16px;background:#0c1d29;border-color:#a16207">
      <strong style="color:#fde68a">总裁判裁定 · 案件 #{{ activeCase.id }}</strong>
      <div class="row wrap" style="gap:10px;margin-top:10px">
        <label style="width:100%">裁定方式</label>
        <button v-for="d in decisions" :key="d.key"
          :class="['sm', decision === d.key ? 'primary' : '']"
          @click="decision = d.key">
          {{ d.label }}
        </button>
      </div>
      <div v-if="decision === 'manual' || decision === 'swimoff'" style="margin-top:10px;width:220px">
        <label>{{ decision === 'swimoff' ? '重赛成绩（必填）' : '手动成绩（留空取最新手记）' }}</label>
        <input v-model="decisionTime" class="mono" placeholder="如 52.94" />
      </div>
      <div style="margin-top:10px">
        <label>裁定说明 / 改判依据（公示可查）</label>
        <textarea v-model="decisionNote" rows="2" placeholder="如：三表一致，触板故障，采用手记；或重赛51.21有效"></textarea>
      </div>
      <div class="muted small" style="margin-top:8px" v-if="decision === 'electronic'">
        将采用该泳道最新一条有效电子触壁读数。
      </div>
      <div style="margin-top:12px;text-align:right">
        <button class="primary danger" style="border-color:#ca8a04;background:linear-gradient(135deg,#ca8a04,#eab308)"
          @click="submitDecision">确认裁定</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { api, parseTimeInput, ROLE_NAMES, DECISION_NAMES } from '../api'

const props = defineProps({ detail: Object, user: Object })
const emit = defineEmits(['changed', 'toast'])

const roleNames = ROLE_NAMES
const decisionNames = DECISION_NAMES

const showManual = ref(false)
const showCaseForm = ref(false)
const showDecision = ref(false)
const manualTime = ref('')
const manualNote = ref('')
const caseReason = ref('电子计时漏记')
const caseDesc = ref('')
const appendText = ref('')
const decision = ref('manual')
const decisionTime = ref('')
const decisionNote = ref('')

const decisions = [
  { key: 'electronic', label: '采用电子成绩' },
  { key: 'manual', label: '采用手动成绩' },
  { key: 'swimoff', label: '采用重赛结果' },
  { key: 'withdraw', label: '撤回成绩' }
]

const activeCase = computed(() => props.detail.cases[0] || null)
const isChief = computed(() => props.user.role === 'chief')
const canContribute = computed(() => ['judge', 'clerk', 'chief'].includes(props.user.role))
const canOpenCase = computed(() =>
  ['judge', 'chief'].includes(props.user.role) && !activeCase.value
)

function fmt(ms) {
  if (ms == null) return '—'
  const cs = Math.round(ms / 10)
  const h = cs % 100
  const totalSec = Math.floor(cs / 100)
  const s = totalSec % 60
  const m = Math.floor(totalSec / 60)
  const ss = String(s).padStart(2, '0')
  const hh = String(h).padStart(2, '0')
  return m > 0 ? `${m}:${ss}.${hh}` : `${s}.${hh}`
}
function fmtTime(t) { return t ? t.replace('T', ' ').slice(5, 16) : '' }

async function submitManual() {
  const ms = parseTimeInput(manualTime.value)
  if (ms == null) return emit('toast', '成绩格式不正确，示例：52.94 或 1:02.34', 'bad')
  try {
    await api(`/lanes/${props.detail.lane.id}/manual`, {
      method: 'POST', body: { time_ms: ms, note: manualNote.value }
    })
    emit('toast', '手记已作为补充材料提交（不影响名次，待总裁判裁定）')
    manualTime.value = ''; manualNote.value = ''; showManual.value = false
    emit('changed')
  } catch (e) { emit('toast', e.message, 'bad') }
}

async function submitCase() {
  if (!caseDesc.value.trim()) return emit('toast', '请填写复核依据', 'bad')
  try {
    await api(`/events/${props.detail.lane.event_id}/cases`, {
      method: 'POST',
      body: { lane_id: props.detail.lane.id, reason: caseReason.value, description: caseDesc.value }
    })
    emit('toast', '复核案件已提交，等待总裁判裁定')
    caseDesc.value = ''; showCaseForm.value = false
    emit('changed')
  } catch (e) { emit('toast', e.message, 'bad') }
}

async function submitAppend() {
  if (!appendText.value.trim() || !activeCase.value) return
  try {
    await api(`/cases/${activeCase.value.id}/append`, {
      method: 'POST', body: { description: appendText.value }
    })
    appendText.value = ''
    emit('changed')
  } catch (e) { emit('toast', e.message, 'bad') }
}

async function submitDecision() {
  const body = { decision: decision.value, note: decisionNote.value }
  if (decision.value === 'manual' || decision.value === 'swimoff') {
    const ms = parseTimeInput(decisionTime.value)
    if (decision.value === 'swimoff' && ms == null) {
      return emit('toast', '重赛结果必须填写有效成绩', 'bad')
    }
    if (ms != null) body.time_ms = ms
  }
  try {
    await api(`/cases/${activeCase.value.id}/decision`, { method: 'POST', body })
    emit('toast', `裁定已记录：${DECISION_NAMES[decision.value]}`)
    showDecision.value = false
    decisionTime.value = ''; decisionNote.value = ''
    emit('changed')
  } catch (e) { emit('toast', e.message, 'bad') }
}

async function reopen() {
  try {
    await api(`/cases/${activeCase.value.id}/reopen`, { method: 'POST' })
    emit('toast', '案件已重新开启复核')
    emit('changed')
  } catch (e) { emit('toast', e.message, 'bad') }
}
</script>

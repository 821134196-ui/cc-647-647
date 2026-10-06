<template>
  <LoginView v-if="!user" @login="onLogin" />
  <div v-else class="layout">
    <aside class="sidebar">
      <div class="login-hero" style="margin-bottom:14px">
        <img src="/favicon.svg" class="logo" alt="logo" />
        <div>
          <div style="font-weight:700">泳道计时复核</div>
          <div class="muted small">电子计时 · 复核 · 公示</div>
        </div>
      </div>

      <div v-for="g in meets" :key="g.meet.id" style="margin-bottom:14px">
        <div class="muted small" style="padding:0 4px 6px">
          {{ g.meet.name }} · {{ g.meet.date }}
        </div>
        <div
          v-for="e in g.events" :key="e.id"
          class="event-item" :class="{ active: eventId === e.id }"
          @click="selectEvent(e.id)"
        >
          <div>{{ e.name }}</div>
          <div class="muted small">{{ e.distance }}m {{ e.stroke }} · {{ e.round }}</div>
        </div>
      </div>

      <button v-if="isChief" class="sm ghost" style="margin-top:8px" @click="showCreateEvent = true">
        + 新建比赛项目
      </button>
    </aside>

    <main class="main">
      <div class="row" style="justify-content:space-between;margin-bottom:16px">
        <div>
          <div class="muted small">当前岗位</div>
          <div class="row" style="gap:8px">
            <strong>{{ user.name }}</strong>
            <span class="tag" :class="user.role">{{ roleNames[user.role] }}</span>
          </div>
        </div>
        <div class="row wrap" style="gap:8px;justify-content:flex-end">
          <button v-if="isChief" class="sm" @click="showDevice = !showDevice">
            {{ showDevice ? '返回复核台' : '设备模拟台' }}
          </button>
          <button v-if="isDevice" class="sm" @click="showDevice = true">设备模拟台</button>
          <button class="sm ghost" @click="logout">退出登录</button>
        </div>
      </div>

      <DevicePanel v-if="showDevice && (isChief || isDevice)" :event-id="eventId" @changed="reload" />
      <EventView
        v-else-if="eventId" :key="eventId" :event-id="eventId" :user="user"
        @toast="toast"
      />
      <div v-else class="card muted">请从左侧选择一个比赛项目</div>
    </main>

    <CreateEventDialog
      v-if="showCreateEvent"
      :meet-id="meets[0]?.meet.id"
      @close="showCreateEvent = false" @created="onEventCreated"
    />
  </div>

  <div v-if="toastMsg" class="toast" :class="toastKind">{{ toastMsg }}</div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, getStoredUser, clearAuth, ROLE_NAMES } from './api'
import LoginView from './components/LoginView.vue'
import EventView from './components/EventView.vue'
import DevicePanel from './components/DevicePanel.vue'
import CreateEventDialog from './components/CreateEventDialog.vue'

const user = ref(getStoredUser())
const meets = ref([])
const eventId = ref(null)
const showDevice = ref(false)
const showCreateEvent = ref(false)
const toastMsg = ref('')
const toastKind = ref('ok')

const isChief = computed(() => user.value?.role === 'chief')
const isDevice = computed(() => user.value?.role === 'device')
const roleNames = ROLE_NAMES

function toast(msg, kind = 'ok') {
  toastMsg.value = msg
  toastKind.value = kind
  setTimeout(() => (toastMsg.value = ''), 3200)
}

function onLogin(u) {
  user.value = u
  showDevice.value = u.role === 'device'
  loadMeets()
}

function logout() {
  clearAuth()
  user.value = null
}

async function loadMeets() {
  meets.value = await api('/meets')
  if (!eventId.value && meets.value[0]?.events?.length) {
    eventId.value = meets.value[0].events[0].id
  }
}

function selectEvent(id) {
  eventId.value = id
  showDevice.value = false
}

function onEventCreated(ev) {
  showCreateEvent.value = false
  loadMeets().then(() => selectEvent(ev.id))
  toast('项目已创建')
}

async function reload() {
  await loadMeets()
}

onMounted(() => {
  if (user.value) loadMeets()
})
</script>

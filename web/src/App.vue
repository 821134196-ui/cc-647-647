<template>
  <div v-if="!user" class="login-wrap">
    <LoginView @logged="onLogged" />
  </div>
  <template v-else>
    <div class="topbar">
      <span class="title">🏊 游泳赛事泳道电子计时复核系统</span>
      <span class="spacer"></span>
      <span class="who">{{ user.name }} · {{ roleLabel[user.role] }}</span>
      <button class="tiny" @click="logout">退出</button>
    </div>
    <div class="layout">
      <div class="sidebar">
        <template v-for="g in meets" :key="g.meet.id">
          <div class="meet-name">{{ g.meet.name }}</div>
          <div class="muted" style="font-size:11px;margin-bottom:8px">{{ g.meet.venue }} · {{ g.meet.date }}</div>
          <button
            v-for="r in g.races" :key="r.id"
            class="race-item" :class="{ active: raceId === r.id }"
            @click="selectRace(r)"
          >
            <div>{{ r.event_name }} <span v-if="r.round === '重赛'" class="chip swimoff">重赛</span></div>
            <div class="code">{{ r.code }} · {{ r.round }}{{ r.has_readings ? ' · 已导入读数' : '' }}</div>
          </button>
        </template>
      </div>
      <div class="main" v-if="currentRace">
        <div class="tabs">
          <button :class="{ active: tab === 'review' }" @click="tab = 'review'">成绩复核</button>
          <button :class="{ active: tab === 'board' }" @click="tab = 'board'">榜单公示</button>
        </div>
        <RaceReview v-if="tab === 'review'" :race="currentRace" :user="user" @changed="loadMeets" />
        <BoardView v-else :race="currentRace" :user="user" @changed="loadMeets" />
      </div>
      <div class="main" v-else>
        <div class="panel muted">请从左侧选择一个比赛项目。</div>
      </div>
    </div>
  </template>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api, getUser, clearSession, ROLE_LABEL } from './api'
import LoginView from './components/LoginView.vue'
import RaceReview from './components/RaceReview.vue'
import BoardView from './components/BoardView.vue'

const user = ref(getUser())
const meets = ref([])
const raceId = ref(null)
const currentRace = ref(null)
const tab = ref('review')
const roleLabel = ROLE_LABEL

async function loadMeets() {
  meets.value = await api.get('/api/meets')
}

function selectRace(r) {
  raceId.value = r.id
  currentRace.value = r
}

function onLogged(u) {
  user.value = u
  loadMeets()
}

function logout() {
  clearSession()
  user.value = null
  currentRace.value = null
}

onMounted(() => {
  if (user.value) loadMeets()
})
</script>

<template>
  <div class="login-wrap">
    <div class="card login-card">
      <div class="login-hero">
        <img src="/favicon.svg" class="logo" alt="logo" />
        <div>
          <h2 style="margin:0">游泳赛事泳道电子计时复核系统</h2>
          <div class="muted small">计时漏记复核 · 成绩争议裁定 · 榜单公示更正</div>
        </div>
      </div>
      <form @submit.prevent="login">
        <div style="margin:14px 0 10px">
          <label>账号</label>
          <input v-model="username" placeholder="如 chief / judge / clerk / device" autofocus />
        </div>
        <div style="margin-bottom:14px">
          <label>密码</label>
          <input v-model="password" type="password" placeholder="演示环境统一为 123456" />
        </div>
        <button class="primary" style="width:100%" :disabled="loading">
          {{ loading ? '登录中…' : '登录' }}
        </button>
        <div v-if="error" style="color:var(--bad);margin-top:10px" class="small">{{ error }}</div>
      </form>
      <div class="muted small" style="margin-top:14px">演示账号（点击快速填充）：</div>
      <div class="quick-users">
        <button v-for="u in demoUsers" :key="u.username" type="button"
          @click="username = u.username; password = '123456'; error = ''">
          {{ u.label }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api, setAuth } from '../api'

const emit = defineEmits(['login'])
const username = ref('chief')
const password = ref('123456')
const error = ref('')
const loading = ref(false)

const demoUsers = [
  { username: 'chief', label: '总裁判 周总裁' },
  { username: 'judge', label: '裁判 吴裁判' },
  { username: 'clerk', label: '录入员 王录入' },
  { username: 'device', label: '电子计时台' }
]

async function login() {
  error.value = ''
  loading.value = true
  try {
    const res = await api('/auth/login', {
      method: 'POST',
      body: { username: username.value, password: password.value }
    })
    setAuth(res.token, res.user)
    emit('login', res.user)
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

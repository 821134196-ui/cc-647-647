<template>
  <div class="login-card">
    <h1>泳道电子计时复核系统</h1>
    <div class="sub">漏记复核 · 手动/电子成绩裁定 · 榜单版本公示</div>
    <div class="field">
      <label>账号</label>
      <input v-model="username" placeholder="clerk / judge / chief" @keyup.enter="login" />
    </div>
    <div class="field">
      <label>密码</label>
      <input v-model="password" type="password" placeholder="默认 clerk123 / judge123 / chief123" @keyup.enter="login" />
    </div>
    <div class="role-quick">
      <button class="ghost tiny" @click="fill('clerk','clerk123')">录入员</button>
      <button class="ghost tiny" @click="fill('judge','judge123')">裁判</button>
      <button class="ghost tiny" @click="fill('chief','chief123')">总裁判</button>
    </div>
    <button style="width:100%" @click="login" :disabled="loading">
      {{ loading ? '登录中…' : '登 录' }}
    </button>
    <div class="err-msg">{{ err }}</div>
    <div class="sub" style="margin:10px 0 0">岗位权限：录入员只能补充材料；裁判可提交手记、发起复核；总裁判终局裁定并发布榜单。</div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api, setSession } from '../api'

const emit = defineEmits(['logged'])
const username = ref('')
const password = ref('')
const err = ref('')
const loading = ref(false)

function fill(u, p) {
  username.value = u
  password.value = p
  err.value = ''
}

async function login() {
  err.value = ''
  loading.value = true
  try {
    const r = await api.post('/api/auth/login', { username: username.value, password: password.value })
    setSession(r.token, r.user)
    emit('logged', r.user)
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

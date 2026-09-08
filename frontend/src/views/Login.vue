<!--
  Login.vue — 登录页面。

  功能：
    - 用户名 + 密码表单登录
    - 登录成功后保存 Token 对（access + refresh）和用户名
    - 支持 redirect 参数：登录后跳转到之前访问的页面
    - 回车键快捷登录（@keyup.enter）

  默认账号：admin / 123456
-->
<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import { saveAuth } from '@/utils/auth'

const router = useRouter()
const route = useRoute()

/** 登录按钮加载状态 */
const loading = ref(false)

/** 登录表单数据 */
const form = reactive({
  username: '',
  password: '',
})

/** 执行登录 */
async function handleLogin() {
  if (!form.username || !form.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }

  loading.value = true
  try {
    const res = await axios.post('/api/login', {
      username: form.username,
      password: form.password,
    })

    if (res.data.code === 0) {
      const { access_token, refresh_token, username } = res.data.data
      // 保存认证信息到 localStorage
      saveAuth(access_token, refresh_token, username)
      ElMessage.success('登录成功')

      // 跳转到 redirect 参数指定的页面或默认首页
      const redirect = (route.query.redirect as string) || '/'
      router.push(redirect)
    } else {
      ElMessage.error(res.data.msg || '登录失败')
    }
  } catch {
    ElMessage.error('登录请求失败，请检查网络')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-container">
    <div class="login-card">
      <h2 class="login-title">KeystoneGo</h2>
      <p class="login-subtitle">通用网站管理后台</p>

      <el-form @submit.prevent="handleLogin">
        <el-form-item>
          <el-input
            v-model="form.username"
            placeholder="用户名"
            size="large"
            prefix-icon="User"
          />
        </el-form-item>

        <el-form-item>
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            size="large"
            prefix-icon="Lock"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            block
            @click="handleLogin"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>

      <p class="login-hint">默认账号: admin / 123456</p>
    </div>
  </div>
</template>

<style scoped>
/* 登录容器：全屏居中 + 渐变背景 */
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: linear-gradient(135deg, #1e3c72 0%, #2a5298 50%, #1e3c72 100%);
}

/* 登录卡片：白色圆角 + 阴影 */
.login-card {
  width: 400px;
  padding: 40px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
}

.login-title {
  text-align: center;
  font-size: 28px;
  color: #303133;
  margin-bottom: 8px;
}

.login-subtitle {
  text-align: center;
  color: #909399;
  margin-bottom: 32px;
  font-size: 14px;
}

.login-hint {
  text-align: center;
  color: #c0c4cc;
  font-size: 12px;
  margin-top: 16px;
}
</style>

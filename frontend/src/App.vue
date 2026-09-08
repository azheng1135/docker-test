<!--
  App.vue — KeystoneGo 根组件。

  布局逻辑：
    - 登录页（/login）：全屏渲染，不显示侧边栏和头部
    - 其他页面：左侧可折叠侧边栏 + 右侧（头部导航 + 主内容区）

  头部功能：
    - 侧边栏折叠/展开切换
    - 刷新页面按钮
    - 用户下拉菜单（个人中心 + 退出登录）

  退出登录流程：确认对话框 → 调用 /auth/logout → 清除 Token → 跳转登录页
-->
<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { Fold, Expand, Refresh, ArrowDown, User, SwitchButton } from '@element-plus/icons-vue'
import Sidebar from './components/Sidebar.vue'
import { removeToken, getToken, getUsername } from './utils/auth'
import request from './utils/request'

const route = useRoute()
const router = useRouter()

/** 侧边栏是否折叠 */
const isCollapse = ref(false)
/** 当前登录用户名 */
const username = ref('')

/** 显示名称：优先使用昵称，fallback 到 "管理员" */
const displayName = computed(() => username.value || '管理员')

onMounted(async () => {
  if (getToken()) {
    // 优先使用本地缓存的用户名，减少首屏闪烁
    username.value = getUsername() || ''
    try {
      const res = await request.get('/user/info')
      username.value = res.data?.user?.nickname || res.data?.user?.username || username.value
    } catch { /* ignore */ }
  }
})

/** 刷新当前页面（硬刷新） */
function handleRefresh() {
  window.location.reload()
}

/** 退出登录：确认 → 调登出接口 → 清 Token → 跳转 */
async function handleLogout() {
  ElMessageBox.confirm('确定要退出登录吗？', '提示', { type: 'warning' }).then(async () => {
    try {
      await request.post('/auth/logout')
    } catch { /* ignore */ }
    removeToken()
    router.push('/login')
  }).catch(() => {})
}
</script>

<template>
  <!-- 登录页：全屏渲染，无布局框架 -->
  <div v-if="route.path === '/login'" class="app-container">
    <router-view />
  </div>

  <!-- 主布局：侧边栏 + 头部 + 内容区 -->
  <el-container v-else class="layout-container">
    <!-- 左侧可折叠侧边栏 -->
    <el-aside :width="isCollapse ? '64px' : '220px'" class="aside">
      <div class="logo">
        <span v-show="!isCollapse">KeystoneGo</span>
        <span v-show="isCollapse">KG</span>
      </div>
      <Sidebar />
    </el-aside>

    <el-container class="main-container">
      <!-- 顶部导航栏 -->
      <el-header class="header">
        <div class="header-left">
          <el-icon class="collapse-btn" @click="isCollapse = !isCollapse">
            <Fold v-if="!isCollapse" />
            <Expand v-else />
          </el-icon>
        </div>
        <div class="header-right">
          <!-- 刷新按钮 -->
          <el-tooltip content="刷新页面" placement="bottom">
            <el-icon class="action-icon" @click="handleRefresh"><Refresh /></el-icon>
          </el-tooltip>
          <!-- 用户信息下拉菜单 -->
          <el-dropdown>
            <span class="user-info">
              {{ displayName }}
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item>
                  <el-icon><User /></el-icon>个人中心
                </el-dropdown-item>
                <el-dropdown-item divided @click="handleLogout">
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <!-- 主内容区（根据路由动态渲染） -->
      <el-main class="main-content">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style>
/* 全局重置 */
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body, #app { height: 100%; width: 100%; }

/* 主布局：固定全屏，防止滚动条 */
.layout-container {
  height: 100vh;
  width: 100vw;
  position: fixed;
  top: 0;
  left: 0;
  overflow: hidden;
}

/* 侧边栏：深色背景 + 展开/折叠过渡动画 */
.aside {
  height: 100%;
  background-color: #304156;
  transition: width 0.3s;
  overflow: hidden;
}

/* Logo 区域 */
.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 18px;
  font-weight: bold;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  flex-shrink: 0;
}

/* 右侧主区域：垂直布局 */
.main-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 顶部导航栏 */
.header {
  background-color: #fff;
  border-bottom: 1px solid #dcdfe6;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 60px;
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
}

.collapse-btn {
  font-size: 20px;
  cursor: pointer;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 操作图标按钮 */
.action-icon {
  font-size: 18px;
  cursor: pointer;
  padding: 8px;
  border-radius: 4px;
  transition: all 0.3s;
  color: #606266;
}

.action-icon:hover {
  background-color: #f5f7fa;
  color: #409eff;
}

/* 用户信息区域 */
.user-info {
  display: flex;
  align-items: center;
  cursor: pointer;
  padding: 8px 12px;
  border-radius: 4px;
  transition: all 0.3s;
  color: #606266;
  gap: 4px;
}

.user-info:hover {
  background-color: #f5f7fa;
  color: #409eff;
}

/* 主内容区：浅灰背景 + 滚动 */
.main-content {
  flex: 1;
  padding: 20px;
  overflow-y: auto;
  background-color: #f0f2f5;
}
</style>

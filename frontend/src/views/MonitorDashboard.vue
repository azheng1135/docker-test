<!--
  MonitorDashboard.vue — 系统监控面板。

  功能：
    - 健康检查卡片：显示 /healthz（存活探针）和 /readyz（就绪探针）状态
    - 运行时指标卡片：运行时间、Goroutines 数量、内存使用、HTTP 错误数
    - 系统信息卡片：总请求数、CPU 核心数、P99 延迟、Go 版本
    - HTTP 请求统计表：按方法+路径+状态码分组展示请求次数

  数据刷新策略：每 5 秒自动轮询一次（setInterval），组件卸载时清理定时器。
-->
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { getMonitorStats, type MonitorStats } from '@/api/monitor'
import { Monitor, Cpu, MagicStick, Timer, Coin, Warning, CircleCheck, CircleClose } from '@element-plus/icons-vue'

const stats = ref<MonitorStats | null>(null)
const loading = ref(false)

/** 健康检查状态 */
const healthzStatus = ref<'loading' | 'ok' | 'fail'>('loading')
/** 就绪检查状态 */
const readyzStatus = ref<'loading' | 'ok' | 'fail'>('loading')

let timer: ReturnType<typeof setInterval> | null = null

/** 检查 /healthz 存活探针 */
async function checkHealth() {
  try {
    const res = await fetch('/healthz')
    healthzStatus.value = res.ok ? 'ok' : 'fail'
  } catch {
    healthzStatus.value = 'fail'
  }
}

/** 检查 /readyz 就绪探针（验证 DB 连接） */
async function checkReady() {
  try {
    const res = await fetch('/readyz')
    readyzStatus.value = res.ok ? 'ok' : 'fail'
  } catch {
    readyzStatus.value = 'fail'
  }
}

/** 格式化运行时长：Xd Xh Xm Xs */
function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m ${s}s`
  return `${m}m ${s}s`
}

/** 格式化内存：自动选择 MB 或 GB */
function formatMemory(mb: number): string {
  if (mb >= 1024) return (mb / 1024).toFixed(2) + ' GB'
  return mb.toFixed(1) + ' MB'
}

/** HTTP 状态码 → Tag 颜色 */
function getStatusType(status: string): 'success' | 'danger' | 'warning' | 'info' {
  const code = parseInt(status)
  if (code >= 500) return 'danger'
  if (code >= 400) return 'warning'
  if (code >= 200 && code < 300) return 'success'
  return 'info'
}

/** HTTP 方法 → Tag 颜色 */
function getMethodType(method: string): 'success' | 'danger' | 'warning' | 'info' | '' {
  switch (method) {
    case 'GET': return 'success'
    case 'POST': return ''
    case 'PUT': return 'warning'
    case 'DELETE': return 'danger'
    default: return 'info'
  }
}

/** 获取监控统计数据 */
async function fetchStats() {
  loading.value = true
  try {
    const res = await getMonitorStats()
    stats.value = res.data
  } catch {
    // ignore
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchStats()
  checkHealth()
  checkReady()
  // 每 5 秒刷新
  timer = setInterval(() => {
    fetchStats()
    checkHealth()
    checkReady()
  }, 5000)
})

// 组件卸载时清理定时器
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="monitor-dashboard">
    <h3 style="margin-bottom: 20px;">系统监控面板</h3>

    <!-- 健康检查卡片 -->
    <el-row :gutter="16" style="margin-bottom: 16px;">
      <el-col :span="6">
        <el-card shadow="hover">
          <a href="/healthz" target="_blank" style="text-decoration: none; color: inherit;">
            <div class="stat-card">
              <el-icon :size="32">
                <CircleCheck v-if="healthzStatus === 'ok'" color="#67c23a" />
                <CircleClose v-else-if="healthzStatus === 'fail'" color="#f56c6c" />
                <Timer v-else color="#909399" />
              </el-icon>
              <div class="stat-info">
                <div class="stat-label">Health Check</div>
                <div class="stat-value" style="font-size: 16px;">
                  <code style="background:#f0f2f5;padding:2px 8px;border-radius:4px;">/healthz</code>
                </div>
                <el-tag :type="healthzStatus === 'ok' ? 'success' : healthzStatus === 'fail' ? 'danger' : 'info'" size="small" style="margin-top: 4px;">
                  {{ healthzStatus === 'ok' ? '正常' : healthzStatus === 'fail' ? '异常' : '检测中' }}
                </el-tag>
              </div>
            </div>
          </a>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <a href="/readyz" target="_blank" style="text-decoration: none; color: inherit;">
            <div class="stat-card">
              <el-icon :size="32">
                <CircleCheck v-if="readyzStatus === 'ok'" color="#67c23a" />
                <CircleClose v-else-if="readyzStatus === 'fail'" color="#f56c6c" />
                <Timer v-else color="#909399" />
              </el-icon>
              <div class="stat-info">
                <div class="stat-label">Ready Check</div>
                <div class="stat-value" style="font-size: 16px;">
                  <code style="background:#f0f2f5;padding:2px 8px;border-radius:4px;">/readyz</code>
                </div>
                <el-tag :type="readyzStatus === 'ok' ? 'success' : readyzStatus === 'fail' ? 'danger' : 'info'" size="small" style="margin-top: 4px;">
                  {{ readyzStatus === 'ok' ? '正常' : readyzStatus === 'fail' ? '异常' : '检测中' }}
                </el-tag>
              </div>
            </div>
          </a>
        </el-card>
      </el-col>
    </el-row>

    <!-- 运行时指标卡片 -->
    <el-row :gutter="16" v-loading="loading">
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <el-icon :size="32" color="#409eff"><Timer /></el-icon>
            <div class="stat-info">
              <div class="stat-label">运行时间</div>
              <div class="stat-value">{{ stats ? formatUptime(stats.uptime_seconds) : '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <el-icon :size="32" color="#67c23a"><Cpu /></el-icon>
            <div class="stat-info">
              <div class="stat-label">Goroutines</div>
              <div class="stat-value">{{ stats?.num_goroutines ?? '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <el-icon :size="32" color="#e6a23c"><MagicStick /></el-icon>
            <div class="stat-info">
              <div class="stat-label">内存使用</div>
              <div class="stat-value">{{ stats ? formatMemory(stats.memory_alloc_mb) : '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <el-icon :size="32" color="#f56c6c"><Warning /></el-icon>
            <div class="stat-info">
              <div class="stat-label">HTTP 错误</div>
              <div class="stat-value">{{ stats?.http_errors ?? '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 系统信息卡片 -->
    <el-row :gutter="16" style="margin-top: 16px;">
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <el-icon :size="32" color="#909399"><Coin /></el-icon>
            <div class="stat-info">
              <div class="stat-label">总请求数</div>
              <div class="stat-value">{{ stats?.http_total ?? '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <el-icon :size="32" color="#909399"><Monitor /></el-icon>
            <div class="stat-info">
              <div class="stat-label">CPU 核心</div>
              <div class="stat-value">{{ stats?.num_cpu ?? '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <div class="stat-info" style="margin-left: 0;">
              <div class="stat-label">P99 延迟</div>
              <div class="stat-value">{{ stats ? (stats.http_p99_seconds * 1000).toFixed(1) + ' ms' : '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <div class="stat-info" style="margin-left: 0;">
              <div class="stat-label">Go 版本</div>
              <div class="stat-value" style="font-size: 14px;">{{ stats?.go_version ?? '--' }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- HTTP 请求统计表 -->
    <el-card style="margin-top: 16px;">
      <template #header>
        <span>HTTP 请求统计</span>
      </template>
      <el-table :data="stats?.http_metrics || []" border size="small" max-height="400" empty-text="暂无请求数据">
        <el-table-column prop="method" label="方法" width="80">
          <template #default="{ row }">
            <el-tag :type="getMethodType(row.method)" size="small">{{ row.method }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路径" min-width="200" />
        <el-table-column prop="status" label="状态码" width="90">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="count" label="请求数" width="100" sortable />
      </el-table>
    </el-card>
  </div>
</template>

<style scoped>
.monitor-dashboard h3 {
  font-weight: 600;
  color: #303133;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-label {
  font-size: 13px;
  color: #909399;
  margin-bottom: 4px;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
}
</style>

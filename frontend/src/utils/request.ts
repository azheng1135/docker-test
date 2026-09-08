/**
 * Axios 请求封装 — 统一拦截器 + Token 自动刷新。
 *
 * 核心机制：
 *  1. 请求拦截器 — 自动注入 Bearer Token
 *  2. 响应拦截器 — 统一处理 code !== 0 的错误
 *  3. Token 无感刷新 — 当返回 20000/20001（未登录/过期）时：
 *     a. 第一个请求触发 refreshAccessToken()
 *     b. 并发请求加入 refreshSubscribers 队列等待
 *     c. 刷新成功后重新执行所有等待的请求
 *     d. 刷新失败则清除 Token 并跳转登录页
 *
 * 使用方式：
 *   import request from '@/utils/request'
 *   const res = await request.get('/users', { params: { page: 1 } })
 */

import axios from 'axios'
import { ElMessage } from 'element-plus'
import { getToken, removeToken, getRefreshToken, saveAuth } from './auth'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

/** 是否正在刷新 Token（防止并发刷新） */
let isRefreshing = false
/** 等待新 Token 的请求队列 */
let refreshSubscribers: ((token: string) => void)[] = []

/** Token 刷新完成后，通知所有等待的请求 */
function onTokenRefreshed(token: string) {
  refreshSubscribers.forEach((cb) => cb(token))
  refreshSubscribers = []
}

/** 将请求加入等待队列 */
function addRefreshSubscriber(cb: (token: string) => void) {
  refreshSubscribers.push(cb)
}

// ============ 请求拦截器 ============

request.interceptors.request.use(
  (config) => {
    // 自动注入 Authorization Header
    const token = getToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  },
)

// ============ 响应拦截器 ============

request.interceptors.response.use(
  (response) => {
    const res = response.data
    const traceId = res.trace_id || response.headers['x-trace-id'] || ''

    if (res.code !== 0) {
      // 未登录(20000) 或 Token 过期(20001) → 尝试无感刷新
      if (res.code === 20000 || res.code === 20001) {
        const refreshToken = getRefreshToken()
        if (refreshToken && !isRefreshing) {
          return refreshAccessToken().then((newToken) => {
            // 重试原请求
            if (response.config.headers) {
              response.config.headers.Authorization = `Bearer ${newToken}`
            }
            return request(response.config)
          }).catch(() => {
            removeToken()
            redirectToLogin()
            return Promise.reject(new Error(res.msg || '登录已过期'))
          })
        }

        // 正在刷新中，将当前请求加入等待队列
        if (isRefreshing) {
          return new Promise((resolve) => {
            addRefreshSubscriber((token: string) => {
              if (response.config.headers) {
                response.config.headers.Authorization = `Bearer ${token}`
              }
              resolve(request(response.config))
            })
          })
        }

        // 无 Refresh Token 可用，直接跳转登录
        removeToken()
        redirectToLogin()
        return Promise.reject(new Error(res.msg || '登录已过期'))
      }

      // 其他业务错误：弹出错误提示
      ElMessage.error(res.msg || '请求失败')
      console.error(`[${traceId}] API Error:`, res.code, res.msg)
      return Promise.reject(new Error(res.msg || '请求失败'))
    }

    return res
  },
  (error) => {
    // HTTP 401 → 清除 Token 跳转登录
    if (error.response?.status === 401) {
      removeToken()
      redirectToLogin()
      return Promise.reject(error)
    }
    ElMessage.error(error.message || '网络异常')
    return Promise.reject(error)
  },
)

// ============ Token 刷新逻辑 ============

/**
 * 使用 Refresh Token 获取新的 Access Token。
 * 通过 isRefreshing 锁防止并发刷新。
 */
async function refreshAccessToken(): Promise<string> {
  isRefreshing = true
  try {
    const refreshToken = getRefreshToken()
    const res = await axios.post('/api/v1/auth/refresh', {
      refresh_token: refreshToken,
    })

    if (res.data.code === 0) {
      const { access_token, refresh_token } = res.data.data
      const username = localStorage.getItem('username') || ''
      // 保存新的 Token 对
      saveAuth(access_token, refresh_token, username)
      // 通知所有等待的请求
      onTokenRefreshed(access_token)
      return access_token
    }
    throw new Error('刷新失败')
  } catch {
    removeToken()
    throw new Error('刷新失败')
  } finally {
    isRefreshing = false
  }
}

/** 跳转到登录页（若非登录页），携带当前路径作为 redirect 参数 */
function redirectToLogin() {
  const currentPath = window.location.pathname
  if (currentPath !== '/login') {
    window.location.href = `/login?redirect=${encodeURIComponent(currentPath)}`
  }
}

export default request

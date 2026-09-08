<!--
  Sidebar.vue — 可折叠侧边栏组件。

  功能：
    - 从后端 API 获取菜单树并渲染为 el-menu
    - 支持多级菜单（目录 M + 子菜单 C 的嵌套结构）
    - 菜单类型为 'M' 且有子节点 → el-sub-menu（可展开）
    - 菜单类型为 'C' 或无子节点 → el-menu-item（直接跳转）
    - API 请求失败时降级使用硬编码的默认菜单结构
    - 图标映射：通过 iconMap 将菜单 icon 字段映射到 Element Plus 图标组件
-->
<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { getMenus, type MenuItem } from '@/api/menu'
import { HomeFilled, Setting, User, Avatar, Menu, Key, Monitor } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

/** 从 API 获取的菜单树数据 */
const menus = ref<MenuItem[]>([])
/** 当前激活的菜单项（高亮） */
const activeMenu = ref(route.path)

// 路由变化时同步更新激活菜单
watch(() => route.path, (path) => { activeMenu.value = path })

/** 菜单 icon 字段 → Element Plus 图标组件的映射表 */
const iconMap: Record<string, any> = {
  HomeFilled, Setting, User, UserFilled: User, Avatar, Menu, Key, Monitor,
}

/** 根据 icon 名称获取对应的 Element Plus 图标组件 */
function getIcon(name?: string) {
  return name ? iconMap[name] || null : null
}

onMounted(async () => {
  try {
    const res = await getMenus()
    menus.value = res.data || []
  } catch {
    // API 不可用时降级使用默认菜单结构
    menus.value = [
      {
        id: 1, parent_id: 0, name: '首页', path: '/dashboard', icon: 'HomeFilled',
        sort: 1, type: 'C', status: 1, hidden: false,
      },
      {
        id: 2, parent_id: 0, name: '系统管理', path: '/system', icon: 'Setting',
        sort: 2, type: 'M', status: 1, hidden: false,
        children: [
          { id: 3, parent_id: 2, name: '用户管理', path: '/system/users', icon: 'User', sort: 1, type: 'C', status: 1, hidden: false },
          { id: 4, parent_id: 2, name: '角色管理', path: '/system/roles', icon: 'Avatar', sort: 2, type: 'C', status: 1, hidden: false },
          { id: 5, parent_id: 2, name: '菜单管理', path: '/system/menus', icon: 'Menu', sort: 3, type: 'C', status: 1, hidden: false },
          { id: 6, parent_id: 2, name: '权限管理', path: '/system/permissions', icon: 'Key', sort: 4, type: 'C', status: 1, hidden: false },
          { id: 7, parent_id: 2, name: '系统监控', path: '/system/monitor', icon: 'Monitor', sort: 5, type: 'C', status: 1, hidden: false },
        ],
      },
    ]
  }
})

/** 菜单项点击 → 路由跳转 */
function handleSelect(index: string) {
  router.push(index)
}
</script>

<template>
  <el-menu
    :default-active="activeMenu"
    background-color="#304156"
    text-color="#bfcbd9"
    active-text-color="#409EFF"
    router
    @select="handleSelect"
  >
    <template v-for="menu in menus" :key="menu.id">
      <!-- 目录类型且有子菜单 → 可展开的子菜单组 -->
      <el-sub-menu v-if="menu.type === 'M' && menu.children?.length" :index="'sub-' + menu.id">
        <template #title>
          <el-icon v-if="getIcon(menu.icon)"><component :is="getIcon(menu.icon)" /></el-icon>
          <span>{{ menu.name }}</span>
        </template>
        <el-menu-item v-for="child in menu.children" :key="child.id" :index="child.path">
          <el-icon v-if="getIcon(child.icon)"><component :is="getIcon(child.icon)" /></el-icon>
          <span>{{ child.name }}</span>
        </el-menu-item>
      </el-sub-menu>
      <!-- 菜单类型或无子菜单 → 直接跳转的单菜单项 -->
      <el-menu-item v-else :index="menu.path">
        <el-icon v-if="getIcon(menu.icon)"><component :is="getIcon(menu.icon)" /></el-icon>
        <span>{{ menu.name }}</span>
      </el-menu-item>
    </template>
  </el-menu>
</template>

<style scoped>
.el-menu {
  border-right: none;
  height: 100%;
}
</style>

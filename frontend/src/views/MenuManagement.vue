<!--
  MenuManagement.vue — 菜单管理页面。

  功能：
    - 菜单树表格展示（el-table 的 tree-props 实现树形展开）
    - 新增菜单（支持选择上级菜单，el-tree-select 组件）
    - 编辑菜单
    - 删除菜单（确认对话框）
    - 状态切换（启用/禁用，点击触发而非 change 事件）
    - 菜单类型：目录(M)、菜单(C)、按钮(F)，不同类型显示不同表单字段

  菜单树构建算法（buildMenuTree）：
    1. 遍历扁平列表创建 Map<id, node>
    2. parent_id === 0 的节点放入根数组
    3. 其余节点挂到父节点的 children 中
    4. cleanEmptyChildren 清理空 children 数组
-->
<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Edit, Delete, Search, Refresh } from '@element-plus/icons-vue'
import {
  getMenuTree, createMenu, updateMenu, deleteMenu, updateMenuStatus, type MenuItem,
} from '@/api/menu'

const loading = ref(false)
const menuList = ref<MenuItem[]>([])
/** el-table 的 key，用于强制重新渲染树表格 */
const tableKey = ref(0)
const searchForm = reactive({ name: '' })

/** 递归清理空 children 数组（避免 el-table 渲染多余展开箭头） */
function cleanEmptyChildren(items: MenuItem[]) {
  for (const item of items) {
    if (item.children?.length === 0) {
      delete item.children
    } else if (item.children) {
      cleanEmptyChildren(item.children)
    }
  }
}

/** 将扁平菜单列表构建为嵌套树结构 */
function buildMenuTree(items: MenuItem[]): MenuItem[] {
  const map = new Map<number, MenuItem>()
  const tree: MenuItem[] = []
  for (const item of items) {
    map.set(item.id, { ...item })
  }
  for (const item of items) {
    const node = map.get(item.id)!
    if (item.parent_id === 0) {
      tree.push(node)
    } else {
      const parent = map.get(item.parent_id)
      if (parent) {
        parent.children = parent.children || []
        parent.children.push(node)
      }
    }
  }
  cleanEmptyChildren(tree)
  return tree
}

// ---- 新增/编辑弹窗 ----
const dialogVisible = ref(false)
const dialogTitle = ref('新增菜单')
const formRef = ref<FormInstance>()
const form = reactive({
  id: undefined as number | undefined,
  parent_id: 0, name: '', path: '', component: '', icon: '', sort: 0, type: 'M' as string, perms: '', hidden: false,
})
const submitting = ref(false)

/** 菜单类型映射：前端标识 → 后端数值 */
const typeMap: Record<string, number> = { M: 1, C: 2, F: 3 }
const typeOptions = [
  { label: '目录', value: 'M' },
  { label: '菜单', value: 'C' },
  { label: '按钮', value: 'F' },
]

interface MenuOption { id: number; name: string; children?: MenuOption[] }
const menuOptions = ref<MenuOption[]>([])

/** 将菜单树转为 el-tree-select 可用的选项（过滤掉按钮类型） */
function buildOptions(tree: MenuItem[]): MenuOption[] {
  return tree.filter(m => m.type !== 'F').map(m => ({
    id: m.id,
    name: m.name,
    children: m.children ? buildOptions(m.children) : undefined,
  }))
}

/** 加载菜单树 */
async function loadTree() {
  loading.value = true
  try {
    const res = await getMenuTree()
    const tree = buildMenuTree(res.data || [])
    menuList.value = tree
    // 添加虚拟根节点 "主目录"
    menuOptions.value = [{ id: 0, name: '主目录', children: buildOptions(tree) }]
    tableKey.value++
  } finally { loading.value = false }
}

function handleSearch() { loadTree() }
function handleReset() { searchForm.name = ''; handleSearch() }

/** 新增菜单（可指定上级菜单） */
function handleAdd(parent?: MenuItem) {
  dialogTitle.value = '新增菜单'
  form.id = undefined
  form.parent_id = parent?.id || 0
  form.name = ''; form.path = ''; form.component = ''; form.icon = ''; form.sort = 0; form.type = 'M'; form.perms = ''; form.hidden = false
  dialogVisible.value = true
}

/** 编辑菜单（预填数据） */
function handleEdit(row: MenuItem) {
  dialogTitle.value = '编辑菜单'
  form.id = row.id
  form.parent_id = row.parent_id || 0
  form.name = row.name
  form.path = row.path
  form.component = row.component || ''
  form.icon = row.icon || ''
  form.sort = row.sort
  form.type = row.type
  form.perms = row.perms || ''
  form.hidden = row.hidden
  dialogVisible.value = true
}

/** 提交新增/编辑 */
async function handleSubmit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      const data: any = { ...form, type: typeMap[form.type] || 1 }
      if (data.type === 1) data.component = ''  // 目录不需要组件路径
      if (form.id) {
        await updateMenu(form.id, data)
        ElMessage.success('更新成功')
      } else {
        await createMenu(data)
        ElMessage.success('创建成功')
      }
      dialogVisible.value = false
      loadTree()
    } finally { submitting.value = false }
  })
}

/** 删除菜单 */
async function handleDelete(row: MenuItem) {
  try {
    await ElMessageBox.confirm(`确定要删除菜单 "${row.name}" 吗？子菜单也会一并删除`, '提示', { type: 'warning' })
    await deleteMenu(row.id)
    ElMessage.success('删除成功')
    loadTree()
  } catch { /* cancelled */ }
}

/** 切换状态（点击触发，而非 change 事件） */
async function handleStatusClick(row: MenuItem) {
  const newVal = row.status === 1 ? 0 : 1
  try {
    await updateMenuStatus(row.id, newVal)
    row.status = newVal
    ElMessage.success('状态更新成功')
  } catch { /* ignore */ }
}

const rules: FormRules = {
  name: [{ required: true, message: '请输入菜单名称', trigger: 'blur' }],
  path: [{ required: true, message: '请输入路由路径', trigger: 'blur' }],
}

/** Tag 颜色映射 */
const typeTagMap: Record<string, 'info' | 'success' | 'warning'> = { M: 'info', C: 'success', F: 'warning' }
/** 类型中文映射 */
const typeLabelMap: Record<string, string> = { M: '目录', C: '菜单', F: '按钮' }

onMounted(loadTree)
</script>

<template>
  <div class="page-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <div class="search-box">
            <el-input v-model="searchForm.name" placeholder="请输入菜单名称" class="search-input" clearable @keyup.enter="handleSearch">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <el-button type="primary" @click="handleSearch"><el-icon><Search /></el-icon>查询</el-button>
            <el-button @click="handleReset"><el-icon><Refresh /></el-icon>重置</el-button>
          </div>
          <el-button type="primary" @click="handleAdd()"><el-icon><Plus /></el-icon>新增根菜单</el-button>
        </div>
      </template>

      <!-- 菜单树表格（el-table 原生支持树形数据） -->
      <el-table :key="tableKey" :data="menuList" v-loading="loading" row-key="id" border
        :tree-props="{ children: 'children', hasChildren: 'hasChildren' }" style="width: 100%">
        <el-table-column prop="name" label="菜单名称" min-width="200" />
        <el-table-column prop="path" label="路由路径" min-width="160">
          <template #default="{ row }">{{ row.path || '-' }}</template>
        </el-table-column>
        <el-table-column prop="component" label="组件路径" min-width="180">
          <template #default="{ row }">{{ row.type === 'M' ? '-' : (row.component || '-') }}</template>
        </el-table-column>
        <el-table-column label="类型" width="80">
          <template #default="{ row }">
            <el-tag :type="typeTagMap[row.type] || 'info'" size="small">{{ typeLabelMap[row.type] || row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="icon" label="图标" width="80">
          <template #default="{ row }">{{ row.icon || '-' }}</template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="70" align="center" />
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.status" :active-value="1" :inactive-value="0" @click="handleStatusClick(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="240" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link @click="handleAdd(row)"><el-icon><Plus /></el-icon>新增</el-button>
            <el-button type="primary" link @click="handleEdit(row)"><el-icon><Edit /></el-icon>编辑</el-button>
            <el-button type="danger" link @click="handleDelete(row)"><el-icon><Delete /></el-icon>删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增/编辑弹窗（表单字段根据菜单类型动态显示） -->
    <el-dialog :title="dialogTitle" v-model="dialogVisible" width="580px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <!-- 上级菜单选择（el-tree-select） -->
        <el-form-item label="上级菜单">
          <el-tree-select v-model="form.parent_id" :data="menuOptions"
            :props="{ label: 'name', value: 'id', children: 'children', disabled: (d: any) => d.id === form.id }"
            placeholder="请选择上级菜单" clearable check-strictly :render-after-expand="false" />
        </el-form-item>

        <el-form-item label="菜单类型">
          <el-radio-group v-model="form.type">
            <el-radio-button v-for="opt in typeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="菜单名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入菜单名称" />
        </el-form-item>

        <!-- 非按钮类型：显示路由路径 -->
        <el-form-item v-if="form.type !== 'F'" label="路由路径" prop="path">
          <el-input v-model="form.path" placeholder="如 /system/users" />
        </el-form-item>

        <!-- 菜单类型：显示组件路径 -->
        <el-form-item v-if="form.type === 'C'" label="组件路径">
          <el-input v-model="form.component" placeholder="如 system/user/index" />
        </el-form-item>

        <!-- 非按钮类型：显示图标 -->
        <el-form-item v-if="form.type !== 'F'" label="图标">
          <el-input v-model="form.icon" placeholder="Element Plus 图标名" />
        </el-form-item>

        <!-- 按钮类型：显示权限标识 -->
        <el-form-item v-if="form.type === 'F'" label="权限标识">
          <el-input v-model="form.perms" placeholder="如 user:create" />
        </el-form-item>

        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" />
        </el-form-item>

        <el-form-item label="隐藏">
          <el-switch v-model="form.hidden" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.card-header { display: flex; justify-content: space-between; align-items: center; }
.search-box { display: flex; gap: 10px; }
.search-input { width: 240px; }
</style>

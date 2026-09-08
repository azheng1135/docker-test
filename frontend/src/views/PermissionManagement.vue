<!--
  PermissionManagement.vue — 权限管理页面。

  功能：
    - 权限定义列表分页查询（支持关键词 + HTTP 方法过滤）
    - 新增/编辑权限定义
    - 删除权限定义

  注意：此页面管理的是权限定义（Permission 表），
  与角色管理中的"分配权限"（Casbin p 规则）是不同维度。
-->
<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Edit, Delete, Search, Refresh } from '@element-plus/icons-vue'
import {
  getPermissions, createPermission, updatePermission, deletePermission, type Permission,
} from '@/api/permission'

const loading = ref(false)
const list = ref<Permission[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(10)

/** 搜索表单：关键词 + HTTP 方法过滤 */
const searchForm = reactive({ keyword: '', method: '' })

// ---- 新增/编辑弹窗 ----
const dialogVisible = ref(false)
const dialogTitle = ref('新增权限')
const formRef = ref<FormInstance>()
const form = reactive({ id: undefined as number | undefined, path: '', method: '', description: '' })
const submitting = ref(false)

const rules: FormRules = {
  path: [{ required: true, message: '请输入请求路径', trigger: 'blur' }],
  method: [{ required: true, message: '请选择请求方法', trigger: 'change' }],
}

/** 根据 HTTP 方法返回对应的 Tag 类型 */
function getMethodTag(m: string) {
  const map: Record<string, string> = { GET: 'success', POST: 'primary', PUT: 'warning', DELETE: 'danger' }
  return map[m] || 'info'
}

/** 加载权限列表 */
async function loadList() {
  loading.value = true
  try {
    const res = await getPermissions({
      page: currentPage.value, page_size: pageSize.value,
      keyword: searchForm.keyword, method: searchForm.method,
    })
    list.value = res.data.list || []
    total.value = res.data.total || 0
  } finally { loading.value = false }
}

function handleSearch() { currentPage.value = 1; loadList() }
function handleReset() { searchForm.keyword = ''; searchForm.method = ''; handleSearch() }

function handleAdd() {
  dialogTitle.value = '新增权限'
  form.id = undefined; form.path = ''; form.method = ''; form.description = ''
  dialogVisible.value = true
}

function handleEdit(row: Permission) {
  dialogTitle.value = '编辑权限'
  Object.assign(form, row)
  dialogVisible.value = true
}

/** 提交新增/编辑 */
async function handleSubmit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      const data = { path: form.path, method: form.method, description: form.description }
      if (form.id) {
        await updatePermission(form.id, data)
        ElMessage.success('更新成功')
      } else {
        await createPermission(data)
        ElMessage.success('创建成功')
      }
      dialogVisible.value = false
      loadList()
    } finally { submitting.value = false }
  })
}

/** 删除权限定义 */
async function handleDelete(row: Permission) {
  try {
    await ElMessageBox.confirm('确定要删除该权限吗？', '提示', { type: 'warning' })
    await deletePermission(row.id)
    ElMessage.success('删除成功')
    loadList()
  } catch { /* cancelled */ }
}

onMounted(loadList)
</script>

<template>
  <div class="page-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <div class="search-box">
            <!-- 路径/描述关键词搜索 -->
            <el-input v-model="searchForm.keyword" placeholder="搜索路径或描述" class="search-input" clearable @keyup.enter="handleSearch">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <!-- HTTP 方法过滤下拉 -->
            <el-select v-model="searchForm.method" placeholder="请求方法" clearable style="width: 120px" @change="handleSearch">
              <el-option label="GET" value="GET" />
              <el-option label="POST" value="POST" />
              <el-option label="PUT" value="PUT" />
              <el-option label="DELETE" value="DELETE" />
            </el-select>
            <el-button type="primary" @click="handleSearch"><el-icon><Search /></el-icon>查询</el-button>
            <el-button @click="handleReset"><el-icon><Refresh /></el-icon>重置</el-button>
          </div>
          <el-button type="primary" @click="handleAdd"><el-icon><Plus /></el-icon>新增权限</el-button>
        </div>
      </template>

      <!-- 权限列表 -->
      <el-table :data="list" v-loading="loading" border>
        <el-table-column prop="path" label="请求路径" min-width="200" show-overflow-tooltip />
        <el-table-column prop="method" label="请求方法" width="100">
          <template #default="{ row }">
            <el-tag :type="getMethodTag(row.method)" size="small">{{ row.method }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link @click="handleEdit(row)"><el-icon><Edit /></el-icon>编辑</el-button>
            <el-button type="danger" link @click="handleDelete(row)"><el-icon><Delete /></el-icon>删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage" v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]" :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadList" @current-change="loadList"
        />
      </div>
    </el-card>

    <!-- 新增/编辑弹窗 -->
    <el-dialog :title="dialogTitle" v-model="dialogVisible" width="500px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="请求路径" prop="path">
          <el-input v-model="form.path" placeholder="如 /api/v1/users" />
        </el-form-item>
        <el-form-item label="请求方法" prop="method">
          <el-select v-model="form.method" placeholder="请选择请求方法" style="width: 100%">
            <el-option label="GET" value="GET" />
            <el-option label="POST" value="POST" />
            <el-option label="PUT" value="PUT" />
            <el-option label="DELETE" value="DELETE" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" placeholder="请输入权限描述" />
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
.pagination-container { margin-top: 20px; display: flex; justify-content: flex-end; }
</style>

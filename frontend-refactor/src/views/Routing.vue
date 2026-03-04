<!-- src/views/Routing.vue -->
<template>
  <div class="routing">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>路由管理</h3>
          <div class="header-actions">
            <el-button type="primary" @click="loadNodes">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
            <el-button type="primary" @click="showAddRouteDialog">
              <el-icon><Plus /></el-icon>
              添加路由
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="filteredRoutes"
        stripe
        style="width: 100%"
        v-loading="loading"
      >
        <el-table-column prop="hostname" label="节点名称" width="150" />
        <el-table-column prop="publicIp" label="公网IP" width="140" />
        <el-table-column prop="region" label="区域" width="100" />
        <el-table-column prop="routes" label="发布的路由" width="200">
          <template #default="{ row }">
            <el-tag
              v-for="route in row.routes"
              :key="route"
              type="success"
              style="margin-right: 5px; margin-bottom: 5px;"
            >
              {{ route }}
            </el-tag>
            <el-tag v-if="row.routes.length === 0" type="info" size="small">无</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="actions" label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showEditRouteDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="showDeleteRouteDialog(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredRoutes.length"
          layout="sizes, prev, pager, next, jumper, ->, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <!-- 添加/编辑路由对话框 -->
    <el-dialog
      v-model="routeDialogVisible"
      :title="dialogMode === 'add' ? '添加路由' : '编辑路由'"
      width="500px"
      :before-close="closeRouteDialog"
    >
      <el-form :model="currentRoute" label-width="100px">
        <el-form-item label="节点名称">
          <el-select v-model="currentRoute.hostname" placeholder="请选择节点" style="width: 100%">
            <el-option
              v-for="node in allNodes"
              :key="node.hostname"
              :label="`${node.hostname} (${node.region})`"
              :value="node.hostname"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="路由网段">
          <el-input
            v-model="currentRoute.cidr"
            placeholder="请输入CIDR格式的路由网段，如 192.168.1.0/24"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="closeRouteDialog">取消</el-button>
          <el-button type="primary" @click="confirmRouteAction">
            {{ dialogMode === 'add' ? '添加' : '更新' }}
          </el-button>
        </span>
      </template>
    </el-dialog>

    <!-- 删除确认对话框 -->
    <el-dialog
      v-model="deleteDialogVisible"
      title="删除路由"
      width="400px"
    >
      <p>确定要从节点 <strong>{{ currentDeleteNode?.hostname }}</strong> 删除路由 <strong>{{ currentDeleteCidr }}</strong> 吗？</p>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="deleteDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="confirmDeleteRoute">确定</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, h } from 'vue'
import { Refresh, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, ElOption, ElSelect } from 'element-plus'
import { useRouteApi } from '@/composables/useRouteApi'

// 响应式数据
const loading = ref(false)
const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)

// 路由数据
const allNodes = ref([])
const routeDialogVisible = ref(false)
const deleteDialogVisible = ref(false)
const dialogMode = ref('add') // 'add' or 'edit'
const currentRoute = ref({
  hostname: '',
  cidr: ''
})
const currentDeleteNode = ref(null)
const currentDeleteCidr = ref('')

// 分页计算
const paginatedRoutes = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredRoutes.value.slice(start, start + pageSize.value)
})

// 过滤计算
const filteredRoutes = computed(() => {
  if (!searchQuery.value) {
    return allNodes.value
  }

  return allNodes.value.filter(node =>
    node.hostname.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
    node.publicIp.includes(searchQuery.value) ||
    node.region.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
    node.routes.some(route => route.includes(searchQuery.value))
  )
})

// 加载节点数据
const loadNodes = async () => {
  loading.value = true
  try {
    allNodes.value = await useRouteApi.getRoutes()
    console.log('[Routing] Loaded routes:', allNodes.value)
  } catch (error) {
    console.error('[Routing] 加载路由失败:', error)
    ElMessage.error('加载路由失败: ' + error.message)
    allNodes.value = []
  } finally {
    loading.value = false
  }
}

// 显示添加路由对话框
const showAddRouteDialog = () => {
  dialogMode.value = 'add'
  currentRoute.value = { hostname: '', cidr: '' }
  routeDialogVisible.value = true
}

// 显示编辑路由对话框
const showEditRouteDialog = (node) => {
  currentRoute.value = { hostname: node.hostname, cidr: '' }
  routeDialogVisible.value = true
}

// 显示删除路由对话框
const showDeleteRouteDialog = (node) => {
  if (node.routes.length === 0) {
    ElMessage.warning('该节点没有可删除的路由')
    return
  }

  if (node.routes.length === 1) {
    // 如果只有一个路由，直接设置并打开确认对话框
    currentDeleteNode.value = node
    currentDeleteCidr.value = node.routes[0]
    deleteDialogVisible.value = true
  } else {
    // 如果有多个路由，创建一个自定义的选择对话框
    const routeOptions = node.routes.map(route =>
      h(ElOption, { key: route, value: route, label: route })
    );

    const selectVNode = h(ElSelect, {
      modelValue: node.routes[0],
      'onUpdate:modelValue': (value) => {
        currentDeleteCidr.value = value;
      },
      style: 'width: 100%; margin-top: 10px;'
    }, { default: () => routeOptions });

    ElMessageBox({
      title: '选择要删除的路由',
      message: h('div', null, [
        h('p', { style: 'margin-bottom: 10px;' }, `请选择要从节点 ${node.hostname} 删除的路由:`),
        selectVNode
      ]),
      showCancelButton: true,
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      customClass: 'custom-message-box'
    }).then(() => {
      currentDeleteNode.value = node;
      // currentDeleteCidr.value 已经通过 onUpdate:modelValue 更新
      deleteDialogVisible.value = true;
    }).catch(() => {
      // 用户取消
    });
  }
}

// 确认路由操作
const confirmRouteAction = async () => {
  if (!currentRoute.value.hostname || !currentRoute.value.cidr) {
    ElMessage.error('请填写完整的路由信息')
    return
  }

  // 验证CIDR格式
  const cidrPattern = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/
  if (!cidrPattern.test(currentRoute.value.cidr)) {
    ElMessage.error('请输入有效的CIDR格式，如 192.168.1.0/24')
    return
  }

  try {
    // 查找目标节点
    const node = allNodes.value.find(n => n.hostname === currentRoute.value.hostname)
    if (!node) {
      ElMessage.error('未找到目标节点')
      return
    }

    // 添加新路由到现有路由列表
    const newRoutes = [...node.routes, currentRoute.value.cidr]
    
    // 调用 API 更新路由
    await useRouteApi.updateRoutes(node.id, newRoutes)
    
    ElMessage.success('路由添加成功')
    await loadNodes() // 重新加载数据
    closeRouteDialog()
  } catch (error) {
    console.error('[Routing] 添加路由失败:', error)
    ElMessage.error('添加路由失败: ' + error.message)
  }
}

// 确认删除路由
const confirmDeleteRoute = async () => {
  try {
    // 从节点的路由列表中移除指定路由
    const newRoutes = currentDeleteNode.value.routes.filter(r => r !== currentDeleteCidr.value)
    
    // 调用 API 更新路由
    await useRouteApi.updateRoutes(currentDeleteNode.value.id, newRoutes)
    
    ElMessage.success('路由删除成功')
    await loadNodes() // 重新加载数据
    deleteDialogVisible.value = false
  } catch (error) {
    console.error('[Routing] 删除路由失败:', error)
    ElMessage.error('删除路由失败: ' + error.message)
  }
}

// 关闭对话框
const closeRouteDialog = () => {
  routeDialogVisible.value = false
  currentRoute.value = { hostname: '', cidr: '' }
}

// 分页处理
const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page) => {
  currentPage.value = page
}

// 初始化数据
onMounted(() => {
  loadNodes()
})
</script>

<style scoped>
.routing {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
<!-- src/views/Nodes.vue -->
<template>
  <div class="nodes-page page-shell">
    <PageHeader
      :title="t('nodeManagement.title')"
      :subtitle="t('nodesPage.subtitle')"
    >
      <template #actions>
        <el-button
          :icon="Refresh"
          @click="refreshNodes"
          :loading="loading"
        >
          {{ t('common.refresh') }}
        </el-button>
        <el-button v-if="hasPermission('nodes:write')" type="primary" :icon="Plus" @click="addNode">
          {{ t('nodesPage.addNode') }}
        </el-button>
      </template>
    </PageHeader>

    <MetricStrip :metrics="nodeMetricItems" />

    <FilterBar>
      <template #filters>
        <el-input
          v-model="searchQuery"
          :placeholder="t('nodesPage.searchPlaceholder')"
          class="search-input"
          clearable
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </template>
    </FilterBar>

    <DataPanel
      class="nodes-card"
      :title="t('nodesPage.nodeFleet')"
      :subtitle="t('nodesPage.nodeFleetSubtitle')"
    >
      <el-table
        :data="paginatedNodes"
        stripe
        class="nodes-table"
        v-loading="loading"
      >
        <el-table-column prop="hostname" :label="t('nodeManagement.hostname')" min-width="140" />
        <el-table-column prop="publicIp" :label="t('nodesPage.publicIp')" width="130" />
        <el-table-column prop="vpnIp" :label="t('nodesPage.vpnIp')" width="120" />
        <el-table-column prop="region" :label="t('nodeManagement.region')" width="80">
          <template #default="{ row }">
            <span class="region-badge">{{ row.region.toUpperCase() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="version" :label="t('nodeManagement.version')" width="100" />
        <el-table-column prop="mode" :label="t('nodesPage.mode')" width="100">
          <template #default="{ row }">
            <div class="mode-badge">
              {{ row.mode }}
              <el-tag
                v-if="row.mode === 'kernel'"
                size="small"
                type="success"
                effect="plain"
              >
                {{ t('nodesPage.optimized') }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('nodeManagement.status')" width="100">
          <template #default="{ row }">
            <StatusBadge :status="row.status" :label="row.status" />
          </template>
        </el-table-column>
        <el-table-column :label="t('nodesPage.onboarding')" width="130">
          <template #default="{ row }">
            <el-tooltip
              v-if="row.onboarding?.lastError || row.onboarding?.nextAction"
              :content="row.onboarding.lastError || row.onboarding.nextAction"
              placement="top"
            >
              <el-tag size="small" :type="getOnboardingPhaseTagType(row.onboarding?.phase)">
                {{ formatOnboardingPhase(row.onboarding?.phase) }}
              </el-tag>
            </el-tooltip>
            <el-tag v-else size="small" :type="getOnboardingPhaseTagType(row.onboarding?.phase)">
              {{ formatOnboardingPhase(row.onboarding?.phase) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('nodesPage.sync')" width="120">
          <template #default="{ row }">
            <el-tooltip
              v-if="row.observedMessage || row.lastCommandError"
              :content="row.observedMessage || row.lastCommandError"
              placement="top"
            >
              <el-tag size="small" :type="getConvergenceTagType(row.stateConvergence)">
                {{ formatConvergence(row.stateConvergence) }}
              </el-tag>
            </el-tooltip>
            <el-tag v-else size="small" :type="getConvergenceTagType(row.stateConvergence)">
              {{ formatConvergence(row.stateConvergence) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('nodesPage.desiredApplied')" width="180">
          <template #default="{ row }">
            <div class="state-version-cell" :class="{ 'version-mismatch': row.desiredStateVersion !== row.appliedStateVersion }">
              <div class="version-line desired">
                <el-tooltip :content="t('nodesPage.desiredStateVersion')" placement="left">
                  <span>{{ shortStateVersion(row.desiredStateVersion) }}</span>
                </el-tooltip>
              </div>
              <div class="version-line applied muted-line">
                <el-tooltip :content="t('nodesPage.appliedStateVersion')" placement="left">
                  <span>{{ shortStateVersion(row.appliedStateVersion) }}</span>
                </el-tooltip>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="pendingCmds" :label="t('nodesPage.pending')" width="90" />
        <el-table-column prop="lastSeen" :label="t('nodeManagement.lastSeen')" width="150" />
        <el-table-column :label="t('nodeManagement.actions')" width="180" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <ActionIconButton :label="t('nodesPage.viewNodeDetails')" @click="viewNodeDetails(row)">
                <el-icon><View /></el-icon>
              </ActionIconButton>
              <ActionIconButton
                v-if="hasPermission('nodes:write')"
                :label="t('nodesPage.editNode')"
                tone="primary"
                @click="handleEditNode(row)"
              >
                <el-icon><Edit /></el-icon>
              </ActionIconButton>
              <el-popconfirm
                v-if="hasPermission('nodes:write')"
                :title="t('nodesPage.deleteConfirm')"
                @confirm="handleDeleteNode(row.id)"
              >
                <template #reference>
                  <ActionIconButton :label="t('nodesPage.deleteNode')" tone="danger">
                    <el-icon><Delete /></el-icon>
                  </ActionIconButton>
                </template>
              </el-popconfirm>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredNodes.length"
          layout="sizes, prev, pager, next, jumper, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </DataPanel>

    <!-- 节点详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      :title="t('nodesPage.nodeDetails')"
      width="800px"
      custom-class="node-detail-dialog"
      @closed="closeDetailDialog"
    >
      <div v-if="selectedNode" class="node-detail-content">
        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Operation /></el-icon>
              {{ t('nodeMonitorDetail.operationsSummary') }}
            </h4>
            <el-button size="small" @click="openMonitoringDetail('commands')">
              {{ t('nodeMonitorDetail.monitoringContext') }}
            </el-button>
          </div>
          <div class="workbench-summary-grid">
            <button
              v-for="item in workbenchSummary"
              :key="item.key"
              type="button"
              class="workbench-summary-item"
              @click="handleSummaryClick(item.focus)"
            >
              <span class="summary-label">{{ item.label }}</span>
              <span class="summary-value">{{ item.value }}</span>
              <el-tag size="small" :type="item.type">{{ item.status }}</el-tag>
            </button>
          </div>
        </div>

        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Connection /></el-icon>
            {{ t('nodesPage.onboardingEvidence') }}
          </h4>
          <el-descriptions :column="2" border>
            <el-descriptions-item :label="t('nodesPage.phase')">
              <el-tag size="small" :type="getOnboardingPhaseTagType(selectedNode.onboarding?.phase)">
                {{ formatOnboardingPhase(selectedNode.onboarding?.phase) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.token')">
              {{ selectedNode.onboarding?.tokenPreview || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.firstSeen')">
              {{ selectedNode.onboarding?.firstSeenAt || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.lastSync')">
              {{ selectedNode.onboarding?.lastSyncAt || selectedNode.lastSyncAt || 'N/A' }}
            </el-descriptions-item>
          </el-descriptions>
          <el-alert
            v-if="selectedNode.onboarding?.lastError || selectedNode.onboarding?.nextAction"
            class="state-alert"
            :title="selectedNode.onboarding?.lastError || selectedNode.onboarding?.nextAction"
            :description="selectedNode.onboarding?.nextAction"
            :type="selectedNode.onboarding?.phase === 'degraded' ? 'error' : 'info'"
            show-icon
            :closable="false"
          />
          <pre class="diagnostic-command">{{ onboardingTroubleshootingCommand(selectedNode) }}</pre>
        </div>

        <!-- 基础信息 -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><InfoFilled /></el-icon>
            {{ t('nodesPage.basicInformation') }}
          </h4>
          <el-descriptions :column="2" border>
            <el-descriptions-item :label="t('nodeManagement.hostname')">{{ selectedNode.hostname }}</el-descriptions-item>
            <el-descriptions-item :label="t('nodeManagement.region')">
              <span class="region-badge">{{ selectedNode.region.toUpperCase() }}</span>
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.publicIp')">{{ selectedNode.publicIp }}</el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.vpnIp')">{{ selectedNode.vpnIp }}</el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.endpoint')">{{ selectedNode.endpoint }}</el-descriptions-item>
            <el-descriptions-item :label="t('nodeManagement.status')">
              <span class="status-badge" :class="selectedNode.status">
                {{ selectedNode.status }}
              </span>
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodeManagement.uptime')">{{ selectedNode.uptime }}</el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Connection /></el-icon>
            {{ t('nodesPage.controlState') }}
          </h4>
          <div class="control-state-grid">
            <div class="control-state-item">
              <div class="control-state-label">{{ t('nodesPage.desiredVersion') }}</div>
              <div class="control-state-value" :class="{ danger: isStateDiverged }">
                {{ selectedNode.desiredStateVersion || 'N/A' }}
              </div>
              <div class="control-state-time">{{ selectedNode.desiredStateUpdatedAt || 'N/A' }}</div>
            </div>
            <div class="control-state-item">
              <div class="control-state-label">{{ t('nodesPage.appliedVersion') }}</div>
              <div class="control-state-value" :class="{ danger: isStateDiverged }">
                {{ selectedNode.appliedStateVersion || 'N/A' }}
              </div>
              <div class="control-state-time">{{ selectedNode.appliedStateUpdatedAt || 'N/A' }}</div>
            </div>
            <div class="control-state-item">
              <div class="control-state-label">{{ t('nodesPage.observedState') }}</div>
              <div class="control-state-value">{{ selectedNode.observedState || 'idle' }}</div>
              <div class="control-state-time">{{ selectedNode.observedAt || 'N/A' }}</div>
            </div>
            <div class="control-state-item">
              <div class="control-state-label">{{ t('nodesPage.convergence') }}</div>
              <div class="control-state-value">
                <el-tag size="small" :type="getConvergenceTagType(selectedNode.stateConvergence)">
                  {{ formatConvergence(selectedNode.stateConvergence) }}
                </el-tag>
              </div>
              <div class="control-state-time">{{ t('nodesPage.lastSync') }}: {{ selectedNode.lastSyncAt || 'N/A' }}</div>
            </div>
          </div>
          <el-alert
            v-if="selectedNode.observedMessage || selectedNode.lastCommandError"
            class="state-alert"
            :title="selectedNode.observedMessage || selectedNode.lastCommandError"
            :type="selectedNode.stateConvergence === 'diverged' || selectedNode.lastCommandError ? 'error' : 'info'"
            show-icon
            :closable="false"
          />
        </div>

        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Key /></el-icon>
              {{ t('nodeMonitorDetail.certificateStatus') }}
            </h4>
            <el-button size="small" @click="openMonitoringDetail('certificate')">
              {{ t('nodesPage.certificateContext') }}
            </el-button>
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item :label="t('nodeManagement.status')">
              <el-tag size="small" :type="getCertificateStatusTagType(selectedNode.certificate?.status)">
                {{ selectedNode.certificate?.status || 'missing' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodeMonitorDetail.serialNumber')">
              {{ selectedNode.certificate?.serial_number || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodeMonitorDetail.issuedAt')">
              {{ formatCommandTime(selectedNode.certificate?.issued_at) }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodeMonitorDetail.expiresAt')">
              {{ formatCommandTime(selectedNode.certificate?.not_after) }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.lastRenewed')">
              {{ formatCommandTime(selectedNode.certificateActivity?.last_renewed_at) }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.renewFailure')">
              {{ selectedNode.certificateActivity?.last_renew_failure || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodesPage.lastRevoked')">
              {{ formatCommandTime(selectedNode.certificateActivity?.last_revoked_at) }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('nodeMonitorDetail.revokeReason')">
              {{ selectedNode.certificateActivity?.last_revoke_reason || selectedNode.certificate?.revoke_reason || 'N/A' }}
            </el-descriptions-item>
          </el-descriptions>
          <div v-if="selectedNode.certificateActivity?.last_renewed_serial_number" class="certificate-note">
            {{ t('nodeMonitorDetail.lastRenewedSerial') }}: {{ selectedNode.certificateActivity.last_renewed_serial_number }}
          </div>
        </div>

        <!-- 实时监控指标 -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><TrendCharts /></el-icon>
            {{ t('nodesPage.realTimeMetrics') }}
          </h4>
          <div class="stats-grid">
            <div class="stat-box">
              <div class="stat-icon-box upload">
                <el-icon><Upload /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.bandwidth?.upload || 0 }} Mbps</div>
                <div class="stat-box-label">{{ t('nodeManagement.upload') }}</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box download">
                <el-icon><Download /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.bandwidth?.download || 0 }} Mbps</div>
                <div class="stat-box-label">{{ t('nodeManagement.download') }}</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box latency">
                <el-icon><Timer /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.latency || 0 }} ms</div>
                <div class="stat-box-label">{{ t('nodeManagement.latency') }}</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box policies">
                <el-icon><Operation /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ formatLargeNumber(policyDatapathStats.aclPackets) }}</div>
                <div class="stat-box-label">{{ t('nodesPage.aclPackets') }}</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box qos">
                <el-icon><TrendCharts /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ formatMetricBytes(policyDatapathStats.qosPassedBytes) }}</div>
                <div class="stat-box-label">{{ t('nodesPage.qosPassed') }}</div>
              </div>
            </div>
          </div>
        </div>

        <!-- 路由信息 (Site-to-Site) -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Upload /></el-icon>
            {{ t('nodesPage.advertisedRoutesSite') }}
          </h4>
          <div class="routes-list">
            <el-empty v-if="!selectedNode.routes || selectedNode.routes.length === 0" :image-size="40" :description="t('nodesPage.noAdvertisedRoutes')" />
            <el-tag
              v-for="route in selectedNode.routes"
              :key="route"
              size="large"
              type="info"
              effect="plain"
              class="route-tag"
            >
              {{ route }}
            </el-tag>
          </div>
        </div>

        <!-- 学习到的路由 (Mesh) -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Position /></el-icon>
            {{ t('nodesPage.learnedRoutesMesh') }}
          </h4>
          <el-table
            :data="selectedNode.learnedRoutes || []"
            size="small"
            :empty-text="t('nodesPage.noLearnedRoutes')"
            class="learned-routes-table"
          >
            <el-table-column prop="cidr" label="CIDR" width="150">
              <template #default="{ row }">
                <code class="route-code">{{ row.cidr }}</code>
              </template>
            </el-table-column>
            <el-table-column prop="next_hop_node" :label="t('nodesPage.nextHopNode')" min-width="120" />
            <el-table-column prop="next_hop_ip" :label="t('nodesPage.vpnIp')" width="120" />
            <el-table-column prop="region" :label="t('nodeManagement.region')" width="100">
              <template #default="{ row }">
                <span class="region-badge">{{ row.region.toUpperCase() }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('nodeManagement.status')" width="100">
              <template #default="{ row }">
                <span class="status-badge" :class="row.status">
                  {{ row.status }}
                </span>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- 最近命令 -->
        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Timer /></el-icon>
              {{ t('nodesPage.recentCommands') }}
            </h4>
            <div class="detail-toolbar">
              <el-button size="small" @click="openMonitoringDetail('commands')">
                {{ t('nodesPage.monitoringDetail') }}
              </el-button>
              <el-button size="small" @click="openPolicyCenter()">
                {{ t('nodesPage.policyCenter') }}
              </el-button>
              <el-button
                v-if="hasPermission('commands:write')"
                size="small"
                type="primary"
                :loading="commandLoading"
                @click="runQuickCommand('sync')"
              >
                {{ t('nodesPage.forceSync') }}
              </el-button>
              <el-button
                v-if="hasPermission('commands:write')"
                size="small"
                :loading="commandLoading"
                @click="runQuickCommand('health_check')"
              >
                {{ t('monitoringPage.healthCheck') }}
              </el-button>
            </div>
          </div>
          <el-table
            :data="selectedNode.recentCommands || []"
            size="small"
            :empty-text="t('nodesPage.noCommandsYet')"
          >
            <el-table-column label="ID" width="110">
              <template #default="{ row }">
                <el-tooltip v-if="row.id" :content="row.id" placement="top">
                  <span class="mono-text">{{ shortCommandId(row.id) }}</span>
                </el-tooltip>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="command" :label="t('nodeMonitorDetail.command')" min-width="120" />
            <el-table-column prop="status" :label="t('nodeManagement.status')" width="120">
              <template #default="{ row }">
                <el-tag :type="commandStatusTagType(row.status)" size="small">
                  {{ commandStatusLabel(row.status, t) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="message" :label="t('nodeMonitorDetail.message')" min-width="220" show-overflow-tooltip />
            <el-table-column :label="t('nodeMonitorDetail.created')" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('nodeMonitorDetail.completed')" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.completed_at || row.updated_at) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('nodeManagement.actions')" width="100">
              <template #default="{ row }">
                <el-button size="small" link @click="openMonitoringCommand(row)">
                  {{ t('nodesPage.open') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Warning /></el-icon>
              {{ t('nodesPage.activeAlerts') }}
            </h4>
            <el-button size="small" @click="openMonitoringDetail('alerts')">
              {{ t('nodesPage.openMonitoring') }}
            </el-button>
          </div>
          <el-table
            :data="selectedNode.activeAlerts || []"
            size="small"
            :empty-text="t('nodeMonitorDetail.noActiveAlerts')"
          >
            <el-table-column prop="severity" :label="t('nodeMonitorDetail.severity')" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="getAlertSeverityTagType(row.severity)">
                  {{ row.severity || 'info' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="alert_type" :label="t('nodeMonitorDetail.type')" width="150" />
            <el-table-column prop="title" :label="t('nodeMonitorDetail.titleColumn')" min-width="180" show-overflow-tooltip />
            <el-table-column prop="message" :label="t('nodeMonitorDetail.message')" min-width="220" show-overflow-tooltip />
            <el-table-column :label="t('nodeMonitorDetail.created')" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('nodeManagement.actions')" width="100">
              <template #default="{ row }">
                <el-button size="small" link @click="openMonitoringAlert(row)">
                  {{ t('nodesPage.open') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Document /></el-icon>
              {{ t('nodesPage.recentPolicyDeliveries') }}
            </h4>
            <el-button size="small" @click="openPolicyCenter()">
              {{ t('nodesPage.openPolicyCenter') }}
            </el-button>
          </div>
          <el-table
            :data="selectedNode.recentPolicyDeliveries || []"
            size="small"
            :empty-text="t('nodeMonitorDetail.noRecentPolicyDeliveries')"
          >
            <el-table-column prop="policy_domain" :label="t('nodeMonitorDetail.domain')" width="120" />
            <el-table-column :label="t('nodesPage.policy')" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">
                {{ row.policy_name || row.policy_ref || '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="policy_ref" :label="t('nodesPage.ref')" width="150" show-overflow-tooltip />
            <el-table-column :label="t('nodeMonitorDetail.command')" width="120">
              <template #default="{ row }">
                <el-tooltip v-if="row.command_id" :content="row.command_id" placement="top">
                  <span class="mono-text">{{ shortCommandId(row.command_id) }}</span>
                </el-tooltip>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="command_status" :label="t('nodeManagement.status')" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="commandStatusTagType(row.command_status)">
                  {{ commandStatusLabel(row.command_status, t) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="t('nodesPage.updated')" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.updated_at || row.completed_at || row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column prop="last_error" :label="t('nodeMonitorDetail.error')" min-width="220" show-overflow-tooltip />
            <el-table-column :label="t('nodeManagement.actions')" width="120">
              <template #default="{ row }">
                <el-button size="small" link @click="openPolicyCenter(row)">
                  {{ t('nodesPage.open') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </el-dialog>

    <!-- 节点接入向导 -->
    <el-dialog
      v-model="onboardingDialogVisible"
      :title="t('nodesPage.onboardNewNode')"
      width="760px"
      custom-class="node-onboarding-dialog"
    >
      <div class="onboarding-flow">
        <el-alert
          :title="t('nodesPage.onboardingIntro')"
          type="info"
          show-icon
          :closable="false"
        />

        <div class="onboarding-section">
          <h4>{{ t('nodesPage.enrollmentTokenStep') }}</h4>
          <el-form :model="onboardingForm" label-width="150px">
            <el-form-item :label="t('nodesPage.tokenTag')">
              <el-input v-model="onboardingForm.tokenTag" placeholder="node-onboarding" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.maxUses')">
              <el-input-number v-model="onboardingForm.maxUses" :min="1" :max="1000" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.ttlHours')">
              <el-input-number v-model="onboardingForm.ttlHours" :min="1" :max="8760" />
            </el-form-item>
            <el-form-item>
              <el-button
                v-if="hasPermission('tokens:write')"
                type="primary"
                :loading="onboardingCreating"
                @click="createOnboardingToken"
              >
                {{ t('nodesPage.createToken') }}
              </el-button>
            </el-form-item>
            <el-form-item v-if="onboardingTokenValue" :label="t('nodesPage.token')">
              <el-input :value="onboardingTokenValue" readonly>
                <template #append>
                  <el-button @click="copyOnboardingToken">{{ t('common.copy') }}</el-button>
                </template>
              </el-input>
            </el-form-item>
          </el-form>
        </div>

        <div class="onboarding-section">
          <h4>{{ t('nodesPage.targetSettingsStep') }}</h4>
          <el-form :model="onboardingForm" label-width="150px">
            <el-form-item :label="t('nodesPage.grpcServer')">
              <el-input v-model="onboardingForm.server" placeholder="https://aria.yun:50051" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.controllerApi')">
              <el-input v-model="onboardingForm.controllerApiUrl" placeholder="https://aria.yun" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.controllerCaPath')">
              <el-input v-model="onboardingForm.caCertPath" placeholder="/etc/aria/certs/ca.crt" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.controllerCaUrl')">
              <el-input v-model="onboardingForm.caUrl" placeholder="https://aria.yun/api/v2/controller-info/grpc-ca.crt" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.tlsServerName')">
              <el-input v-model="onboardingForm.tlsServerName" placeholder="aria.yun" />
            </el-form-item>
            <el-form-item :label="t('nodeManagement.region')">
              <el-input v-model="onboardingForm.region" placeholder="default" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.interface')">
              <el-input v-model="onboardingForm.interface" placeholder="aria0" />
            </el-form-item>
            <el-form-item :label="t('nodeManagement.hostname')">
              <el-input v-model="onboardingForm.hostname" :placeholder="t('nodesPage.optionalPlaceholder')" />
            </el-form-item>
            <el-form-item :label="t('nodesPage.advertiseRoutes')">
              <el-input v-model="onboardingForm.advertiseRoutes" :placeholder="t('nodesPage.commaCidrsPlaceholder')" />
            </el-form-item>
          </el-form>
        </div>

        <div class="onboarding-section">
          <h4>{{ t('nodesPage.installCommandStep') }}</h4>
          <pre class="init-command">{{ onboardingInstallCommand }}</pre>
          <div class="onboarding-actions">
            <el-button :disabled="!onboardingInstallCommand" @click="copyOnboardingCommand">
              {{ t('nodesPage.copyInstallCommand') }}
            </el-button>
          </div>
          <h5>{{ t('nodesPage.advancedInitOnlyCommand') }}</h5>
          <pre class="init-command">{{ onboardingInitCommand }}</pre>
          <div class="onboarding-actions">
            <el-button :disabled="!onboardingInitCommand" @click="copyOnboardingInitCommand">
              {{ t('nodesPage.copyInitOnlyCommand') }}
            </el-button>
          </div>
        </div>

        <div class="onboarding-section">
          <div class="onboarding-section-header">
            <h4>{{ t('nodesPage.progressStep') }}</h4>
            <el-button size="small" :icon="Refresh" :loading="loading" @click="refreshOnboardingProgress">
              {{ t('common.refresh') }}
            </el-button>
          </div>
          <el-table :data="recentOnboardingNodes" size="small" :empty-text="t('nodesPage.noRegisteredNodesYet')">
            <el-table-column prop="hostname" :label="t('nodeManagement.hostname')" min-width="150" />
            <el-table-column :label="t('nodesPage.phase')" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="getOnboardingPhaseTagType(row.onboarding?.phase)">
                  {{ formatOnboardingPhase(row.onboarding?.phase) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="t('nodesPage.token')" width="130">
              <template #default="{ row }">{{ row.onboarding?.tokenPreview || 'N/A' }}</template>
            </el-table-column>
            <el-table-column :label="t('nodesPage.lastSync')" min-width="160">
              <template #default="{ row }">{{ row.onboarding?.lastSyncAt || row.lastSyncAt || 'N/A' }}</template>
            </el-table-column>
            <el-table-column :label="t('nodesPage.action')" width="100">
              <template #default="{ row }">
                <el-button size="small" link type="primary" @click="openNodeDetailFromOnboarding(row)">
                  {{ t('nodesPage.detail') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="onboarding-section">
          <h4>{{ t('nodesPage.verifyStep') }}</h4>
          <ul class="onboarding-checklist">
            <li>{{ t('nodesPage.verifyInstall') }}</li>
            <li>{{ t('nodesPage.verifySystemd') }}</li>
            <li>{{ t('nodesPage.verifyJournal') }}</li>
            <li>{{ t('nodesPage.verifyRefresh') }}</li>
            <li>{{ t('nodesPage.verifyNodeDetail') }}</li>
          </ul>
        </div>
      </div>
      <template #footer>
        <el-button @click="onboardingDialogVisible = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>

    <!-- 节点编辑对话框 -->
    <el-dialog
      v-model="editDialogVisible"
      :title="t('nodesPage.editNodeSettings')"
      width="550px"
    >
      <el-form :model="editForm" label-width="140px" ref="editFormRef">
        <el-form-item :label="t('nodeManagement.hostname')">
          <el-input v-model="editForm.hostname" />
        </el-form-item>
        <el-form-item :label="t('nodeManagement.region')">
          <el-input v-model="editForm.region" placeholder="e.g. cn-shanghai" />
        </el-form-item>
        <el-form-item :label="t('nodeManagement.routes')">
          <div class="edit-routes-container">
            <el-tag
              v-for="tag in editForm.advertised_routes"
              :key="tag"
              closable
              :disable-transitions="false"
              @close="handleRemoveRoute(tag)"
              class="route-edit-tag"
            >
              {{ tag }}
            </el-tag>
            <el-input
              v-if="inputVisible"
              ref="InputRef"
              v-model="inputValue"
              class="new-route-input"
              size="small"
              @keyup.enter="handleInputConfirm"
              @blur="handleInputConfirm"
              placeholder="e.g. 192.168.1.0/24"
            />
            <el-button v-else class="button-new-tag" size="small" @click="showInput">
              {{ t('nodesPage.newRoute') }}
            </el-button>
          </div>
          <p class="form-help">{{ t('nodesPage.advertisedRoutesHelp') }}</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button v-if="hasPermission('nodes:write')" type="primary" @click="saveNodeChanges" :loading="submitting">
          {{ t('nodesPage.saveChanges') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, reactive, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import {
  Search,
  Refresh,
  Plus,
  View,
  Edit,
  Delete,
  InfoFilled,
  TrendCharts,
  Upload,
  Download,
  Timer,
  Position,
  Warning,
  Document,
  Operation,
  Connection,
  Key
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import useNodeStore from '../stores/node'
import { useAgentProxyApi } from '../composables/useAgentProxyApi'
import { useMonitorApi } from '../composables/useMonitorApi'
import { useRouteApi } from '../composables/useRouteApi'
import { useTokenApi } from '../composables/useTokenApi'
import { fetchControllerInfo } from '../composables/useControllerInfo'
import { usePermission } from '../composables/usePermission'
import { useTenantChangeReload } from '../composables/useTenantChangeReload'
import { t } from '../i18n'
import ActionIconButton from '../components/ui/ActionIconButton.vue'
import DataPanel from '../components/ui/DataPanel.vue'
import FilterBar from '../components/ui/FilterBar.vue'
import MetricStrip from '../components/ui/MetricStrip.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import StatusBadge from '../components/ui/StatusBadge.vue'
import {
  commandStatusLabel,
  commandStatusTagType,
  isFailedCommandStatus,
  isPendingCommandStatus,
  isTerminalCommandStatus
} from '../utils/controlLoopStatus'

// 使用节点 store
const nodeStore = useNodeStore()
const { hasPermission } = usePermission()
const router = useRouter()

type AnyRecord = Record<string, any>
type TimerHandle = ReturnType<typeof setTimeout>
type InputRefType = { focus?: () => void }

interface EditFormState {
  id: string
  hostname: string
  region: string
  advertised_routes: string[]
}

interface OnboardingFormState {
  tokenTag: string
  maxUses: number
  ttlHours: number
  server: string
  controllerApiUrl: string
  caCertPath: string
  caUrl: string
  caSha256: string
  agentUrl: string
  agentSha256: string
  tlsServerName: string
  region: string
  interface: string
  hostname: string
  advertiseRoutes: string
}

const errorMessage = (error: unknown, fallback = t('policyTerms.unknownError')): string =>
  error instanceof Error ? error.message : (typeof error === 'string' ? error : fallback)

// 节点数据从 store 获取
const nodes = computed<AnyRecord[]>(() => nodeStore.nodes as AnyRecord[])
const loading = computed(() => nodeStore.loading)

const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const detailDialogVisible = ref(false)
const selectedNode = ref<AnyRecord | null>(null)
const commandLoading = ref(false)
const commandPollTimer = ref<TimerHandle | null>(null)
const onboardingDialogVisible = ref(false)
const onboardingCreating = ref(false)
const onboardingToken = ref<AnyRecord | null>(null)
const onboardingControllerInfo = ref<AnyRecord>({})

const currentOrigin = () => {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin
  }
  return 'https://aria.yun'
}

const defaultGrpcServer = () => {
  try {
    const origin = new URL(currentOrigin())
    return `${origin.protocol}//${origin.hostname}:50051`
  } catch {
    return 'https://aria.yun:50051'
  }
}

const inferTLSServerName = (server?: string) => {
  try {
    return new URL(server || defaultGrpcServer()).hostname || 'aria.yun'
  } catch {
    return 'aria.yun'
  }
}

const onboardingForm = reactive<OnboardingFormState>({
  tokenTag: 'node-onboarding',
  maxUses: 1,
  ttlHours: 24,
  server: defaultGrpcServer(),
  controllerApiUrl: currentOrigin(),
  caCertPath: '/etc/aria/certs/ca.crt',
  caUrl: '',
  caSha256: '',
  agentUrl: '',
  agentSha256: '',
  tlsServerName: '',
  region: 'default',
  interface: 'aria0',
  hostname: '',
  advertiseRoutes: ''
})

// 编辑相关状态
const editDialogVisible = ref(false)
const submitting = ref(false)
const editOriginalAdvertisedRoutes = ref<string[]>([])
const editForm = reactive<EditFormState>({
  id: '',
  hostname: '',
  region: '',
  advertised_routes: []
})

// 路由输入相关
const inputVisible = ref(false)
const inputValue = ref('')
const InputRef = ref<InputRefType | null>(null)

// 计算属性
const onlineCount = computed(() => nodes.value.filter(n => n.status === 'online').length)
const offlineCount = computed(() => nodes.value.filter(n => n.status === 'offline').length)
const maintenanceCount = computed(() => nodes.value.filter(n => n.status === 'maintenance').length)
const nodeMetricItems = computed(() => [
  {
    key: 'total',
    label: t('nodesPage.totalNodes'),
    value: nodes.value.length,
    status: 'info',
    meta: t('nodesPage.totalNodesMeta')
  },
  {
    key: 'online',
    label: t('common.online'),
    value: onlineCount.value,
    status: 'success',
    meta: t('nodesPage.onlineMeta')
  },
  {
    key: 'offline',
    label: t('common.offline'),
    value: offlineCount.value,
    status: offlineCount.value > 0 ? 'danger' : 'muted',
    meta: t('nodesPage.offlineMeta')
  },
  {
    key: 'maintenance',
    label: t('common.maintenance'),
    value: maintenanceCount.value,
    status: maintenanceCount.value > 0 ? 'warning' : 'muted',
    meta: t('nodesPage.maintenanceMeta')
  }
])
const recentCommandCount = computed(() => selectedNode.value?.recentCommands?.length || 0)
const recentDeliveryCount = computed(() => selectedNode.value?.recentPolicyDeliveries?.length || 0)
const activeAlertCount = computed(() => selectedNode.value?.activeAlerts?.length || 0)
const failedCommandCount = computed(() => (selectedNode.value?.recentCommands || []).filter((item: AnyRecord) => isFailedCommandStatus(item.status)).length)
const pendingCommandCount = computed(() => (selectedNode.value?.recentCommands || []).filter((item: AnyRecord) => isPendingCommandStatus(item.status)).length)
const failedDeliveryCount = computed(() => (selectedNode.value?.recentPolicyDeliveries || []).filter((item: AnyRecord) => isFailedCommandStatus(item.command_status)).length)
const pendingDeliveryCount = computed(() => (selectedNode.value?.recentPolicyDeliveries || []).filter((item: AnyRecord) => isPendingCommandStatus(item.command_status)).length)
const policyDatapathStats = computed(() => {
  const raw = selectedNode.value?.policyStats || selectedNode.value?.policy_stats || {}
  const acl = raw.acl || {}
  const qos = raw.qos || {}
  return {
    aclPackets: Number(raw.acl_packets ?? acl.packets ?? 0),
    aclDroppedPackets: Number(raw.acl_dropped_packets ?? acl.dropped_packets ?? 0),
    qosPassedBytes: Number(raw.qos_passed_bytes ?? qos.passed_bytes ?? 0),
    qosDroppedBytes: Number(raw.qos_dropped_bytes ?? qos.dropped_bytes ?? 0),
    qosShapedBytes: Number(raw.qos_shaped_bytes ?? qos.shaped_bytes ?? 0)
  }
})
const onboardingTokenValue = computed(() => onboardingToken.value?.token || '')
const onboardingTokenPreview = computed(() => tokenPreview(onboardingTokenValue.value))
const onboardingInstallCommand = computed(() => buildOnboardingInstallCommand())
const onboardingInitCommand = computed(() => buildOnboardingInitCommand())
const recentOnboardingNodes = computed(() => {
  const token = onboardingTokenPreview.value
  const items = nodes.value.filter(node => node.onboarding)
  const matched = token
    ? items.filter(node => node.onboarding?.tokenPreview === token)
    : items
  return matched.slice(0, 6)
})
const isStateDiverged = computed(() => {
  if (!selectedNode.value) return false
  const desired = selectedNode.value.desiredStateVersion
  const applied = selectedNode.value.appliedStateVersion
  return Boolean(desired && applied && desired !== applied)
})
const workbenchSummary = computed(() => {
  const certStatus = selectedNode.value?.certificate?.status || 'missing'
  return [
    {
      key: 'commands',
      label: t('nodeMonitorDetail.commands'),
      value: recentCommandCount.value,
      status: failedCommandCount.value > 0
        ? t('nodeMonitorDetail.failedCount').replace('{count}', String(failedCommandCount.value))
        : t('nodeMonitorDetail.pendingCount').replace('{count}', String(pendingCommandCount.value)),
      type: failedCommandCount.value > 0 ? 'danger' : pendingCommandCount.value > 0 ? 'warning' : 'success',
      focus: 'commands'
    },
    {
      key: 'policies',
      label: t('nodeMonitorDetail.policies'),
      value: recentDeliveryCount.value,
      status: failedDeliveryCount.value > 0
        ? t('nodeMonitorDetail.failedCount').replace('{count}', String(failedDeliveryCount.value))
        : t('nodeMonitorDetail.pendingCount').replace('{count}', String(pendingDeliveryCount.value)),
      type: failedDeliveryCount.value > 0 ? 'danger' : pendingDeliveryCount.value > 0 ? 'warning' : 'success',
      focus: 'policies'
    },
    {
      key: 'alerts',
      label: t('nodeMonitorDetail.activeAlerts'),
      value: activeAlertCount.value,
      status: activeAlertCount.value > 0 ? t('nodeMonitorDetail.active') : t('nodeMonitorDetail.clear'),
      type: activeAlertCount.value > 0 ? 'danger' : 'success',
      focus: 'alerts'
    },
    {
      key: 'certificate',
      label: t('nodeMonitorDetail.certificate'),
      value: certStatus,
      status: selectedNode.value?.certificateActivity?.last_revoke_reason
        ? t('nodeMonitorDetail.revoked')
        : selectedNode.value?.certificateActivity?.last_renew_failure
          ? t('nodeMonitorDetail.renewFailed')
          : certStatus,
      type: getCertificateStatusTagType(certStatus),
      focus: 'certificate'
    }
  ]
})

const filteredNodes = computed(() => {
  if (!searchQuery.value) {
    return nodes.value
  }
  const query = searchQuery.value.toLowerCase()
  return nodes.value.filter(node =>
    String(node.hostname || '').toLowerCase().includes(query) ||
    String(node.publicIp || '').includes(query) ||
    String(node.vpnIp || '').includes(query) ||
    String(node.endpoint || '').includes(query) ||
    String(node.region || '').toLowerCase().includes(query)
  )
})

const paginatedNodes = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredNodes.value.slice(start, end)
})

// 方法
const refreshNodes = async () => {
  await nodeStore.loadNodes()
}

const refreshOnboardingProgress = async () => {
  await refreshNodes()
}

const addNode = async () => {
  onboardingDialogVisible.value = true
  await loadOnboardingControllerInfo()
}

const loadOnboardingControllerInfo = async () => {
  try {
    const info = await fetchControllerInfo()
    onboardingControllerInfo.value = info
    const grpcTLS = info.grpc_tls || {}
    const agent = info.agent || {}
    if (info.controller_api_url) onboardingForm.controllerApiUrl = info.controller_api_url
    if (info.grpc?.server) onboardingForm.server = info.grpc.server
    if (grpcTLS.ca_cert_path) onboardingForm.caCertPath = grpcTLS.ca_cert_path
    if (grpcTLS.ca_cert_url) onboardingForm.caUrl = grpcTLS.ca_cert_url
    if (grpcTLS.ca_cert_sha256) onboardingForm.caSha256 = grpcTLS.ca_cert_sha256
    if (grpcTLS.server_name) onboardingForm.tlsServerName = grpcTLS.server_name
    if (agent.default_interface) onboardingForm.interface = agent.default_interface
    if (agent.default_region) onboardingForm.region = agent.default_region
    if (agent.download_url) onboardingForm.agentUrl = agent.download_url
    if (agent.sha256) onboardingForm.agentSha256 = agent.sha256
  } catch (error) {
    console.warn('Failed to load controller onboarding info:', error)
  }
}

const createOnboardingToken = async () => {
  if (!hasPermission('tokens:write')) {
    ElMessage.error(t('nodesPage.missingTokenPermission'))
    return
  }
  const tag = String(onboardingForm.tokenTag || '').trim()
  if (!tag) {
    ElMessage.error(t('nodesPage.tokenTagRequired'))
    return
  }

  onboardingCreating.value = true
  try {
    onboardingToken.value = await useTokenApi.createToken({
      tag,
      max_uses: Number(onboardingForm.maxUses || 1),
      ttl: `${Number(onboardingForm.ttlHours || 24)}h`
    })
    ElMessage.success(t('nodesPage.tokenCreated'))
  } catch (error) {
    console.error('Failed to create enrollment token:', error)
    ElMessage.error(t('nodesPage.createTokenFailed').replace('{message}', errorMessage(error, String(error || t('policyTerms.unknownError')))))
  } finally {
    onboardingCreating.value = false
  }
}

const shellArg = (value: unknown) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  if (/^[A-Za-z0-9_./:@,+-]+$/.test(raw)) return raw
  return `'${raw.replace(/'/g, `'\\''`)}'`
}

const buildOnboardingInitCommand = () => {
  const token = onboardingTokenValue.value || '<enrollment-token>'
  const parts = [
    'sudo',
    'aria-agent',
    'init',
    '--server',
    shellArg(onboardingForm.server || defaultGrpcServer()),
    '--token',
    shellArg(token),
    '--controller-api-url',
    shellArg(onboardingForm.controllerApiUrl || currentOrigin())
  ]
  const optionalArgs = [
    ['--ca-cert', onboardingForm.caCertPath],
    ['--client-cert', '/etc/aria/certs/agent.crt'],
    ['--client-key', '/etc/aria/certs/agent.key'],
    ['--tls-server-name', onboardingForm.tlsServerName || inferTLSServerName(onboardingForm.server)],
    ['--region', onboardingForm.region],
    ['--interface', onboardingForm.interface],
    ['--hostname', onboardingForm.hostname],
    ['--advertise-routes', onboardingForm.advertiseRoutes]
  ]
  optionalArgs.forEach(([flag, value]) => {
    const arg = shellArg(value)
    if (arg) {
      parts.push(flag, arg)
    }
  })
  return parts.join(' ')
}

const buildOnboardingInstallCommand = () => {
  const token = onboardingTokenValue.value || '<enrollment-token>'
  const controllerApiUrl = onboardingForm.controllerApiUrl || currentOrigin()
  const installScriptUrl = `${String(controllerApiUrl).replace(/\/+$/, '')}/api/v2/install/agent.sh`
  const parts = [
    'curl',
    '-fsSL',
    shellArg(installScriptUrl),
    '|',
    'sudo',
    'bash',
    '-s',
    '--',
    '--controller-api-url',
    shellArg(controllerApiUrl),
    '--server',
    shellArg(onboardingForm.server || defaultGrpcServer()),
    '--token',
    shellArg(token)
  ]
  const optionalArgs = [
    ['--ca-url', onboardingForm.caUrl],
    ['--ca-sha256', onboardingForm.caSha256],
    ['--client-cert', '/etc/aria/certs/agent.crt'],
    ['--client-key', '/etc/aria/certs/agent.key'],
    ['--tls-server-name', onboardingForm.tlsServerName || inferTLSServerName(onboardingForm.server)],
    ['--region', onboardingForm.region],
    ['--interface', onboardingForm.interface],
    ['--hostname', onboardingForm.hostname],
    ['--agent-url', onboardingForm.agentUrl],
    ['--agent-sha256', onboardingForm.agentSha256],
    ['--public-ip', 'auto'],
    ['--public-endpoint', 'auto']
  ]
  optionalArgs.forEach(([flag, value]) => {
    const arg = shellArg(value)
    if (arg) {
      parts.push(flag, arg)
    }
  })
  return parts.join(' ')
}

const copyText = async (value: string, successMessage: string) => {
  if (!value) {
    ElMessage.warning(t('nodesPage.nothingToCopy'))
    return
  }
  if (!navigator?.clipboard?.writeText) {
    ElMessage.error(t('nodesPage.clipboardUnavailable'))
    return
  }
  await navigator.clipboard.writeText(value)
  ElMessage.success(successMessage)
}

const copyOnboardingToken = () => copyText(onboardingTokenValue.value, t('nodesPage.tokenCopied'))

const copyOnboardingCommand = () => copyText(onboardingInstallCommand.value, t('nodesPage.installCommandCopied'))

const copyOnboardingInitCommand = () => copyText(onboardingInitCommand.value, t('nodesPage.initCommandCopied'))

const tokenPreview = (value: unknown) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  if (raw.length <= 10) return 'redacted'
  return `${raw.slice(0, 6)}...${raw.slice(-4)}`
}

const getOnboardingPhaseTagType = (phase?: string) => {
  switch (phase) {
    case 'online':
      return 'success'
    case 'syncing':
      return 'warning'
    case 'degraded':
      return 'danger'
    case 'registered':
    default:
      return 'info'
  }
}

const formatOnboardingPhase = (phase?: string) => {
  switch (phase) {
    case 'online':
      return t('nodesPage.onboardingOnline')
    case 'syncing':
      return t('nodesPage.onboardingSyncing')
    case 'degraded':
      return t('nodesPage.onboardingDegraded')
    case 'registered':
    default:
      return t('nodesPage.onboardingRegistered')
  }
}

const onboardingTroubleshootingCommand = (node: AnyRecord) => {
  const configPath = onboardingControllerInfo.value?.agent?.config_path || '/etc/aria/agent.yaml'
  const lines = [
    `sudo aria-agent doctor --config ${configPath}`,
    'sudo systemctl status aria-agent --no-pager',
    'sudo journalctl -u aria-agent -n 120 --no-pager'
  ]
  if (node?.hostname) {
    lines.unshift(`# ${node.hostname}`)
  }
  return lines.join('\n')
}

const openNodeDetailFromOnboarding = async (node: AnyRecord) => {
  onboardingDialogVisible.value = false
  await viewNodeDetails(node)
}

const viewNodeDetails = async (node: AnyRecord) => {
  try {
    selectedNode.value = await nodeStore.loadNodeDetail(node.id)
    // Fetch real bandwidth/latency metrics
    try {
      const metrics = await useMonitorApi.getNodeMetrics(node.id)
      if (metrics && selectedNode.value) {
        selectedNode.value.bandwidth = {
          upload: (metrics as AnyRecord).upload_mbps != null ? Number((metrics as AnyRecord).upload_mbps.toFixed(2)) : 0,
          download: (metrics as AnyRecord).download_mbps != null ? Number((metrics as AnyRecord).download_mbps.toFixed(2)) : 0
        }
        selectedNode.value.latency = metrics.latency_ms != null ? Number(metrics.latency_ms.toFixed(1)) : 0
      }
    } catch (metricsError) {
      console.error('Failed to load node metrics:', metricsError)
    }
    detailDialogVisible.value = true
  } catch (error) {
    console.error('Failed to load node detail:', error)
    ElMessage.error(t('nodesPage.loadNodeDetailsFailed'))
  }
}

// 编辑逻辑
const handleEditNode = (node: AnyRecord) => {
  editForm.id = node.id
  editForm.hostname = node.hostname
  editForm.region = node.region
  // 确保是数组拷贝
  editForm.advertised_routes = Array.isArray(node.routes) ? [...node.routes] : []
  editOriginalAdvertisedRoutes.value = [...editForm.advertised_routes]
  editDialogVisible.value = true
}

const handleRemoveRoute = (tag: string) => {
  editForm.advertised_routes.splice(editForm.advertised_routes.indexOf(tag), 1)
}

const showInput = () => {
  inputVisible.value = true
  nextTick(() => {
    InputRef.value?.focus?.()
  })
}

const handleInputConfirm = (): boolean => {
  const pendingRoute = inputValue.value.trim()
  if (pendingRoute) {
    if (!/^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/.test(pendingRoute)) {
      ElMessage.warning(t('nodesPage.invalidCidr'))
      return false
    }
    if (!editForm.advertised_routes.includes(pendingRoute)) {
      editForm.advertised_routes.push(pendingRoute)
    }
  }
  inputVisible.value = false
  inputValue.value = ''
  return true
}

const normalizeEditedRoutes = (routes: string[]): string[] => {
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const route of routes) {
    const trimmed = String(route || '').trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    normalized.push(trimmed)
  }
  return normalized
}

const syncEditedAdvertisedRoutes = async () => {
  const original = normalizeEditedRoutes(editOriginalAdvertisedRoutes.value)
  const next = normalizeEditedRoutes(editForm.advertised_routes)
  const originalSet = new Set(original)
  const nextSet = new Set(next)
  const routesToAdd = next.filter((route) => !originalSet.has(route))
  const routesToDelete = original.filter((route) => !nextSet.has(route))

  for (const route of routesToAdd) {
    await useRouteApi.addRoute(editForm.id, route)
  }
  for (const route of routesToDelete) {
    await useRouteApi.deleteRoute(editForm.id, route)
  }
}

const saveNodeChanges = async () => {
  if (!handleInputConfirm()) {
    return
  }
  submitting.value = true
  try {
    await nodeStore.updateNodeRemote(editForm.id, {
      hostname: editForm.hostname,
      region: editForm.region
    })
    await syncEditedAdvertisedRoutes()
    ElMessage.success(t('nodesPage.nodeUpdated'))
    editDialogVisible.value = false
    await refreshNodes()
  } catch (error) {
    ElMessage.error(t('nodesPage.updateNodeFailed'))
  } finally {
    submitting.value = false
  }
}

const handleDeleteNode = async (id: string) => {
  try {
    await nodeStore.deleteNodeRemote(id)
    ElMessage.success(t('nodesPage.nodeDeleted'))
  } catch (error) {
    ElMessage.error(t('nodesPage.deleteNodeFailed'))
  }
}

const closeDetailDialog = () => {
  stopCommandPolling()
  detailDialogVisible.value = false
  selectedNode.value = null
}

const normalizeContextRecord = (context: unknown): AnyRecord => {
  if (!context || typeof context !== 'object' || Array.isArray(context)) {
    return {}
  }
  return context as AnyRecord
}

const buildMonitoringQuery = (focus = '', context: AnyRecord = {}) => {
  const query: Record<string, string> = {}
  if (focus) query.focus = focus
  if (context.commandId) query.commandId = String(context.commandId)
  if (context.alertId) query.alertId = String(context.alertId)
  if (context.eventType) query.eventType = String(context.eventType)
  if (context.policyRef) query.policyRef = String(context.policyRef)
  if (context.policyDomain) query.policyDomain = String(context.policyDomain)
  return query
}

const openMonitoringDetail = (focus = '', context: AnyRecord = {}) => {
  if (!selectedNode.value?.id) return
  const query = buildMonitoringQuery(focus, context)
  detailDialogVisible.value = false
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: selectedNode.value.id },
    ...(Object.keys(query).length > 0 ? { query } : {})
  })
}

const openMonitoringCommand = (command: AnyRecord = {}) => {
  openMonitoringDetail('commands', {
    commandId: command.id
  })
}

const monitoringFocusForAlert = (alert: AnyRecord = {}) => {
  const context = normalizeContextRecord(alert.context)
  if (context.command_id) return 'commands'
  if (context.policy_ref) return 'policies'
  if (String(alert.alert_type || '').startsWith('certificate_')) return 'certificate'
  return 'alerts'
}

const openMonitoringAlert = (alert: AnyRecord = {}) => {
  const context = normalizeContextRecord(alert.context)
  openMonitoringDetail(monitoringFocusForAlert(alert), {
    alertId: alert.id,
    eventType: alert.alert_type,
    commandId: context.command_id,
    policyRef: context.policy_ref,
    policyDomain: context.policy_domain
  })
}

const openPolicyCenter = (delivery: AnyRecord | null = null) => {
  if (!selectedNode.value?.id) return
  detailDialogVisible.value = false
  router.push({
    name: 'Policies',
    query: {
      nodeId: selectedNode.value.id,
      ...(delivery?.policy_ref ? { policyRef: delivery.policy_ref } : {}),
      ...(delivery?.policy_domain ? { kind: delivery.policy_domain } : {}),
      ...(delivery?.command_id ? { commandId: delivery.command_id } : {})
    }
  })
}

const handleSummaryClick = (focus: string) => {
  if (focus === 'policies') {
    openPolicyCenter()
    return
  }
  openMonitoringDetail(focus)
}

const handleSizeChange = (size: number) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page: number) => {
  currentPage.value = page
}

const reloadSelectedNode = async (preserveCommand: AnyRecord | null = null) => {
  if (!selectedNode.value?.id) return
  selectedNode.value = await nodeStore.loadNodeDetail(selectedNode.value.id)
  if (preserveCommand) {
    prependCommandIfMissing(preserveCommand)
  }
}

const findRecentCommand = (commandId?: string) => {
  if (!commandId || !selectedNode.value?.recentCommands) return null
  return selectedNode.value.recentCommands.find((item: AnyRecord) => item.id === commandId) || null
}

const stopCommandPolling = () => {
  if (commandPollTimer.value) {
    clearTimeout(commandPollTimer.value)
    commandPollTimer.value = null
  }
}

const pollCommandStatus = (command: AnyRecord, attemptsRemaining = 15) => {
  stopCommandPolling()
  if (!command?.id || attemptsRemaining <= 0 || !detailDialogVisible.value) return

  commandPollTimer.value = setTimeout(async () => {
    commandPollTimer.value = null
    if (!selectedNode.value?.id || !detailDialogVisible.value) return

    try {
      await reloadSelectedNode(command)
      const latest = findRecentCommand(command.id)
      if (!latest || !isTerminalCommandStatus(latest.status)) {
        pollCommandStatus(command, attemptsRemaining - 1)
      }
    } catch (error) {
      console.error('Failed to poll command status:', error)
      pollCommandStatus(command, attemptsRemaining - 1)
    }
  }, 2000)
}

const normalizeQueuedCommand = (command: string, response: AnyRecord = {}) => ({
  id: response?.command_id || response?.id || '',
  command: response?.command || command,
  status: response?.status || 'pending',
  message: response?.message || t('nodesPage.commandQueuedForDelivery'),
  created_at: response?.created_at || new Date().toISOString(),
  updated_at: response?.updated_at || response?.created_at || new Date().toISOString(),
  timeout_seconds: response?.timeout_seconds,
  priority: response?.priority
})

const prependCommandIfMissing = (command: AnyRecord) => {
  if (!selectedNode.value || !command?.id) return
  const existing = Array.isArray(selectedNode.value.recentCommands) ? selectedNode.value.recentCommands : []
  if (existing.some(item => item.id === command.id)) {
    selectedNode.value.recentCommands = existing
    return
  }
  selectedNode.value.recentCommands = [command, ...existing]
}

const runQuickCommand = async (command: string) => {
  if (!selectedNode.value?.id) return
  if (!hasPermission('commands:write')) {
    ElMessage.error(t('nodesPage.missingCommandPermission'))
    return
  }

  commandLoading.value = true
  try {
    const response = await useAgentProxyApi.sendAgentCommand(selectedNode.value.id, {
      command,
      params: {},
      timeout: 30
    } as any)
    const queuedCommand = normalizeQueuedCommand(command, response)
    prependCommandIfMissing(queuedCommand)
    ElMessage.success(t('nodesPage.commandQueued').replace('{command}', command))
    await reloadSelectedNode(queuedCommand)
    const latest = findRecentCommand(queuedCommand.id)
    if (!latest || !isTerminalCommandStatus(latest.status)) {
      pollCommandStatus(queuedCommand)
    }
  } catch (error) {
    console.error(`Failed to queue ${command}:`, error)
    ElMessage.error(t('nodesPage.commandQueueFailed').replace('{command}', command))
  } finally {
    commandLoading.value = false
  }
}

const formatConvergence = (state?: string) => {
  const map: Record<string, string> = {
    converged: t('policies.converged'),
    pending: t('policies.pending'),
    diverged: t('policies.diverged'),
    idle: t('policies.idle')
  }
  return map[state || ''] || state || t('status.unknown')
}

const getConvergenceTagType = (state?: string) => {
  switch (state) {
    case 'converged': return 'success'
    case 'pending': return 'warning'
    case 'diverged': return 'danger'
    default: return 'info'
  }
}

const getAlertSeverityTagType = (severity?: string) => {
  switch (severity) {
    case 'critical':
    case 'error':
      return 'danger'
    case 'warning':
      return 'warning'
    case 'info':
      return 'info'
    default:
      return 'info'
  }
}

const getCertificateStatusTagType = (status?: string) => {
  switch (status) {
    case 'issued':
      return 'success'
    case 'expiring':
      return 'warning'
    case 'expired':
    case 'revoked':
      return 'danger'
    default:
      return 'info'
  }
}

const shortStateVersion = (value?: string) => {
  if (!value) return 'N/A'
  return value.length > 12 ? `${value.slice(0, 12)}...` : value
}

const shortCommandId = (value?: string) => {
  if (!value) return '-'
  return String(value).length > 10 ? `${String(value).slice(0, 10)}...` : String(value)
}

const formatLargeNumber = (value: unknown) => {
  const number = Number(value || 0)
  if (number >= 1000000) return `${(number / 1000000).toFixed(1)}M`
  if (number >= 1000) return `${(number / 1000).toFixed(1)}K`
  return String(number)
}

const formatMetricBytes = (value: unknown) => {
  const bytes = Number(value || 0)
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

const formatCommandTime = (value?: string) => {
  if (!value) return 'N/A'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'N/A'
  return date.toLocaleString()
}

const reloadTenantScopedData = async () => {
  currentPage.value = 1
  selectedNode.value = null
  detailDialogVisible.value = false
  editDialogVisible.value = false
  await refreshNodes()
}

onMounted(() => {
  refreshNodes()
})

onBeforeUnmount(() => {
  stopCommandPolling()
})

useTenantChangeReload(reloadTenantScopedData)
</script>

<style scoped>
/* ============================================
   Nodes Page Styles
   ============================================ */
.nodes-page { display: flex; flex-direction: column; gap: 18px; }
.detail-toolbar { display: flex; gap: 12px; }
.section-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.section-header .section-title { margin-bottom: 0; }

/* Edit Form Styles */
.edit-routes-container {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  border: 1px solid var(--el-border-color);
  padding: 8px;
  border-radius: 4px;
  min-height: 40px;
}
.route-edit-tag { margin: 2px; }
.new-route-input { width: 140px; margin: 2px; vertical-align: middle; }
.button-new-tag { margin: 2px; height: 24px; line-height: 22px; padding-top: 0; padding-bottom: 0; }
.form-help { font-size: 12px; color: var(--aria-text-muted); margin-top: 4px; line-height: 1.4; }

.search-input { width: 280px; }

.nodes-card { background: var(--aria-bg-secondary); }
.region-badge { display: inline-block; padding: 4px 10px; background: rgba(59, 130, 246, 0.1); color: #3B82F6; border-radius: var(--radius-full); font-size: 12px; font-weight: 600; }
.status-badge { display: inline-flex; align-items: center; gap: 6px; padding: 4px 12px; border-radius: var(--radius-full); font-size: 12px; font-weight: 500; }
.status-badge::before { content: ''; width: 6px; height: 6px; border-radius: 50%; }
.status-badge.online { background: rgba(34, 197, 94, 0.15); color: #22C55E; }
.status-badge.online::before { background: #22C55E; animation: pulse-dot 2s ease-in-out infinite; }
.status-badge.offline { background: rgba(239, 68, 68, 0.15); color: #EF4444; }
.status-badge.offline::before { background: #EF4444; }

.state-version-cell { display: flex; flex-direction: column; gap: 2px; font-family: monospace; font-size: 11px; }
.version-mismatch .desired { color: var(--el-color-warning); font-weight: 600; }
.muted-line { color: var(--aria-text-secondary); opacity: 0.7; }
.action-buttons { display: flex; align-items: center; gap: 4px; }
.pagination-wrapper { display: flex; justify-content: flex-end; padding-top: 20px; border-top: 1px solid var(--aria-border-primary); }

.node-detail-content { max-height: 600px; overflow-y: auto; }
.detail-section { margin-bottom: 32px; }
.section-title { display: flex; align-items: center; gap: 8px; font-size: 16px; font-weight: 600; color: var(--aria-text-primary); margin: 0 0 16px 0; }
.workbench-summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.workbench-summary-item { min-height: 92px; padding: 14px; border: 1px solid var(--aria-border-primary); border-radius: var(--aria-radius-md); background: var(--aria-bg-tertiary); color: inherit; cursor: pointer; display: flex; flex-direction: column; align-items: flex-start; justify-content: space-between; text-align: left; transition: border-color var(--aria-transition-base), background var(--aria-transition-base); }
.workbench-summary-item:hover { border-color: var(--aria-primary); background: var(--aria-bg-secondary); }
.summary-label { font-size: 12px; color: var(--aria-text-secondary); text-transform: uppercase; letter-spacing: 0; }
.summary-value { font-size: 20px; font-weight: 700; color: var(--aria-text-primary); word-break: break-word; }
.control-state-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.control-state-item { min-height: 94px; padding: 14px; background: var(--aria-bg-tertiary); border: 1px solid var(--aria-border-primary); border-radius: var(--aria-radius-md); }
.control-state-label { font-size: 12px; color: var(--aria-text-secondary); text-transform: uppercase; letter-spacing: 0; margin-bottom: 8px; }
.control-state-value { font-size: 14px; font-weight: 700; color: var(--aria-text-primary); word-break: break-all; }
.control-state-value.danger { color: #EF4444; }
.control-state-time { margin-top: 8px; font-size: 12px; color: var(--aria-text-secondary); word-break: break-word; }
.state-alert { margin-top: 12px; }
.diagnostic-command {
  margin-top: 12px;
  padding: 12px;
  background: var(--aria-bg-tertiary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-md);
  color: var(--aria-text-primary);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
}
.certificate-note { margin-top: 10px; color: var(--aria-text-secondary); font-size: 12px; }
.mono-text { font-family: Menlo, Monaco, Consolas, "Courier New", monospace; font-size: 12px; }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; }
.stat-box { display: flex; align-items: center; gap: 16px; padding: 20px; background: var(--aria-bg-tertiary); border: 1px solid var(--aria-border-primary); border-radius: var(--aria-radius-lg); }
.stat-icon-box { width: 48px; height: 48px; border-radius: var(--aria-radius-md); display: flex; align-items: center; justify-content: center; font-size: 24px; flex-shrink: 0; }
.stat-icon-box.upload { background: rgba(59, 130, 246, 0.15); color: #3B82F6; }
.stat-icon-box.download { background: rgba(34, 197, 94, 0.15); color: #22C55E; }
.stat-icon-box.latency { background: rgba(245, 158, 11, 0.15); color: #F59E0B; }
.stat-icon-box.policies { background: rgba(139, 92, 246, 0.15); color: #8B5CF6; }
.stat-icon-box.qos { background: rgba(20, 184, 166, 0.15); color: #14B8A6; }
.stat-box-value { font-size: 20px; font-weight: 700; color: var(--aria-text-primary); }
.stat-box-label { font-size: 12px; color: var(--aria-text-secondary); }
.routes-list { display: flex; flex-wrap: wrap; gap: 10px; }
.route-tag { font-family: monospace; }
.route-code { color: #cf9236; }
.onboarding-section-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.onboarding-section-header h4 { margin: 0; }

@keyframes pulse-dot { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
@media (max-width: 960px) { .workbench-summary-grid, .control-state-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 768px) { .search-input { width: 100%; } .stats-grid { grid-template-columns: 1fr; } .detail-toolbar { flex-wrap: wrap; } }
@media (max-width: 520px) { .workbench-summary-grid, .control-state-grid { grid-template-columns: 1fr; } }
</style>

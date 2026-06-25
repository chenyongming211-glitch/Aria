<!-- src/views/Nodes.vue - 现代化节点管理页面 -->
<template>
  <div class="nodes-page page-shell">
    <!-- 页面头部 -->
    <section class="page-hero">
      <div class="page-hero-main">
        <div class="page-eyebrow">Fleet Inventory</div>
        <h1 class="page-heading">Node Management</h1>
        <p class="page-description">Monitor registration, runtime health, desired state convergence, and node-level operations.</p>
      </div>
      <div class="page-actions">
        <el-input
          v-model="searchQuery"
          placeholder="Search nodes..."
          class="search-input"
          clearable
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-button
          :icon="Refresh"
          @click="refreshNodes"
          :loading="loading"
        >
          Refresh
        </el-button>
        <el-button v-if="hasPermission('nodes:write')" type="primary" :icon="Plus" @click="addNode">
          Add Node
        </el-button>
      </div>
    </section>

    <!-- 统计卡片 -->
    <div class="stats-cards kpi-grid">
      <div class="stat-item kpi-card">
        <div class="stat-icon blue">
          <el-icon><Monitor /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ nodes.length }}</div>
          <div class="stat-label">Total Nodes</div>
        </div>
      </div>
      <div class="stat-item kpi-card">
        <div class="stat-icon green">
          <el-icon><CircleCheck /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ onlineCount }}</div>
          <div class="stat-label">Online</div>
        </div>
      </div>
      <div class="stat-item kpi-card">
        <div class="stat-icon orange">
          <el-icon><CircleClose /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ offlineCount }}</div>
          <div class="stat-label">Offline</div>
        </div>
      </div>
      <div class="stat-item kpi-card">
        <div class="stat-icon purple">
          <el-icon><Setting /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ maintenanceCount }}</div>
          <div class="stat-label">Maintenance</div>
        </div>
      </div>
    </div>

    <!-- 节点列表卡片 -->
    <el-card class="nodes-card table-card" shadow="never">
      <el-table
        :data="paginatedNodes"
        stripe
        class="nodes-table"
        v-loading="loading"
      >
        <el-table-column prop="hostname" label="Hostname" min-width="140" />
        <el-table-column prop="publicIp" label="Public IP" width="130" />
        <el-table-column prop="vpnIp" label="VPN IP" width="120" />
        <el-table-column prop="region" label="Region" width="80">
          <template #default="{ row }">
            <span class="region-badge">{{ row.region.toUpperCase() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="Version" width="100" />
        <el-table-column prop="mode" label="Mode" width="100">
          <template #default="{ row }">
            <div class="mode-badge">
              {{ row.mode }}
              <el-tag
                v-if="row.mode === 'kernel'"
                size="small"
                type="success"
                effect="plain"
              >
                Opt
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Status" width="100">
          <template #default="{ row }">
            <span class="status-badge" :class="row.status">
              {{ row.status }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="Onboarding" width="130">
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
        <el-table-column label="Sync" width="120">
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
        <el-table-column label="Desired / Applied" width="180">
          <template #default="{ row }">
            <div class="state-version-cell" :class="{ 'version-mismatch': row.desiredStateVersion !== row.appliedStateVersion }">
              <div class="version-line desired">
                <el-tooltip content="Desired State Version" placement="left">
                  <span>{{ shortStateVersion(row.desiredStateVersion) }}</span>
                </el-tooltip>
              </div>
              <div class="version-line applied muted-line">
                <el-tooltip content="Applied State Version" placement="left">
                  <span>{{ shortStateVersion(row.appliedStateVersion) }}</span>
                </el-tooltip>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="pendingCmds" label="Pending" width="90" />
        <el-table-column prop="lastSeen" label="Last Seen" width="150" />
        <el-table-column label="Actions" width="180" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button
                size="small"
                link
                @click="viewNodeDetails(row)"
              >
                <el-icon><View /></el-icon>
              </el-button>
              <el-button
                v-if="hasPermission('nodes:write')"
                size="small"
                link
                type="primary"
                @click="handleEditNode(row)"
              >
                <el-icon><Edit /></el-icon>
              </el-button>
              <el-popconfirm
                v-if="hasPermission('nodes:write')"
                title="Are you sure to delete this node?"
                @confirm="handleDeleteNode(row.id)"
              >
                <template #reference>
                  <el-button
                    size="small"
                    link
                    type="danger"
                  >
                    <el-icon><Delete /></el-icon>
                  </el-button>
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
    </el-card>

    <!-- 节点详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      title="Node Details"
      width="800px"
      custom-class="node-detail-dialog"
      @closed="closeDetailDialog"
    >
      <div v-if="selectedNode" class="node-detail-content">
        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Operation /></el-icon>
              Operations Summary
            </h4>
            <el-button size="small" @click="openMonitoringDetail('commands')">
              Monitoring Context
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
            Onboarding Evidence
          </h4>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Phase">
              <el-tag size="small" :type="getOnboardingPhaseTagType(selectedNode.onboarding?.phase)">
                {{ formatOnboardingPhase(selectedNode.onboarding?.phase) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Token">
              {{ selectedNode.onboarding?.tokenPreview || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="First Seen">
              {{ selectedNode.onboarding?.firstSeenAt || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Last Sync">
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
            Basic Information
          </h4>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Hostname">{{ selectedNode.hostname }}</el-descriptions-item>
            <el-descriptions-item label="Region">
              <span class="region-badge">{{ selectedNode.region.toUpperCase() }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="Public IP">{{ selectedNode.publicIp }}</el-descriptions-item>
            <el-descriptions-item label="VPN IP">{{ selectedNode.vpnIp }}</el-descriptions-item>
            <el-descriptions-item label="Endpoint">{{ selectedNode.endpoint }}</el-descriptions-item>
            <el-descriptions-item label="Status">
              <span class="status-badge" :class="selectedNode.status">
                {{ selectedNode.status }}
              </span>
            </el-descriptions-item>
            <el-descriptions-item label="Uptime">{{ selectedNode.uptime }}</el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Connection /></el-icon>
            Control State
          </h4>
          <div class="control-state-grid">
            <div class="control-state-item">
              <div class="control-state-label">Desired Version</div>
              <div class="control-state-value" :class="{ danger: isStateDiverged }">
                {{ selectedNode.desiredStateVersion || 'N/A' }}
              </div>
              <div class="control-state-time">{{ selectedNode.desiredStateUpdatedAt || 'N/A' }}</div>
            </div>
            <div class="control-state-item">
              <div class="control-state-label">Applied Version</div>
              <div class="control-state-value" :class="{ danger: isStateDiverged }">
                {{ selectedNode.appliedStateVersion || 'N/A' }}
              </div>
              <div class="control-state-time">{{ selectedNode.appliedStateUpdatedAt || 'N/A' }}</div>
            </div>
            <div class="control-state-item">
              <div class="control-state-label">Observed State</div>
              <div class="control-state-value">{{ selectedNode.observedState || 'idle' }}</div>
              <div class="control-state-time">{{ selectedNode.observedAt || 'N/A' }}</div>
            </div>
            <div class="control-state-item">
              <div class="control-state-label">Convergence</div>
              <div class="control-state-value">
                <el-tag size="small" :type="getConvergenceTagType(selectedNode.stateConvergence)">
                  {{ formatConvergence(selectedNode.stateConvergence) }}
                </el-tag>
              </div>
              <div class="control-state-time">Last sync: {{ selectedNode.lastSyncAt || 'N/A' }}</div>
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
              Certificate Status
            </h4>
            <el-button size="small" @click="openMonitoringDetail('certificate')">
              Certificate Context
            </el-button>
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Status">
              <el-tag size="small" :type="getCertificateStatusTagType(selectedNode.certificate?.status)">
                {{ selectedNode.certificate?.status || 'missing' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Serial Number">
              {{ selectedNode.certificate?.serial_number || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Issued At">
              {{ formatCommandTime(selectedNode.certificate?.issued_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Expires At">
              {{ formatCommandTime(selectedNode.certificate?.not_after) }}
            </el-descriptions-item>
            <el-descriptions-item label="Last Renewed">
              {{ formatCommandTime(selectedNode.certificateActivity?.last_renewed_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Renew Failure">
              {{ selectedNode.certificateActivity?.last_renew_failure || 'N/A' }}
            </el-descriptions-item>
          </el-descriptions>
          <div v-if="selectedNode.certificateActivity?.last_renewed_serial_number" class="certificate-note">
            Last renewed serial: {{ selectedNode.certificateActivity.last_renewed_serial_number }}
          </div>
        </div>

        <!-- 实时监控指标 -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><TrendCharts /></el-icon>
            Real-time Metrics
          </h4>
          <div class="stats-grid">
            <div class="stat-box">
              <div class="stat-icon-box upload">
                <el-icon><Upload /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.bandwidth?.upload || 0 }} Mbps</div>
                <div class="stat-box-label">Upload</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box download">
                <el-icon><Download /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.bandwidth?.download || 0 }} Mbps</div>
                <div class="stat-box-label">Download</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box latency">
                <el-icon><Timer /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.latency || 0 }} ms</div>
                <div class="stat-box-label">Latency</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box policies">
                <el-icon><Operation /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ formatLargeNumber(policyDatapathStats.aclPackets) }}</div>
                <div class="stat-box-label">ACL Packets</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box qos">
                <el-icon><TrendCharts /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ formatMetricBytes(policyDatapathStats.qosPassedBytes) }}</div>
                <div class="stat-box-label">QoS Passed</div>
              </div>
            </div>
          </div>
        </div>

        <!-- 路由信息 (Site-to-Site) -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Upload /></el-icon>
            Advertised Routes (Site-to-Site)
          </h4>
          <div class="routes-list">
            <el-empty v-if="!selectedNode.routes || selectedNode.routes.length === 0" :image-size="40" description="No advertised routes" />
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
            Learned Routes (Mesh)
          </h4>
          <el-table
            :data="selectedNode.learnedRoutes || []"
            size="small"
            empty-text="No routes learned from peers"
            class="learned-routes-table"
          >
            <el-table-column prop="cidr" label="CIDR" width="150">
              <template #default="{ row }">
                <code class="route-code">{{ row.cidr }}</code>
              </template>
            </el-table-column>
            <el-table-column prop="next_hop_node" label="Next Hop Node" min-width="120" />
            <el-table-column prop="next_hop_ip" label="VPN IP" width="120" />
            <el-table-column prop="region" label="Region" width="100">
              <template #default="{ row }">
                <span class="region-badge">{{ row.region.toUpperCase() }}</span>
              </template>
            </el-table-column>
            <el-table-column label="Status" width="100">
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
              Recent Commands
            </h4>
            <div class="detail-toolbar">
              <el-button size="small" @click="openMonitoringDetail('commands')">
                Monitoring Detail
              </el-button>
              <el-button size="small" @click="openPolicyCenter()">
                Policy Center
              </el-button>
              <el-button
                v-if="hasPermission('commands:write')"
                size="small"
                type="primary"
                :loading="commandLoading"
                @click="runQuickCommand('sync')"
              >
                Force Sync
              </el-button>
              <el-button
                v-if="hasPermission('commands:write')"
                size="small"
                :loading="commandLoading"
                @click="runQuickCommand('health_check')"
              >
                Health Check
              </el-button>
            </div>
          </div>
          <el-table
            :data="selectedNode.recentCommands || []"
            size="small"
            empty-text="No commands yet"
          >
            <el-table-column label="ID" width="110">
              <template #default="{ row }">
                <el-tooltip v-if="row.id" :content="row.id" placement="top">
                  <span class="mono-text">{{ shortCommandId(row.id) }}</span>
                </el-tooltip>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="command" label="Command" min-width="120" />
            <el-table-column prop="status" label="Status" width="120">
              <template #default="{ row }">
                <el-tag :type="getCommandStatusTagType(row.status)" size="small">
                  {{ row.status }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="message" label="Message" min-width="220" show-overflow-tooltip />
            <el-table-column label="Created" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column label="Completed" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.completed_at || row.updated_at) }}
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Warning /></el-icon>
              Active Alerts
            </h4>
            <el-button size="small" @click="openMonitoringDetail('alerts')">
              Open Monitoring
            </el-button>
          </div>
          <el-table
            :data="selectedNode.activeAlerts || []"
            size="small"
            empty-text="No active alerts"
          >
            <el-table-column prop="severity" label="Severity" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="getAlertSeverityTagType(row.severity)">
                  {{ row.severity || 'info' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="alert_type" label="Type" width="150" />
            <el-table-column prop="title" label="Title" min-width="180" show-overflow-tooltip />
            <el-table-column prop="message" label="Message" min-width="220" show-overflow-tooltip />
            <el-table-column label="Created" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="detail-section">
          <div class="section-header">
            <h4 class="section-title">
              <el-icon><Document /></el-icon>
              Recent Policy Deliveries
            </h4>
            <el-button size="small" @click="openPolicyCenter()">
              Open Policy Center
            </el-button>
          </div>
          <el-table
            :data="selectedNode.recentPolicyDeliveries || []"
            size="small"
            empty-text="No recent policy deliveries"
          >
            <el-table-column prop="policy_domain" label="Domain" width="120" />
            <el-table-column label="Policy" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">
                {{ row.policy_name || row.policy_ref || '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="policy_ref" label="Ref" width="150" show-overflow-tooltip />
            <el-table-column label="Command" width="120">
              <template #default="{ row }">
                <el-tooltip v-if="row.command_id" :content="row.command_id" placement="top">
                  <span class="mono-text">{{ shortCommandId(row.command_id) }}</span>
                </el-tooltip>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="command_status" label="Status" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="getDeliveryStatusTagType(row.command_status)">
                  {{ row.command_status || 'unknown' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Updated" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.updated_at || row.completed_at || row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column prop="last_error" label="Error" min-width="220" show-overflow-tooltip />
            <el-table-column label="Actions" width="120">
              <template #default="{ row }">
                <el-button size="small" link @click="openPolicyCenter(row)">
                  Open
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
      title="Onboard New Node"
      width="760px"
      custom-class="node-onboarding-dialog"
    >
      <div class="onboarding-flow">
        <el-alert
          title="Create an enrollment token, copy the install command, run it on the target machine, then watch the node come online."
          type="info"
          show-icon
          :closable="false"
        />

        <div class="onboarding-section">
          <h4>1. Enrollment Token</h4>
          <el-form :model="onboardingForm" label-width="150px">
            <el-form-item label="Token Tag">
              <el-input v-model="onboardingForm.tokenTag" placeholder="node-onboarding" />
            </el-form-item>
            <el-form-item label="Max Uses">
              <el-input-number v-model="onboardingForm.maxUses" :min="1" :max="1000" />
            </el-form-item>
            <el-form-item label="TTL Hours">
              <el-input-number v-model="onboardingForm.ttlHours" :min="1" :max="8760" />
            </el-form-item>
            <el-form-item>
              <el-button
                v-if="hasPermission('tokens:write')"
                type="primary"
                :loading="onboardingCreating"
                @click="createOnboardingToken"
              >
                Create Token
              </el-button>
            </el-form-item>
            <el-form-item v-if="onboardingTokenValue" label="Token">
              <el-input :value="onboardingTokenValue" readonly>
                <template #append>
                  <el-button @click="copyOnboardingToken">Copy</el-button>
                </template>
              </el-input>
            </el-form-item>
          </el-form>
        </div>

        <div class="onboarding-section">
          <h4>2. Target Settings</h4>
          <el-form :model="onboardingForm" label-width="150px">
            <el-form-item label="gRPC Server">
              <el-input v-model="onboardingForm.server" placeholder="https://aria.yun:50051" />
            </el-form-item>
            <el-form-item label="Controller API">
              <el-input v-model="onboardingForm.controllerApiUrl" placeholder="https://aria.yun" />
            </el-form-item>
            <el-form-item label="Controller CA Path">
              <el-input v-model="onboardingForm.caCertPath" placeholder="/etc/aria/certs/ca.crt" />
            </el-form-item>
            <el-form-item label="Controller CA URL">
              <el-input v-model="onboardingForm.caUrl" placeholder="https://aria.yun/api/v2/controller-info/grpc-ca.crt" />
            </el-form-item>
            <el-form-item label="TLS Server Name">
              <el-input v-model="onboardingForm.tlsServerName" placeholder="aria.yun" />
            </el-form-item>
            <el-form-item label="Region">
              <el-input v-model="onboardingForm.region" placeholder="default" />
            </el-form-item>
            <el-form-item label="Interface">
              <el-input v-model="onboardingForm.interface" placeholder="aria0" />
            </el-form-item>
            <el-form-item label="Hostname">
              <el-input v-model="onboardingForm.hostname" placeholder="optional" />
            </el-form-item>
            <el-form-item label="Advertise Routes">
              <el-input v-model="onboardingForm.advertiseRoutes" placeholder="optional, comma separated CIDRs" />
            </el-form-item>
          </el-form>
        </div>

        <div class="onboarding-section">
          <h4>3. Install Command</h4>
          <pre class="init-command">{{ onboardingInstallCommand }}</pre>
          <div class="onboarding-actions">
            <el-button :disabled="!onboardingInstallCommand" @click="copyOnboardingCommand">
              Copy Install Command
            </el-button>
          </div>
          <h5>Advanced: init-only command</h5>
          <pre class="init-command">{{ onboardingInitCommand }}</pre>
          <div class="onboarding-actions">
            <el-button :disabled="!onboardingInitCommand" @click="copyOnboardingInitCommand">
              Copy Init-Only Command
            </el-button>
          </div>
        </div>

        <div class="onboarding-section">
          <div class="onboarding-section-header">
            <h4>4. Progress</h4>
            <el-button size="small" :icon="Refresh" :loading="loading" @click="refreshOnboardingProgress">
              Refresh
            </el-button>
          </div>
          <el-table :data="recentOnboardingNodes" size="small" empty-text="No registered nodes yet">
            <el-table-column prop="hostname" label="Hostname" min-width="150" />
            <el-table-column label="Phase" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="getOnboardingPhaseTagType(row.onboarding?.phase)">
                  {{ formatOnboardingPhase(row.onboarding?.phase) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Token" width="130">
              <template #default="{ row }">{{ row.onboarding?.tokenPreview || 'N/A' }}</template>
            </el-table-column>
            <el-table-column label="Last Sync" min-width="160">
              <template #default="{ row }">{{ row.onboarding?.lastSyncAt || row.lastSyncAt || 'N/A' }}</template>
            </el-table-column>
            <el-table-column label="Action" width="100">
              <template #default="{ row }">
                <el-button size="small" link type="primary" @click="openNodeDetailFromOnboarding(row)">
                  Detail
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="onboarding-section">
          <h4>5. Verify</h4>
          <ul class="onboarding-checklist">
            <li>Run the install command on the target machine.</li>
            <li>Confirm <code>systemctl status aria-agent --no-pager</code> is active.</li>
            <li>Check <code>journalctl -u aria-agent -n 120 --no-pager</code> if the service does not become healthy.</li>
            <li>Refresh this page and confirm the node is online or degraded with a visible reason.</li>
            <li>Open node detail to check last sync, desired/applied versions, commands, alerts, and certificate status.</li>
          </ul>
        </div>
      </div>
      <template #footer>
        <el-button @click="onboardingDialogVisible = false">Close</el-button>
      </template>
    </el-dialog>

    <!-- 节点编辑对话框 -->
    <el-dialog
      v-model="editDialogVisible"
      title="Edit Node Settings"
      width="550px"
    >
      <el-form :model="editForm" label-width="140px" ref="editFormRef">
        <el-form-item label="Hostname">
          <el-input v-model="editForm.hostname" />
        </el-form-item>
        <el-form-item label="Region">
          <el-input v-model="editForm.region" placeholder="e.g. cn-shanghai" />
        </el-form-item>
        <el-form-item label="Advertised Routes">
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
              + New Route
            </el-button>
          </div>
          <p class="form-help">设置节点向 Mesh 网络宣告的本地子网路由</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">Cancel</el-button>
        <el-button v-if="hasPermission('nodes:write')" type="primary" @click="saveNodeChanges" :loading="submitting">
          Save Changes
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, reactive, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import {
  Search,
  Refresh,
  Plus,
  Monitor,
  CircleCheck,
  CircleClose,
  Setting,
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
import { useTokenApi } from '../composables/useTokenApi'
import { fetchControllerInfo } from '../composables/useControllerInfo'
import { usePermission } from '../composables/usePermission'
import { useTenantChangeReload } from '../composables/useTenantChangeReload'

// 使用节点 store
const nodeStore = useNodeStore()
const { hasPermission } = usePermission()
const router = useRouter()

// 节点数据从 store 获取
const nodes = computed(() => nodeStore.nodes)
const loading = computed(() => nodeStore.loading)

const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const detailDialogVisible = ref(false)
const selectedNode = ref(null)
const commandLoading = ref(false)
const commandPollTimer = ref(null)
const onboardingDialogVisible = ref(false)
const onboardingCreating = ref(false)
const onboardingToken = ref(null)
const onboardingControllerInfo = ref({})

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

const inferTLSServerName = (server) => {
  try {
    return new URL(server || defaultGrpcServer()).hostname || 'aria.yun'
  } catch {
    return 'aria.yun'
  }
}

const onboardingForm = reactive({
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
const editForm = reactive({
  id: '',
  hostname: '',
  region: '',
  advertised_routes: []
})

// 路由输入相关
const inputVisible = ref(false)
const inputValue = ref('')
const InputRef = ref(null)

// 计算属性
const onlineCount = computed(() => nodes.value.filter(n => n.status === 'online').length)
const offlineCount = computed(() => nodes.value.filter(n => n.status === 'offline').length)
const maintenanceCount = computed(() => nodes.value.filter(n => n.status === 'maintenance').length)
const recentCommandCount = computed(() => selectedNode.value?.recentCommands?.length || 0)
const recentDeliveryCount = computed(() => selectedNode.value?.recentPolicyDeliveries?.length || 0)
const activeAlertCount = computed(() => selectedNode.value?.activeAlerts?.length || 0)
const failedCommandCount = computed(() => (selectedNode.value?.recentCommands || []).filter(item => item.status === 'failed').length)
const pendingCommandCount = computed(() => (selectedNode.value?.recentCommands || []).filter(item => isPendingCommandStatus(item.status)).length)
const failedDeliveryCount = computed(() => (selectedNode.value?.recentPolicyDeliveries || []).filter(item => item.command_status === 'failed').length)
const pendingDeliveryCount = computed(() => (selectedNode.value?.recentPolicyDeliveries || []).filter(item => isPendingCommandStatus(item.command_status)).length)
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
      label: 'Commands',
      value: recentCommandCount.value,
      status: failedCommandCount.value > 0 ? `${failedCommandCount.value} failed` : `${pendingCommandCount.value} pending`,
      type: failedCommandCount.value > 0 ? 'danger' : pendingCommandCount.value > 0 ? 'warning' : 'success',
      focus: 'commands'
    },
    {
      key: 'policies',
      label: 'Policy Deliveries',
      value: recentDeliveryCount.value,
      status: failedDeliveryCount.value > 0 ? `${failedDeliveryCount.value} failed` : `${pendingDeliveryCount.value} pending`,
      type: failedDeliveryCount.value > 0 ? 'danger' : pendingDeliveryCount.value > 0 ? 'warning' : 'success',
      focus: 'policies'
    },
    {
      key: 'alerts',
      label: 'Active Alerts',
      value: activeAlertCount.value,
      status: activeAlertCount.value > 0 ? 'active' : 'clear',
      type: activeAlertCount.value > 0 ? 'danger' : 'success',
      focus: 'alerts'
    },
    {
      key: 'certificate',
      label: 'Certificate',
      value: certStatus,
      status: selectedNode.value?.certificateActivity?.last_renew_failure ? 'renew failed' : certStatus,
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
    ElMessage.error('Missing permission to create enrollment tokens')
    return
  }
  const tag = String(onboardingForm.tokenTag || '').trim()
  if (!tag) {
    ElMessage.error('Token tag is required')
    return
  }

  onboardingCreating.value = true
  try {
    onboardingToken.value = await useTokenApi.createToken({
      tag,
      max_uses: Number(onboardingForm.maxUses || 1),
      ttl: `${Number(onboardingForm.ttlHours || 24)}h`
    })
    ElMessage.success('Enrollment token created')
  } catch (error) {
    console.error('Failed to create enrollment token:', error)
    ElMessage.error(`Failed to create token: ${error.message || error}`)
  } finally {
    onboardingCreating.value = false
  }
}

const shellArg = (value) => {
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

const copyText = async (value, successMessage) => {
  if (!value) {
    ElMessage.warning('Nothing to copy')
    return
  }
  if (!navigator?.clipboard?.writeText) {
    ElMessage.error('Clipboard API is unavailable')
    return
  }
  await navigator.clipboard.writeText(value)
  ElMessage.success(successMessage)
}

const copyOnboardingToken = () => copyText(onboardingTokenValue.value, 'Token copied')

const copyOnboardingCommand = () => copyText(onboardingInstallCommand.value, 'Install command copied')

const copyOnboardingInitCommand = () => copyText(onboardingInitCommand.value, 'Init command copied')

const tokenPreview = (value) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  if (raw.length <= 10) return 'redacted'
  return `${raw.slice(0, 6)}...${raw.slice(-4)}`
}

const getOnboardingPhaseTagType = (phase) => {
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

const formatOnboardingPhase = (phase) => {
  switch (phase) {
    case 'online':
      return 'Online'
    case 'syncing':
      return 'Syncing'
    case 'degraded':
      return 'Degraded'
    case 'registered':
    default:
      return 'Registered'
  }
}

const onboardingTroubleshootingCommand = (node) => {
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

const openNodeDetailFromOnboarding = async (node) => {
  onboardingDialogVisible.value = false
  await viewNodeDetails(node)
}

const viewNodeDetails = async (node) => {
  try {
    selectedNode.value = await nodeStore.loadNodeDetail(node.id)
    // Fetch real bandwidth/latency metrics
    try {
      const metrics = await useMonitorApi.getNodeMetrics(node.id)
      if (metrics && selectedNode.value) {
        selectedNode.value.bandwidth = {
          upload: metrics.upload_mbps != null ? Number(metrics.upload_mbps.toFixed(2)) : 0,
          download: metrics.download_mbps != null ? Number(metrics.download_mbps.toFixed(2)) : 0
        }
        selectedNode.value.latency = metrics.latency_ms != null ? Number(metrics.latency_ms.toFixed(1)) : 0
      }
    } catch (metricsError) {
      console.error('Failed to load node metrics:', metricsError)
    }
    detailDialogVisible.value = true
  } catch (error) {
    console.error('Failed to load node detail:', error)
    ElMessage.error('Failed to load node details')
  }
}

// 编辑逻辑
const handleEditNode = (node) => {
  editForm.id = node.id
  editForm.hostname = node.hostname
  editForm.region = node.region
  // 确保是数组拷贝
  editForm.advertised_routes = Array.isArray(node.routes) ? [...node.routes] : []
  editDialogVisible.value = true
}

const handleRemoveRoute = (tag) => {
  editForm.advertised_routes.splice(editForm.advertised_routes.indexOf(tag), 1)
}

const showInput = () => {
  inputVisible.value = true
  nextTick(() => {
    InputRef.value?.focus()
  })
}

const handleInputConfirm = () => {
  if (inputValue.value) {
    // 简单的 CIDR 校验逻辑
    if (!/^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/.test(inputValue.value)) {
      ElMessage.warning('Please enter a valid CIDR (e.g. 192.168.1.0/24)')
      return
    }
    if (!editForm.advertised_routes.includes(inputValue.value)) {
      editForm.advertised_routes.push(inputValue.value)
    }
  }
  inputVisible.value = false
  inputValue.value = ''
}

const saveNodeChanges = async () => {
  submitting.value = true
  try {
    await nodeStore.updateNodeRemote(editForm.id, {
      hostname: editForm.hostname,
      region: editForm.region,
      advertised_routes: editForm.advertised_routes
    })
    ElMessage.success('Node settings updated successfully')
    editDialogVisible.value = false
    await refreshNodes()
  } catch (error) {
    ElMessage.error('Failed to update node settings')
  } finally {
    submitting.value = false
  }
}

const handleDeleteNode = async (id) => {
  try {
    await nodeStore.deleteNodeRemote(id)
    ElMessage.success('Node deleted')
  } catch (error) {
    ElMessage.error('Failed to delete node')
  }
}

const closeDetailDialog = () => {
  stopCommandPolling()
  detailDialogVisible.value = false
  selectedNode.value = null
}

const openMonitoringDetail = (focus = '') => {
  if (!selectedNode.value?.id) return
  detailDialogVisible.value = false
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: selectedNode.value.id },
    ...(focus ? { query: { focus } } : {})
  })
}

const openPolicyCenter = (delivery = null) => {
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

const handleSummaryClick = (focus) => {
  if (focus === 'policies') {
    openPolicyCenter()
    return
  }
  openMonitoringDetail(focus)
}

const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page) => {
  currentPage.value = page
}

const reloadSelectedNode = async (preserveCommand = null) => {
  if (!selectedNode.value?.id) return
  selectedNode.value = await nodeStore.loadNodeDetail(selectedNode.value.id)
  if (preserveCommand) {
    prependCommandIfMissing(preserveCommand)
  }
}

const isTerminalCommandStatus = (status) => ['completed', 'failed'].includes(status)

const findRecentCommand = (commandId) => {
  if (!commandId || !selectedNode.value?.recentCommands) return null
  return selectedNode.value.recentCommands.find(item => item.id === commandId) || null
}

const stopCommandPolling = () => {
  if (commandPollTimer.value) {
    clearTimeout(commandPollTimer.value)
    commandPollTimer.value = null
  }
}

const pollCommandStatus = (command, attemptsRemaining = 15) => {
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

const normalizeQueuedCommand = (command, response) => ({
  id: response?.command_id || response?.id || '',
  command: response?.command || command,
  status: response?.status || 'pending',
  message: response?.message || 'Command queued for delivery',
  created_at: response?.created_at || new Date().toISOString(),
  updated_at: response?.updated_at || response?.created_at || new Date().toISOString(),
  timeout_seconds: response?.timeout_seconds,
  priority: response?.priority
})

const prependCommandIfMissing = (command) => {
  if (!selectedNode.value || !command?.id) return
  const existing = Array.isArray(selectedNode.value.recentCommands) ? selectedNode.value.recentCommands : []
  if (existing.some(item => item.id === command.id)) {
    selectedNode.value.recentCommands = existing
    return
  }
  selectedNode.value.recentCommands = [command, ...existing]
}

const runQuickCommand = async (command) => {
  if (!selectedNode.value?.id) return
  if (!hasPermission('commands:write')) {
    ElMessage.error('Missing permission to send commands')
    return
  }

  commandLoading.value = true
  try {
    const response = await useAgentProxyApi.sendAgentCommand(selectedNode.value.id, {
      command,
      params: {},
      timeout: 30
    })
    const queuedCommand = normalizeQueuedCommand(command, response)
    prependCommandIfMissing(queuedCommand)
    ElMessage.success(`${command} queued`)
    await reloadSelectedNode(queuedCommand)
    const latest = findRecentCommand(queuedCommand.id)
    if (!latest || !isTerminalCommandStatus(latest.status)) {
      pollCommandStatus(queuedCommand)
    }
  } catch (error) {
    console.error(`Failed to queue ${command}:`, error)
    ElMessage.error(`Failed to queue ${command}`)
  } finally {
    commandLoading.value = false
  }
}

const formatConvergence = (state) => {
  const map = {
    converged: 'Converged',
    pending: 'Pending',
    diverged: 'Diverged',
    idle: 'Idle'
  }
  return map[state] || state || 'Unknown'
}

const getConvergenceTagType = (state) => {
  switch (state) {
    case 'converged': return 'success'
    case 'pending': return 'warning'
    case 'diverged': return 'danger'
    default: return 'info'
  }
}

const getAlertSeverityTagType = (severity) => {
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

const getDeliveryStatusTagType = (status) => {
  switch (status) {
    case 'completed':
    case 'applied':
      return 'success'
    case 'pending':
    case 'queued':
    case 'sent':
    case 'acknowledged':
    case 'in_progress':
      return 'warning'
    case 'stale':
      return 'info'
    case 'failed':
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}

const getCommandStatusTagType = (status) => getDeliveryStatusTagType(status)

const getCertificateStatusTagType = (status) => {
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

const isPendingCommandStatus = (status) => ['pending', 'queued', 'sent', 'acknowledged', 'in_progress', 'running'].includes(status)

const shortStateVersion = (value) => {
  if (!value) return 'N/A'
  return value.length > 12 ? `${value.slice(0, 12)}...` : value
}

const shortCommandId = (value) => {
  if (!value) return '-'
  return String(value).length > 10 ? `${String(value).slice(0, 10)}...` : String(value)
}

const formatLargeNumber = (value) => {
  const number = Number(value || 0)
  if (number >= 1000000) return `${(number / 1000000).toFixed(1)}M`
  if (number >= 1000) return `${(number / 1000).toFixed(1)}K`
  return String(number)
}

const formatMetricBytes = (value) => {
  const bytes = Number(value || 0)
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

const formatCommandTime = (value) => {
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

.stats-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
.stat-item { display: flex; align-items: center; gap: 16px; padding: 16px; background: var(--aria-bg-secondary); border: 1px solid var(--aria-border-primary); border-radius: var(--aria-radius-lg); transition: border-color var(--aria-transition-base), box-shadow var(--aria-transition-base); }
.stat-item:hover { border-color: var(--aria-border-hover); box-shadow: var(--aria-shadow); }
.stat-icon { width: 48px; height: 48px; border-radius: var(--aria-radius-md); display: flex; align-items: center; justify-content: center; font-size: 24px; flex-shrink: 0; }
.stat-icon.blue { background: rgba(59, 130, 246, 0.15); color: #3B82F6; }
.stat-icon.green { background: rgba(34, 197, 94, 0.15); color: #22C55E; }
.stat-icon.orange { background: rgba(245, 158, 11, 0.15); color: #F59E0B; }
.stat-icon.purple { background: rgba(139, 92, 246, 0.15); color: #8B5CF6; }
.stat-value { font-size: 28px; font-weight: 700; color: var(--aria-text-primary); line-height: 1; }
.stat-label { font-size: 13px; font-weight: 500; color: var(--aria-text-secondary); }

.nodes-card { background: var(--aria-bg-secondary); border: 1px solid var(--aria-border-primary); border-radius: var(--aria-radius-lg); }
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
@media (max-width: 768px) { .stats-cards { grid-template-columns: repeat(2, 1fr); } .stats-grid { grid-template-columns: 1fr; } .detail-toolbar { flex-wrap: wrap; } }
@media (max-width: 520px) { .workbench-summary-grid, .control-state-grid { grid-template-columns: 1fr; } }
</style>

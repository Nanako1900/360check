import type {
  AppConfig,
  AuthTokens,
  DictItem,
  DictType,
  ExportJob,
  ExportType,
  Inspection,
  InspectionTask,
  MediaAsset,
  Permission,
  Problem,
  ProblemFeature,
  ProblemProcessingLog,
  Project,
  Role,
  StatsOverview,
  TrajectoryPoint,
  User,
} from '@/shared/api/types'

export const NOW = '2026-06-26T00:00:00Z'

/**
 * 完整权限码集合（§P2 目录）—— 让 dev 菜单展示全部入口。
 * 权威来源是后端种子 backend/db/migrations/000002_seed.up.sql（37 码），此处逐一镜像，
 * 前端不得臆造权限码（§7）；mock 的 GET /permissions 必须与真实后端返回一致。
 */
export const ALL_PERMISSIONS: string[] = [
  'user:read',
  'user:create',
  'user:update',
  'user:delete',
  'user:roles:read',
  'user:roles:write',
  'role:read',
  'role:create',
  'role:update',
  'role:delete',
  'role:perms:read',
  'role:perms:write',
  'permission:read',
  'dict:read',
  'dict:create',
  'dict:update',
  'dict:delete',
  'config:read',
  'config:update',
  'project:read',
  'project:create',
  'project:update',
  'project:delete',
  'task:read',
  'task:create',
  'task:update',
  'task:delete',
  'inspection:read',
  'problem:read',
  'problem:create',
  'problem:update',
  'problem:delete',
  'problem_log:write',
  'media:read',
  'stats:read',
  'export:create',
  'export:read',
]

/**
 * 兼容旧用例的稳定 seed 用户引用（P0/P1 测试用 `USERS[0]` 取 admin）。
 * 注意：列表 CRUD 的真源是 `db.users`（可变）；`USERS` 仅作不可变种子参考。
 */
export const USERS: User[] = seedUsers()

/** 可变用户列表（P2 mock 后端真源）：seed admin + 两名巡查员（含 last_login_at）。 */
function seedUsers(): User[] {
  return [
    {
      id: 1,
      username: 'admin',
      display_name: '系统管理员',
      phone: null,
      email: null,
      avatar_media_id: null,
      is_active: true,
      last_login_at: NOW,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 2,
      username: 'inspector_li',
      display_name: '李巡查',
      phone: '13800000002',
      email: 'li@example.com',
      avatar_media_id: null,
      is_active: true,
      last_login_at: '2026-06-20T03:30:00Z',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 3,
      username: 'inspector_wang',
      display_name: '王巡查',
      phone: null,
      email: null,
      avatar_media_id: null,
      is_active: false,
      last_login_at: null,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** 可变角色列表：is_system 的 admin/inspector + 一个自定义角色。 */
function seedRoles(): Role[] {
  return [
    {
      id: 1,
      code: 'admin',
      name: '管理员',
      description: '系统超级管理员',
      is_system: true,
      sort_order: 0,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 2,
      code: 'inspector',
      name: '巡查员',
      description: '现场巡查与标注',
      is_system: true,
      sort_order: 1,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 3,
      code: 'auditor',
      name: '审计员',
      description: '只读审计角色',
      is_system: false,
      sort_order: 2,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** 权限目录（按 group_name 分组覆盖 §P2 权限码）。 */
function seedPermissions(): Permission[] {
  // 与后端种子 000002_seed.up.sql 的 37 个权限码逐一对齐（code 为唯一权威；
  // 分组沿用本 mock 的展示聚合，仅用于 dev 权限树呈现）。
  const rows: Array<[string, string, string]> = [
    // [code, name, group_name]
    ['user:read', '查看用户', '用户管理'],
    ['user:create', '新建用户', '用户管理'],
    ['user:update', '编辑用户', '用户管理'],
    ['user:delete', '删除用户', '用户管理'],
    ['user:roles:read', '查看用户角色', '用户管理'],
    ['user:roles:write', '分配用户角色', '用户管理'],
    ['role:read', '查看角色', '角色权限'],
    ['role:create', '新建角色', '角色权限'],
    ['role:update', '编辑角色', '角色权限'],
    ['role:delete', '删除角色', '角色权限'],
    ['role:perms:read', '查看角色权限', '角色权限'],
    ['role:perms:write', '配置角色权限', '角色权限'],
    ['permission:read', '查看权限目录', '角色权限'],
    ['dict:read', '查看字典', '字典配置'],
    ['dict:create', '新建字典', '字典配置'],
    ['dict:update', '编辑字典', '字典配置'],
    ['dict:delete', '删除字典', '字典配置'],
    ['config:read', '查看系统配置', '字典配置'],
    ['config:update', '编辑系统配置', '字典配置'],
    ['project:read', '查看项目', '项目任务'],
    ['project:create', '新建项目', '项目任务'],
    ['project:update', '编辑项目', '项目任务'],
    ['project:delete', '删除项目', '项目任务'],
    ['task:read', '查看任务', '项目任务'],
    ['task:create', '新建任务', '项目任务'],
    ['task:update', '编辑任务', '项目任务'],
    ['task:delete', '删除任务', '项目任务'],
    ['inspection:read', '查看巡查记录', '巡查问题'],
    ['problem:read', '查看问题', '巡查问题'],
    ['problem:create', '新建问题', '巡查问题'],
    ['problem:update', '处理问题', '巡查问题'],
    ['problem:delete', '删除问题', '巡查问题'],
    ['problem_log:write', '追加处理记录', '巡查问题'],
    ['media:read', '查看媒体', '媒体统计'],
    ['stats:read', '查看统计', '媒体统计'],
    ['export:create', '创建导出', '媒体统计'],
    ['export:read', '查询导出', '媒体统计'],
  ]
  return rows.map(([code, name, group_name], i) => {
    const [object, action] = code.split(':')
    return {
      id: 500 + i,
      code,
      name,
      object,
      action,
      group_name,
      sort_order: i,
    }
  })
}

/** roleId → permission id 列表（casbin p-rules 镜像）。admin 拥有全部。 */
function seedRolePermissions(perms: Permission[]): Record<number, number[]> {
  const allIds = perms.map((p) => p.id)
  const byCode = (code: string): number => {
    const found = perms.find((p) => p.code === code)
    return found ? found.id : -1
  }
  return {
    1: allIds, // admin → 全部
    2: [
      'project:read',
      'task:read',
      'inspection:read',
      'problem:read',
      'problem:update',
      'problem_log:write',
      'media:read',
    ].map(byCode),
    3: ['project:read', 'inspection:read', 'problem:read', 'stats:read'].map(byCode),
  }
}

/** userId → role id 列表（casbin g-rules 镜像）。 */
function seedUserRoles(): Record<number, number[]> {
  return {
    1: [1], // admin → admin role
    2: [2], // inspector_li → inspector
    3: [2], // inspector_wang → inspector
  }
}

/** 可变字典/配置种子（P1-2 mock 后端的真源）。深拷贝以隔离 reset 之间的可变副作用。 */
function seedDictTypes(): DictType[] {
  return [
    {
      id: 10,
      code: 'problem_status',
      name: '问题状态',
      scope: 'problem_status',
      description: '问题处理状态',
      version: 1,
      content_hash: 'hash-ps-1',
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 11,
      code: 'problem_type',
      name: '问题类型',
      scope: 'problem_type',
      description: '巡查问题分类',
      version: 1,
      content_hash: 'hash-pt-1',
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 14,
      code: 'problem_category',
      name: '问题分类',
      scope: 'problem_category',
      description: '问题归类维度',
      version: 1,
      content_hash: 'hash-pc-1',
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 12,
      code: 'capture_main',
      name: '主拍照预设',
      scope: 'capture_preset',
      description: '默认拍照参数',
      version: 1,
      content_hash: 'hash-cp-1',
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 13,
      code: 'legacy_misc',
      name: '历史杂项',
      scope: 'misc',
      description: '已退役类型',
      version: 1,
      content_hash: 'hash-mc-1',
      is_active: false,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

function seedDictItems(): DictItem[] {
  return [
    {
      id: 101,
      dict_type_id: 10,
      code: 'OPEN',
      label: '待处理',
      color: '#8c8c8c',
      sort_order: 0,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 102,
      dict_type_id: 10,
      code: 'PROCESSING',
      label: '处理中',
      color: '#1677ff',
      sort_order: 1,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 103,
      dict_type_id: 10,
      code: 'RESOLVED',
      label: '已解决',
      color: '#52c41a',
      sort_order: 2,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 104,
      dict_type_id: 10,
      code: 'CLOSED',
      label: '已关闭',
      color: '#bfbfbf',
      sort_order: 3,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 199,
      dict_type_id: 10,
      code: 'LEGACY',
      label: '历史状态',
      color: '#999999',
      sort_order: 9,
      is_active: false,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 201,
      dict_type_id: 11,
      code: 'CRACK',
      label: '裂缝',
      color: '#fa541c',
      sort_order: 0,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 202,
      dict_type_id: 11,
      code: 'POTHOLE',
      label: '坑洼',
      color: '#d48806',
      sort_order: 1,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 401,
      dict_type_id: 14,
      code: 'SURFACE',
      label: '路面',
      color: '#722ed1',
      sort_order: 0,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 402,
      dict_type_id: 14,
      code: 'FACILITY',
      label: '设施',
      color: '#13c2c2',
      sort_order: 1,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 301,
      dict_type_id: 12,
      code: 'HIGH_RES',
      label: '高清',
      color: null,
      extra: { width: 4096, quality: 80 },
      sort_order: 0,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

function seedAppConfigs(): AppConfig[] {
  return [
    {
      id: 1,
      config_key: 'export.image',
      value: { width: 4096, quality: 80 },
      version: 2,
      content_hash: 'cfg-exp-2',
      is_active: true,
      effective_from: NOW,
      effective_to: null,
      description: '导出图片尺寸/质量',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 2,
      config_key: 'capture.default',
      value: { width: 4096, quality: 90, hdr: true },
      version: 1,
      content_hash: 'cfg-cap-1',
      is_active: true,
      effective_from: NOW,
      effective_to: null,
      description: '拍照默认参数',
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** config_key → 历史版本（含已失效旧版，按版本升序）。 */
function seedConfigHistory(): Record<string, AppConfig[]> {
  return {
    'export.image': [
      {
        id: 11,
        config_key: 'export.image',
        value: { width: 2048, quality: 70 },
        version: 1,
        content_hash: 'cfg-exp-1',
        is_active: false,
        effective_from: NOW,
        effective_to: NOW,
        description: '旧版导出配置',
        created_at: NOW,
        updated_at: NOW,
      },
      {
        id: 1,
        config_key: 'export.image',
        value: { width: 4096, quality: 80 },
        version: 2,
        content_hash: 'cfg-exp-2',
        is_active: true,
        effective_from: NOW,
        effective_to: null,
        description: '导出图片尺寸/质量',
        created_at: NOW,
        updated_at: NOW,
      },
    ],
    'capture.default': [
      {
        id: 2,
        config_key: 'capture.default',
        value: { width: 4096, quality: 90, hdr: true },
        version: 1,
        content_hash: 'cfg-cap-1',
        is_active: true,
        effective_from: NOW,
        effective_to: null,
        description: '拍照默认参数',
        created_at: NOW,
        updated_at: NOW,
      },
    ],
  }
}

/** 可变项目列表（§P3 mock 真源）：含 custom_fields 与三种状态。 */
function seedProjects(): Project[] {
  return [
    {
      id: 1,
      code: 'PRJ-2026-001',
      name: '滨江绿道巡查',
      description: '滨江沿线市政绿道日常巡查',
      status: 'ACTIVE',
      custom_fields: { region: '滨江区', budget: 120 },
      start_date: '2026-03-01',
      end_date: '2026-12-31',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 2,
      code: 'PRJ-2026-002',
      name: '老城区桥梁排查',
      description: '老城区桥梁结构安全排查',
      status: 'PAUSED',
      custom_fields: { region: '上城区' },
      start_date: '2026-04-15',
      end_date: null,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 3,
      code: 'PRJ-2025-009',
      name: '已归档示范项目',
      description: '上一年度归档项目',
      status: 'ARCHIVED',
      start_date: '2025-01-01',
      end_date: '2025-12-31',
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** 可变任务列表：归属 project 1，覆盖多个 task_status。 */
function seedTasks(): InspectionTask[] {
  return [
    {
      id: 1,
      project_id: 1,
      title: '一号段步道巡查',
      description: '0–2km 路段',
      status: 'PENDING',
      assignee_id: 2,
      planned_start: '2026-06-01T01:00:00Z',
      planned_end: '2026-06-01T09:00:00Z',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 2,
      project_id: 1,
      title: '二号段桥下巡查',
      status: 'IN_PROGRESS',
      assignee_id: 3,
      planned_start: null,
      planned_end: null,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 3,
      project_id: 2,
      title: '主桥承重检查',
      status: 'COMPLETED',
      assignee_id: null,
      planned_start: null,
      planned_end: null,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** 可变巡查记录：一条 FINISHED（含里程/时长/点数/route_geom）+ 一条 IN_PROGRESS（route_geom 为 null）。 */
function seedInspections(): Inspection[] {
  return [
    {
      id: 1,
      client_uuid: '11111111-1111-1111-1111-111111111111',
      project_id: 1,
      task_id: 1,
      inspector_id: 2,
      status: 'FINISHED',
      started_at: '2026-06-20T01:00:00Z',
      ended_at: '2026-06-20T02:05:25Z',
      duration_seconds: 3925,
      mileage_meters: 12567.8,
      point_count: 1842,
      route_geom: {
        type: 'LineString',
        coordinates: [
          [120.21, 30.25],
          [120.212, 30.2512],
          [120.214, 30.2518],
          [120.215, 30.252],
        ],
      },
      note: '路面良好，发现 2 处裂缝',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 2,
      client_uuid: '22222222-2222-2222-2222-222222222222',
      project_id: 1,
      task_id: 2,
      inspector_id: 3,
      status: 'IN_PROGRESS',
      started_at: '2026-06-26T00:30:00Z',
      ended_at: null,
      duration_seconds: 0,
      mileage_meters: 0,
      point_count: 0,
      route_geom: undefined,
      note: undefined,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** 巡查 1 的轨迹点（WGS84，与 route_geom 同走向）。inspection 2（进行中）无点。 */
function seedTrajectoryPoints(): Record<number, TrajectoryPoint[]> {
  const coords: Array<[number, number]> = [
    [120.21, 30.25],
    [120.212, 30.2512],
    [120.214, 30.2518],
    [120.215, 30.252],
  ]
  return {
    1: coords.map((c, i) => ({
      id: 9000 + i,
      client_uuid: `aaaaaaaa-0000-0000-0000-00000000000${i}`,
      inspection_id: 1,
      seq: i,
      geom: { type: 'Point', coordinates: c },
      recorded_at: '2026-06-20T01:00:00Z',
    })),
    2: [],
  }
}

/**
 * 问题点（GeoJSON Feature，WGS84）。归属 project 1 / inspection 1，带类型/状态标签 + 颜色。
 * 用于 /problems/map，按 inspection_id / project_id 过滤。
 */
/** mock 存储项：Feature + 服务端过滤键（project_id / inspection_id），过滤后只回 Feature。 */
export interface StoredProblemFeature {
  project_id: number
  inspection_id: number | null
  feature: ProblemFeature
}

function seedProblemFeatures(): StoredProblemFeature[] {
  const features: ProblemFeature[] = [
    {
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [120.2115, 30.2509] },
      properties: {
        id: 5001,
        type_item_id: 201,
        status_item_id: 101,
        category_item_id: null,
        type_label: '裂缝',
        status_label: '待处理',
        status_color: '#8c8c8c',
        title: '步道路面横向裂缝，长约 1.2m',
        captured_at: '2026-06-20T01:20:00Z',
        cover_media_id: 7001,
      },
    },
    {
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [120.2138, 30.2516] },
      properties: {
        id: 5002,
        type_item_id: 201,
        status_item_id: 102,
        category_item_id: null,
        type_label: '裂缝',
        status_label: '处理中',
        status_color: '#1677ff',
        title: '桥下接缝渗水',
        captured_at: '2026-06-20T01:45:00Z',
        cover_media_id: null,
      },
    },
  ]
  return features.map((feature) => ({ project_id: 1, inspection_id: 1, feature }))
}

/**
 * 可变问题列表（§P6 mock 真源）。覆盖：
 * - 5001：项目 1 / 巡查 1，active 状态 OPEN(101)，含封面 7101。
 * - 5002：项目 1 / 巡查 1，active 状态 PROCESSING(102)，无封面。
 * - 5003：项目 2 / 无巡查，**引用退役状态 LEGACY(199)**（历史容忍验证：列表/详情仍须渲染）。
 */
function seedProblems(): Problem[] {
  return [
    {
      id: 5001,
      client_uuid: 'dddddddd-0000-0000-0000-000000005001',
      project_id: 1,
      inspection_id: 1,
      inspector_id: 2,
      geom: { type: 'Point', coordinates: [120.2115, 30.2509] },
      type_item_id: 201,
      status_item_id: 101,
      category_item_id: 401,
      dict_version_used: 1,
      title: '步道路面横向裂缝，长约 1.2m',
      description: '位于一号段步道中段，横向延伸，需评估是否扩展。',
      note: '已现场拍照',
      captured_at: '2026-06-20T01:20:00Z',
      cover_media_id: 7101,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 5002,
      client_uuid: 'dddddddd-0000-0000-0000-000000005002',
      project_id: 1,
      inspection_id: 1,
      inspector_id: 2,
      geom: { type: 'Point', coordinates: [120.2138, 30.2516] },
      type_item_id: 201,
      status_item_id: 102,
      category_item_id: 402,
      dict_version_used: 1,
      title: '桥下接缝渗水',
      description: '桥梁伸缩缝处渗水，雨后明显。',
      captured_at: '2026-06-20T01:45:00Z',
      cover_media_id: null,
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 5003,
      client_uuid: 'dddddddd-0000-0000-0000-000000005003',
      project_id: 2,
      inspection_id: null,
      inspector_id: 3,
      geom: { type: 'Point', coordinates: [120.16, 30.27] },
      type_item_id: 202,
      // 退役字典项（is_active=false）：离线回传历史问题，客户端必须能渲染。
      status_item_id: 199,
      category_item_id: null,
      dict_version_used: 1,
      title: '历史问题（引用已退役状态）',
      captured_at: '2025-12-01T02:00:00Z',
      cover_media_id: null,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/** problemId → 处理记录（追加式审计，按时间升序）。含后端写的 STATUS_CHANGE + 前端可写的 COMMENT。 */
function seedProblemLogs(): Record<number, ProblemProcessingLog[]> {
  return {
    5001: [
      {
        id: 8001,
        problem_id: 5001,
        action: 'COMMENT',
        note: '初步登记，待复核',
        operator_id: 2,
        created_at: '2026-06-20T01:25:00Z',
      },
    ],
    5002: [],
    5003: [],
  }
}

/**
 * 媒体资产（§P5）：一张 CONFIRMED 的 `web` tier 等距柱状图（2:1，4096×2048，含签名 URL）用于全景渲染，
 * 同 media_group 的 `thumb`；外加一条 UNCONFIRMED（verified_at=null）证明「未确认不渲染」路径。
 * cover_media_id=7001（问题 5001）映射到 web 媒体 7101（mock 简化：直接以 7101 作为 web 资源 id）。
 */
const MEDIA_GROUP = '99999999-9999-9999-9999-999999999999'

function seedMedia(): MediaAsset[] {
  return [
    {
      id: 7101,
      client_uuid: 'bbbbbbbb-0000-0000-0000-000000007101',
      owner_type: 'problem',
      owner_id: 5001,
      tier: 'web',
      cos_bucket: 'c5-media',
      cos_key: 'web/7101.jpg',
      cos_region: 'ap-shanghai',
      content_type: 'image/jpeg',
      width: 4096,
      height: 2048,
      capture_state: 'CONFIRMED',
      verified_at: NOW,
      media_group: MEDIA_GROUP,
      signed_url: 'https://cdn.example.com/web/7101.jpg?sign=stub-web',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 7102,
      client_uuid: 'bbbbbbbb-0000-0000-0000-000000007102',
      owner_type: 'problem',
      owner_id: 5001,
      tier: 'thumb',
      cos_bucket: 'c5-media',
      cos_key: 'thumb/7102.jpg',
      cos_region: 'ap-shanghai',
      content_type: 'image/jpeg',
      width: 512,
      height: 256,
      capture_state: 'CONFIRMED',
      verified_at: NOW,
      media_group: MEDIA_GROUP,
      signed_url: 'https://cdn.example.com/thumb/7102.jpg?sign=stub-thumb',
      created_at: NOW,
      updated_at: NOW,
    },
    {
      id: 7103,
      client_uuid: 'bbbbbbbb-0000-0000-0000-000000007103',
      owner_type: 'problem',
      owner_id: 5002,
      tier: 'web',
      cos_bucket: 'c5-media',
      cos_key: 'web/7103.jpg',
      cos_region: 'ap-shanghai',
      content_type: 'image/jpeg',
      width: 4096,
      height: 2048,
      // 处理中：尚未确认 → verified_at 为空 → 不可渲染。
      capture_state: 'UPLOADED',
      verified_at: null,
      media_group: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
      signed_url: null,
      created_at: NOW,
      updated_at: NOW,
    },
  ]
}

/**
 * 统计概览基线（§P7 / D2 §4.7 单一权威形状）。颜色取自 dict_item（含退役项：
 * counts_by_type 含「历史杂项 LEGACY」桶，type_item_id=910，对应已退役 dict_item，color+label 齐全，
 * 证明历史统计仍计入退役类型）。handlers 按 project_id/inspector_id/from/to 略微缩放计数，使筛选可被断言。
 */
function seedStatsOverview(): StatsOverview {
  return {
    inspection_count: 120,
    problem_count: 340,
    total_mileage_meters: 1234567.8,
    total_duration_seconds: 987654,
    avg_duration_seconds: 8230,
    counts_by_type: [
      { item_id: 201, label: '裂缝', color: '#fa541c', count: 180 },
      { item_id: 202, label: '坑洼', color: '#d48806', count: 96 },
      // 退役类型（dict_item is_active=false）：历史问题仍引用，统计必须计入并按 dict 颜色渲染。
      { item_id: 910, label: '历史类型（已退役）', color: '#999999', count: 64 },
    ],
    counts_by_status: [
      { item_id: 101, label: '待处理', color: '#8c8c8c', count: 120 },
      { item_id: 102, label: '处理中', color: '#1677ff', count: 140 },
      { item_id: 103, label: '已解决', color: '#52c41a', count: 80 },
    ],
    counts_by_inspector: [
      { inspector_id: 2, label: '李巡查', count: 210 },
      { inspector_id: 3, label: '王巡查', count: 130 },
    ],
    counts_by_project: [
      { project_id: 1, label: '滨江绿道巡查', count: 240 },
      { project_id: 2, label: '老城区桥梁排查', count: 100 },
    ],
  }
}

/** 导出基线供 handlers/测试引用（不可变 —— handlers 据此派生过滤后的新对象）。 */
export const STATS_OVERVIEW: StatsOverview = seedStatsOverview()

interface MockDb {
  passwords: Record<string, string>
  accessTokens: Map<string, string>
  refreshTokens: Map<string, string>
  users: User[]
  roles: Role[]
  permissionCatalog: Permission[]
  rolePermissions: Record<number, number[]>
  userRoles: Record<number, number[]>
  /** 当前会话（/auth/me）的有效权限码：dev 默认全集，保证菜单可见。 */
  permissions: string[]
  counter: number
  dictTypes: DictType[]
  dictItems: DictItem[]
  appConfigs: AppConfig[]
  configHistory: Record<string, AppConfig[]>
  projects: Project[]
  tasks: InspectionTask[]
  inspections: Inspection[]
  trajectoryPoints: Record<number, TrajectoryPoint[]>
  problemFeatures: StoredProblemFeature[]
  problems: Problem[]
  problemLogs: Record<number, ProblemProcessingLog[]>
  media: MediaAsset[]
  statsOverview: StatsOverview
  /** 导出任务（P8）：按 job_uuid 索引；GET 轮询逐次推进进度直到 SUCCEEDED。 */
  exportJobs: Map<string, ExportJob>
  idSeq: number
}

const seededPermissions = seedPermissions()

export const db: MockDb = {
  passwords: { admin: 'admin12345' },
  accessTokens: new Map(),
  refreshTokens: new Map(),
  users: seedUsers(),
  roles: seedRoles(),
  permissionCatalog: seededPermissions,
  rolePermissions: seedRolePermissions(seededPermissions),
  userRoles: seedUserRoles(),
  permissions: ALL_PERMISSIONS,
  counter: 0,
  dictTypes: seedDictTypes(),
  dictItems: seedDictItems(),
  appConfigs: seedAppConfigs(),
  configHistory: seedConfigHistory(),
  projects: seedProjects(),
  tasks: seedTasks(),
  inspections: seedInspections(),
  trajectoryPoints: seedTrajectoryPoints(),
  problemFeatures: seedProblemFeatures(),
  problems: seedProblems(),
  problemLogs: seedProblemLogs(),
  media: seedMedia(),
  statsOverview: seedStatsOverview(),
  exportJobs: new Map<string, ExportJob>(),
  idSeq: 1000,
}

// —— 导出任务 mock（P8）——

/** 进度推进步长（每次轮询 GET 推进的百分比），便于断言 PENDING→RUNNING→SUCCEEDED。 */
export const EXPORT_PROGRESS_STEP = 40
/** SUCCEEDED 时返回的签名 CDN URL（mock）。 */
export const EXPORT_RESULT_URL = 'https://cdn.example.com/exports/result.xlsx?sig=mock'
const EXPORT_TOTAL_ROWS = 100

/** 创建一个 PENDING 导出任务并入库（POST /exports）。 */
export function createExportJob(type: ExportType, params?: Record<string, unknown>): ExportJob {
  db.idSeq += 1
  const jobUuid = `job-${db.idSeq}`
  const job: ExportJob = {
    id: db.idSeq,
    job_uuid: jobUuid,
    type,
    params,
    status: 'PENDING',
    progress: 0,
    total_rows: EXPORT_TOTAL_ROWS,
    processed_rows: 0,
    result_url: null,
    error: null,
    created_at: NOW,
  }
  db.exportJobs.set(jobUuid, job)
  return job
}

/**
 * 推进一个导出任务（每次 GET 轮询调用）：PENDING→RUNNING(progress 递增)→SUCCEEDED(result_url)。
 * 返回推进后的不可变副本（写回库），便于「重复 GET 逐步前进」测试。
 */
export function advanceExportJob(jobUuid: string): ExportJob | undefined {
  const current = db.exportJobs.get(jobUuid)
  if (!current) return undefined
  if (
    current.status === 'SUCCEEDED' ||
    current.status === 'FAILED' ||
    current.status === 'CANCELLED'
  ) {
    return current
  }
  const nextProgress = Math.min(100, current.progress + EXPORT_PROGRESS_STEP)
  const processed = Math.round((nextProgress / 100) * (current.total_rows ?? EXPORT_TOTAL_ROWS))
  const next: ExportJob =
    nextProgress >= 100
      ? {
          ...current,
          status: 'SUCCEEDED',
          progress: 100,
          processed_rows: current.total_rows ?? EXPORT_TOTAL_ROWS,
          result_url: EXPORT_RESULT_URL,
          finished_at: NOW,
        }
      : {
          ...current,
          status: 'RUNNING',
          progress: nextProgress,
          processed_rows: processed,
          started_at: current.started_at ?? NOW,
        }
  db.exportJobs.set(jobUuid, next)
  return next
}

/** content_hash 生成（mock）：随变更单调递增，模拟后端按内容重算哈希。 */
export function bumpDictTypeVersion(type: DictType): DictType {
  return {
    ...type,
    version: type.version + 1,
    content_hash: `${type.content_hash}-v${type.version + 1}`,
    updated_at: NOW,
  }
}

/** 字典 mock（problem_status 基线 + 一个退役项，用于历史容忍验证）。 */
export interface DictFixture {
  version: number
  contentHash: string
  items: DictItem[]
}

export const DICT_FIXTURES: Record<string, DictFixture> = {
  problem_status: {
    version: 1,
    contentHash: 'hash-ps-1',
    items: [
      {
        id: 101,
        dict_type_id: 10,
        code: 'OPEN',
        label: '待处理',
        color: '#8c8c8c',
        sort_order: 0,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      {
        id: 102,
        dict_type_id: 10,
        code: 'PROCESSING',
        label: '处理中',
        color: '#1677ff',
        sort_order: 1,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      {
        id: 103,
        dict_type_id: 10,
        code: 'RESOLVED',
        label: '已解决',
        color: '#52c41a',
        sort_order: 2,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      {
        id: 104,
        dict_type_id: 10,
        code: 'CLOSED',
        label: '已关闭',
        color: '#bfbfbf',
        sort_order: 3,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      // 退役项：历史问题可能仍引用，客户端必须能渲染
      {
        id: 199,
        dict_type_id: 10,
        code: 'LEGACY',
        label: '历史状态',
        color: '#999999',
        sort_order: 9,
        is_active: false,
        created_at: NOW,
        updated_at: NOW,
      },
    ],
  },
  // project_field：驱动项目 custom_fields 动态表单（§P3）。item.code=字段键，item.label=字段标签，
  // extra.type='number' 时渲染数字输入；退役项（is_active=false）不进入表单。
  project_field: {
    version: 1,
    contentHash: 'hash-pf-1',
    items: [
      {
        id: 401,
        dict_type_id: 14,
        code: 'region',
        label: '所属区域',
        color: null,
        sort_order: 0,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      {
        id: 402,
        dict_type_id: 14,
        code: 'budget',
        label: '预算（万元）',
        color: null,
        extra: { type: 'number' },
        sort_order: 1,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      {
        id: 403,
        dict_type_id: 14,
        code: 'legacy_owner',
        label: '历史负责人',
        color: null,
        sort_order: 9,
        is_active: false,
        created_at: NOW,
        updated_at: NOW,
      },
    ],
  },
}

export function makeTokens(user: User): AuthTokens {
  db.counter += 1
  const access = `access-${db.counter}`
  const refresh = `refresh-${db.counter}`
  db.accessTokens.set(access, user.username)
  db.refreshTokens.set(refresh, user.username)
  return { access_token: access, refresh_token: refresh, expires_in: 900, user }
}

export function resetDb(): void {
  const perms = seedPermissions()
  db.passwords = { admin: 'admin12345' }
  db.accessTokens = new Map()
  db.refreshTokens = new Map()
  db.users = seedUsers()
  db.roles = seedRoles()
  db.permissionCatalog = perms
  db.rolePermissions = seedRolePermissions(perms)
  db.userRoles = seedUserRoles()
  db.permissions = ALL_PERMISSIONS
  db.counter = 0
  db.dictTypes = seedDictTypes()
  db.dictItems = seedDictItems()
  db.appConfigs = seedAppConfigs()
  db.configHistory = seedConfigHistory()
  db.projects = seedProjects()
  db.tasks = seedTasks()
  db.inspections = seedInspections()
  db.trajectoryPoints = seedTrajectoryPoints()
  db.problemFeatures = seedProblemFeatures()
  db.problems = seedProblems()
  db.problemLogs = seedProblemLogs()
  db.media = seedMedia()
  db.statsOverview = seedStatsOverview()
  db.exportJobs = new Map<string, ExportJob>()
  db.idSeq = 1000
}

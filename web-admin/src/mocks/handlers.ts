import { http, HttpResponse } from 'msw'
import type { ApiErrorCode, ApiErrorDetail } from '@/shared/api/apiError'
import type {
  AppConfig,
  AppConfigUpdate,
  AuthMe,
  DictItem,
  DictItemCreate,
  DictItemsPayload,
  DictItemUpdate,
  DictType,
  DictTypeCreate,
  DictTypeUpdate,
  InspectionTask,
  InspectionTaskCreate,
  InspectionTaskUpdate,
  MediaAsset,
  Problem,
  ProblemFeature,
  ProblemFeatureCollection,
  ProblemLogCreate,
  ProblemProcessingLog,
  ProblemUpdate,
  Project,
  ProjectCreate,
  ProjectUpdate,
  ResetPasswordRequest,
  Role,
  RoleCreate,
  RoleUpdate,
  SetRolePermissionsRequest,
  SetUserRolesRequest,
  StatsOverview,
  Trajectory,
  User,
  UserCreate,
  UserUpdate,
} from '@/shared/api/types'
import type { ExportJobCreate, ExportType } from '@/shared/api/types'
import {
  advanceExportJob,
  bumpDictTypeVersion,
  createExportJob,
  db,
  DICT_FIXTURES,
  EXPORT_RESULT_URL,
  makeTokens,
  NOW,
} from './db'

const BASE = '*/api/v1'

function ok<T>(data: T) {
  return HttpResponse.json({ success: true, data, error: null, meta: null })
}

function okList<T>(data: T[], total = data.length) {
  return HttpResponse.json({
    success: true,
    data,
    error: null,
    meta: { total, page: 1, page_size: total },
  })
}

function fail(code: ApiErrorCode, message: string, status: number, details: ApiErrorDetail[] = []) {
  return HttpResponse.json(
    { success: false, data: null, error: { code, message, details }, meta: null },
    { status },
  )
}

function bearer(request: Request): string | null {
  const header = request.headers.get('Authorization')
  return header?.startsWith('Bearer ') ? header.slice('Bearer '.length) : null
}

/** dev/test 默认「快乐路径」后端。需要异常序列的单测用 server.use() 覆盖。 */
export const handlers = [
  http.post(`${BASE}/auth/login`, async ({ request }) => {
    const body = (await request.json()) as { username?: string; password?: string }
    if (!body.username || !body.password) {
      return fail('VALIDATION_FAILED', '用户名与密码必填', 422, [
        { field: !body.username ? 'username' : 'password', code: 'REQUIRED', message: '必填' },
      ])
    }
    const user = db.users.find((u) => u.username === body.username)
    if (!user || db.passwords[body.username] !== body.password) {
      return fail('UNAUTHENTICATED', '用户名或密码错误', 401)
    }
    return ok(makeTokens(user))
  }),

  http.post(`${BASE}/auth/refresh`, async ({ request }) => {
    const body = (await request.json()) as { refresh_token?: string }
    const token = body.refresh_token
    if (!token || !db.refreshTokens.has(token)) {
      return fail('UNAUTHENTICATED', '刷新令牌无效或已过期', 401)
    }
    const username = db.refreshTokens.get(token)
    db.refreshTokens.delete(token) // 轮换
    const user = db.users.find((u) => u.username === username)
    if (!user) return fail('UNAUTHENTICATED', '用户不存在', 401)
    return ok(makeTokens(user))
  }),

  http.post(`${BASE}/auth/logout`, () => ok(null)),

  http.get(`${BASE}/auth/me`, ({ request }) => {
    const token = bearer(request)
    if (!token) return fail('UNAUTHENTICATED', '未认证', 401)
    const username = db.accessTokens.get(token)
    if (!username) return fail('UNAUTHENTICATED', '无效的访问令牌', 401)
    const user = db.users.find((u) => u.username === username)
    if (!user) return fail('UNAUTHENTICATED', '用户不存在', 401)
    const me: AuthMe = { user, roles: db.roles, permissions: db.permissions }
    return ok(me)
  }),

  http.put(`${BASE}/auth/password`, async ({ request }) => {
    const body = (await request.json()) as { old_password?: string; new_password?: string }
    if (body.old_password !== db.passwords.admin) {
      return fail('VALIDATION_FAILED', '当前密码不正确', 422, [
        { field: 'old_password', code: 'INVALID', message: '当前密码不正确' },
      ])
    }
    if (body.new_password) db.passwords.admin = body.new_password
    return ok(null)
  }),

  // —— 字典类型 CRUD（§P1-2）。读端点不在 mock 层做鉴权（鉴权是 UI 守卫职责）——
  http.get(`${BASE}/dict/types`, ({ request }) => {
    const url = new URL(request.url)
    const scope = url.searchParams.get('scope')
    const isActiveParam = url.searchParams.get('is_active')
    let list = db.dictTypes
    if (scope) list = list.filter((t) => t.scope === scope)
    if (isActiveParam !== null) {
      const want = isActiveParam === 'true'
      list = list.filter((t) => t.is_active === want)
    }
    return okList(list)
  }),

  http.post(`${BASE}/dict/types`, async ({ request }) => {
    const body = (await request.json()) as DictTypeCreate
    if (!body.code || !body.name || !body.scope) {
      return fail('VALIDATION_FAILED', '机器键 / 名称 / 作用域必填', 422, [
        {
          field: !body.code ? 'code' : !body.name ? 'name' : 'scope',
          code: 'REQUIRED',
          message: '必填',
        },
      ])
    }
    if (db.dictTypes.some((t) => t.code === body.code)) {
      return fail('VALIDATION_FAILED', '机器键已存在', 422, [
        { field: 'code', code: 'DUPLICATE', message: '机器键已存在' },
      ])
    }
    db.idSeq += 1
    const created: DictType = {
      id: db.idSeq,
      code: body.code,
      name: body.name,
      scope: body.scope,
      description: body.description,
      version: 1,
      content_hash: `hash-${body.code}-1`,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    }
    db.dictTypes = [...db.dictTypes, created]
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  http.put(`${BASE}/dict/types/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.dictTypes.findIndex((t) => t.id === id)
    if (idx < 0) return fail('NOT_FOUND', '字典类型不存在', 404)
    const body = (await request.json()) as DictTypeUpdate
    const prev = db.dictTypes[idx]
    const next: DictType = {
      ...prev,
      ...(body.name !== undefined ? { name: body.name } : {}),
      ...(body.description !== undefined ? { description: body.description } : {}),
      ...(body.is_active !== undefined ? { is_active: body.is_active } : {}),
      version: prev.version + 1,
      content_hash: `${prev.content_hash}-v${prev.version + 1}`,
      updated_at: NOW,
    }
    db.dictTypes = db.dictTypes.map((t) => (t.id === id ? next : t))
    return ok(next)
  }),

  http.delete(`${BASE}/dict/types/:id`, ({ params }) => {
    const id = Number(params.id)
    if (!db.dictTypes.some((t) => t.id === id)) return fail('NOT_FOUND', '字典类型不存在', 404)
    db.dictTypes = db.dictTypes.filter((t) => t.id !== id)
    db.dictItems = db.dictItems.filter((i) => i.dict_type_id !== id)
    return ok(null)
  }),

  // 字典项 ETag 拉取（§P1）：命中 If-None-Match → 304 无体；否则 200 + ETag。退役项一并返回。
  // 数据源优先 mutable db（让 CRUD 反映到 DictTag），否则回退 DICT_FIXTURES。
  http.get(`${BASE}/dict/types/:code/items`, ({ request, params }) => {
    const code = String(params.code)
    const type = db.dictTypes.find((t) => t.code === code)
    if (type) {
      const etag = `"${type.content_hash}"`
      if (request.headers.get('If-None-Match') === etag) {
        return new HttpResponse(null, { status: 304, headers: { ETag: etag } })
      }
      const items = db.dictItems.filter((i) => i.dict_type_id === type.id)
      const payload: DictItemsPayload = {
        type,
        version: type.version,
        content_hash: type.content_hash,
        items,
      }
      return HttpResponse.json(
        { success: true, data: payload, error: null, meta: null },
        { headers: { ETag: etag } },
      )
    }
    const fixture = DICT_FIXTURES[code]
    if (!fixture) return fail('NOT_FOUND', '字典类型不存在', 404)
    const etag = `"${fixture.contentHash}"`
    if (request.headers.get('If-None-Match') === etag) {
      return new HttpResponse(null, { status: 304, headers: { ETag: etag } })
    }
    const payload: DictItemsPayload = {
      type: {
        id: 10,
        code,
        name: code,
        scope: 'problem_status',
        version: fixture.version,
        content_hash: fixture.contentHash,
        is_active: true,
        created_at: NOW,
        updated_at: NOW,
      },
      version: fixture.version,
      content_hash: fixture.contentHash,
      items: fixture.items,
    }
    return HttpResponse.json(
      { success: true, data: payload, error: null, meta: null },
      { headers: { ETag: etag } },
    )
  }),

  // —— 字典项 CRUD（§P1-2）：变更后 bump 所属 dict_type 的 version/content_hash ——
  http.post(`${BASE}/dict/items`, async ({ request }) => {
    const body = (await request.json()) as DictItemCreate
    const type = db.dictTypes.find((t) => t.id === body.dict_type_id)
    if (!type) return fail('NOT_FOUND', '字典类型不存在', 404)
    if (!body.code || !body.label) {
      return fail('VALIDATION_FAILED', '机器键 / 显示名必填', 422, [
        { field: !body.code ? 'code' : 'label', code: 'REQUIRED', message: '必填' },
      ])
    }
    db.idSeq += 1
    const created: DictItem = {
      id: db.idSeq,
      dict_type_id: body.dict_type_id,
      code: body.code,
      label: body.label,
      color: body.color ?? null,
      extra: body.extra,
      sort_order: body.sort_order ?? 0,
      is_active: true,
      created_at: NOW,
      updated_at: NOW,
    }
    db.dictItems = [...db.dictItems, created]
    db.dictTypes = db.dictTypes.map((t) => (t.id === type.id ? bumpDictTypeVersion(t) : t))
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  http.put(`${BASE}/dict/items/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.dictItems.findIndex((i) => i.id === id)
    if (idx < 0) return fail('NOT_FOUND', '字典项不存在', 404)
    const body = (await request.json()) as DictItemUpdate
    const prev = db.dictItems[idx]
    const next: DictItem = {
      ...prev,
      ...(body.label !== undefined ? { label: body.label } : {}),
      ...(body.color !== undefined ? { color: body.color } : {}),
      ...(body.extra !== undefined ? { extra: body.extra } : {}),
      ...(body.sort_order !== undefined ? { sort_order: body.sort_order } : {}),
      ...(body.is_active !== undefined ? { is_active: body.is_active } : {}),
      updated_at: NOW,
    }
    db.dictItems = db.dictItems.map((i) => (i.id === id ? next : i))
    db.dictTypes = db.dictTypes.map((t) =>
      t.id === prev.dict_type_id ? bumpDictTypeVersion(t) : t,
    )
    return ok(next)
  }),

  http.delete(`${BASE}/dict/items/:id`, ({ params }) => {
    const id = Number(params.id)
    const item = db.dictItems.find((i) => i.id === id)
    if (!item) return fail('NOT_FOUND', '字典项不存在', 404)
    db.dictItems = db.dictItems.filter((i) => i.id !== id)
    db.dictTypes = db.dictTypes.map((t) =>
      t.id === item.dict_type_id ? bumpDictTypeVersion(t) : t,
    )
    return ok(null)
  }),

  // —— 系统配置（§P1-2）：GET 带 ETag/304；PUT 产生新版本并写历史 ——
  http.get(`${BASE}/config/:key`, ({ request, params }) => {
    const key = String(params.key)
    const cfg = db.appConfigs.find((c) => c.config_key === key)
    if (!cfg) return fail('NOT_FOUND', '配置不存在', 404)
    const etag = `"${cfg.content_hash}"`
    if (request.headers.get('If-None-Match') === etag) {
      return new HttpResponse(null, { status: 304, headers: { ETag: etag } })
    }
    return HttpResponse.json(
      { success: true, data: cfg, error: null, meta: null },
      { headers: { ETag: etag } },
    )
  }),

  http.put(`${BASE}/config/:key`, async ({ request, params }) => {
    const key = String(params.key)
    const idx = db.appConfigs.findIndex((c) => c.config_key === key)
    if (idx < 0) return fail('NOT_FOUND', '配置不存在', 404)
    const body = (await request.json()) as AppConfigUpdate
    if (!body.value || typeof body.value !== 'object') {
      return fail('VALIDATION_FAILED', '配置值必须为对象', 422, [
        { field: 'value', code: 'INVALID', message: '配置值必须为对象' },
      ])
    }
    const prev = db.appConfigs[idx]
    const next: AppConfig = {
      ...prev,
      value: body.value,
      version: prev.version + 1,
      content_hash: `${prev.content_hash}-v${prev.version + 1}`,
      description: body.description ?? prev.description,
      effective_from: body.effective_from ?? NOW,
      effective_to: null,
      updated_at: NOW,
    }
    db.appConfigs = db.appConfigs.map((c) => (c.config_key === key ? next : c))
    const retiredPrev: AppConfig = { ...prev, is_active: false, effective_to: NOW }
    const history = db.configHistory[key] ?? []
    db.configHistory = {
      ...db.configHistory,
      [key]: [...history.filter((h) => h.version !== prev.version), retiredPrev, next],
    }
    return ok(next)
  }),

  http.get(`${BASE}/config/:key/history`, ({ params }) => {
    const key = String(params.key)
    const history = db.configHistory[key]
    if (!history) return fail('NOT_FOUND', '配置不存在', 404)
    return okList(history)
  }),

  // —— 用户 CRUD（§P2）：软删（从 db.users 移除，列表不再出现）——
  http.get(`${BASE}/users`, ({ request }) => {
    const url = new URL(request.url)
    const q = (url.searchParams.get('keyword') ?? url.searchParams.get('q') ?? '')
      .trim()
      .toLowerCase()
    const page = Number(url.searchParams.get('page') ?? '1')
    const pageSize = Number(url.searchParams.get('page_size') ?? '20')
    let list = db.users
    if (q) {
      list = list.filter(
        (u) => u.username.toLowerCase().includes(q) || u.display_name.toLowerCase().includes(q),
      )
    }
    const total = list.length
    const start = (page - 1) * pageSize
    const pageItems = list.slice(start, start + pageSize)
    return HttpResponse.json({
      success: true,
      data: pageItems,
      error: null,
      meta: { total, page, page_size: pageSize },
    })
  }),

  http.post(`${BASE}/users`, async ({ request }) => {
    const body = (await request.json()) as UserCreate
    if (!body.username || !body.password) {
      return fail('VALIDATION_FAILED', '用户名与初始密码必填', 422, [
        { field: !body.username ? 'username' : 'password', code: 'REQUIRED', message: '必填' },
      ])
    }
    if (db.users.some((u) => u.username.toLowerCase() === body.username.toLowerCase())) {
      return fail('VALIDATION_FAILED', '用户名已存在', 422, [
        { field: 'username', code: 'DUPLICATE', message: '用户名已存在' },
      ])
    }
    db.idSeq += 1
    const created: User = {
      id: db.idSeq,
      username: body.username,
      display_name: body.display_name ?? body.username,
      phone: body.phone ?? null,
      email: body.email ?? null,
      avatar_media_id: null,
      is_active: body.is_active ?? true,
      last_login_at: null,
      created_at: NOW,
      updated_at: NOW,
    }
    db.users = [...db.users, created]
    db.passwords = { ...db.passwords, [body.username]: body.password }
    db.userRoles = { ...db.userRoles, [created.id]: body.role_ids ? [...body.role_ids] : [] }
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  http.get(`${BASE}/users/:id`, ({ params }) => {
    const id = Number(params.id)
    const user = db.users.find((u) => u.id === id)
    if (!user) return fail('NOT_FOUND', '用户不存在', 404)
    return ok(user)
  }),

  http.put(`${BASE}/users/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.users.findIndex((u) => u.id === id)
    if (idx < 0) return fail('NOT_FOUND', '用户不存在', 404)
    const body = (await request.json()) as UserUpdate
    const prev = db.users[idx]
    const next: User = {
      ...prev,
      ...(body.display_name !== undefined ? { display_name: body.display_name } : {}),
      ...(body.phone !== undefined ? { phone: body.phone } : {}),
      ...(body.email !== undefined ? { email: body.email } : {}),
      ...(body.avatar_media_id !== undefined ? { avatar_media_id: body.avatar_media_id } : {}),
      ...(body.is_active !== undefined ? { is_active: body.is_active } : {}),
      updated_at: NOW,
    }
    db.users = db.users.map((u) => (u.id === id ? next : u))
    return ok(next)
  }),

  http.delete(`${BASE}/users/:id`, ({ params }) => {
    const id = Number(params.id)
    if (!db.users.some((u) => u.id === id)) return fail('NOT_FOUND', '用户不存在', 404)
    // 软删：mock 直接从列表移除，列表不再出现
    db.users = db.users.filter((u) => u.id !== id)
    return ok(null)
  }),

  http.put(`${BASE}/users/:id/password`, async ({ request, params }) => {
    const id = Number(params.id)
    const user = db.users.find((u) => u.id === id)
    if (!user) return fail('NOT_FOUND', '用户不存在', 404)
    const body = (await request.json()) as ResetPasswordRequest
    if (!body.new_password) {
      return fail('VALIDATION_FAILED', '新密码必填', 422, [
        { field: 'new_password', code: 'REQUIRED', message: '必填' },
      ])
    }
    db.passwords = { ...db.passwords, [user.username]: body.new_password }
    return ok(null)
  }),

  // —— 用户角色（casbin g-rules）——
  http.get(`${BASE}/users/:id/roles`, ({ params }) => {
    const id = Number(params.id)
    if (!db.users.some((u) => u.id === id)) return fail('NOT_FOUND', '用户不存在', 404)
    const ids = db.userRoles[id] ?? []
    return okList(db.roles.filter((r) => ids.includes(r.id)))
  }),

  http.put(`${BASE}/users/:id/roles`, async ({ request, params }) => {
    const id = Number(params.id)
    if (!db.users.some((u) => u.id === id)) return fail('NOT_FOUND', '用户不存在', 404)
    const body = (await request.json()) as SetUserRolesRequest
    db.userRoles = { ...db.userRoles, [id]: [...body.role_ids] }
    return okList(db.roles.filter((r) => body.role_ids.includes(r.id)))
  }),

  // —— 角色 CRUD（§P2）：is_system 不可删（409 CONFLICT）——
  http.get(`${BASE}/roles`, () => okList(db.roles)),

  http.post(`${BASE}/roles`, async ({ request }) => {
    const body = (await request.json()) as RoleCreate
    if (!body.code || !body.name) {
      return fail('VALIDATION_FAILED', '角色编码与名称必填', 422, [
        { field: !body.code ? 'code' : 'name', code: 'REQUIRED', message: '必填' },
      ])
    }
    if (db.roles.some((r) => r.code === body.code)) {
      return fail('VALIDATION_FAILED', '角色编码已存在', 422, [
        { field: 'code', code: 'DUPLICATE', message: '角色编码已存在' },
      ])
    }
    db.idSeq += 1
    const created: Role = {
      id: db.idSeq,
      code: body.code,
      name: body.name,
      description: body.description,
      is_system: false,
      sort_order: body.sort_order ?? db.roles.length,
      created_at: NOW,
      updated_at: NOW,
    }
    db.roles = [...db.roles, created]
    db.rolePermissions = { ...db.rolePermissions, [created.id]: [] }
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  http.put(`${BASE}/roles/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.roles.findIndex((r) => r.id === id)
    if (idx < 0) return fail('NOT_FOUND', '角色不存在', 404)
    const body = (await request.json()) as RoleUpdate
    const prev = db.roles[idx]
    const next: Role = {
      ...prev,
      ...(body.name !== undefined ? { name: body.name } : {}),
      ...(body.description !== undefined ? { description: body.description } : {}),
      ...(body.sort_order !== undefined ? { sort_order: body.sort_order } : {}),
      updated_at: NOW,
    }
    db.roles = db.roles.map((r) => (r.id === id ? next : r))
    return ok(next)
  }),

  http.delete(`${BASE}/roles/:id`, ({ params }) => {
    const id = Number(params.id)
    const role = db.roles.find((r) => r.id === id)
    if (!role) return fail('NOT_FOUND', '角色不存在', 404)
    if (role.is_system) return fail('CONFLICT', '系统角色受保护，不可删除', 409)
    db.roles = db.roles.filter((r) => r.id !== id)
    const nextRolePerms = { ...db.rolePermissions }
    delete nextRolePerms[id]
    db.rolePermissions = nextRolePerms
    return ok(null)
  }),

  // —— 角色权限（casbin p-rules）——
  http.get(`${BASE}/roles/:id/permissions`, ({ params }) => {
    const id = Number(params.id)
    if (!db.roles.some((r) => r.id === id)) return fail('NOT_FOUND', '角色不存在', 404)
    const ids = db.rolePermissions[id] ?? []
    return okList(db.permissionCatalog.filter((p) => ids.includes(p.id)))
  }),

  http.put(`${BASE}/roles/:id/permissions`, async ({ request, params }) => {
    const id = Number(params.id)
    if (!db.roles.some((r) => r.id === id)) return fail('NOT_FOUND', '角色不存在', 404)
    const body = (await request.json()) as SetRolePermissionsRequest
    db.rolePermissions = { ...db.rolePermissions, [id]: [...body.permission_ids] }
    return okList(db.permissionCatalog.filter((p) => body.permission_ids.includes(p.id)))
  }),

  // —— 权限目录（全量）——
  http.get(`${BASE}/permissions`, () => okList(db.permissionCatalog)),

  // —— 项目 CRUD（§P3）：删除为 RESTRICT，有巡查记录时 409 CONFLICT ——
  http.get(`${BASE}/projects`, ({ request }) => {
    const url = new URL(request.url)
    const status = url.searchParams.get('status')
    const q = (url.searchParams.get('q') ?? '').trim().toLowerCase()
    const page = Number(url.searchParams.get('page') ?? '1')
    const pageSize = Number(url.searchParams.get('page_size') ?? '20')
    let list = db.projects
    if (status) list = list.filter((p) => p.status === status)
    if (q) {
      list = list.filter(
        (p) => p.code.toLowerCase().includes(q) || p.name.toLowerCase().includes(q),
      )
    }
    const total = list.length
    const start = (page - 1) * pageSize
    const pageItems = list.slice(start, start + pageSize)
    return HttpResponse.json({
      success: true,
      data: pageItems,
      error: null,
      meta: { total, page, page_size: pageSize },
    })
  }),

  http.post(`${BASE}/projects`, async ({ request }) => {
    const body = (await request.json()) as ProjectCreate
    if (!body.code || !body.name) {
      return fail('VALIDATION_FAILED', '项目编码与名称必填', 422, [
        { field: !body.code ? 'code' : 'name', code: 'REQUIRED', message: '必填' },
      ])
    }
    if (db.projects.some((p) => p.code === body.code)) {
      return fail('VALIDATION_FAILED', '项目编码已存在', 422, [
        { field: 'code', code: 'DUPLICATE', message: '项目编码已存在' },
      ])
    }
    db.idSeq += 1
    const created: Project = {
      id: db.idSeq,
      code: body.code,
      name: body.name,
      description: body.description,
      status: body.status ?? 'ACTIVE',
      custom_fields: body.custom_fields,
      start_date: body.start_date ?? null,
      end_date: body.end_date ?? null,
      created_at: NOW,
      updated_at: NOW,
    }
    db.projects = [...db.projects, created]
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  http.get(`${BASE}/projects/:id`, ({ params }) => {
    const id = Number(params.id)
    const project = db.projects.find((p) => p.id === id)
    if (!project) return fail('NOT_FOUND', '项目不存在', 404)
    return ok(project)
  }),

  http.put(`${BASE}/projects/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.projects.findIndex((p) => p.id === id)
    if (idx < 0) return fail('NOT_FOUND', '项目不存在', 404)
    const body = (await request.json()) as ProjectUpdate
    const prev = db.projects[idx]
    const next: Project = {
      ...prev,
      ...(body.name !== undefined ? { name: body.name } : {}),
      ...(body.description !== undefined ? { description: body.description } : {}),
      ...(body.status !== undefined ? { status: body.status } : {}),
      ...(body.custom_fields !== undefined ? { custom_fields: body.custom_fields } : {}),
      ...(body.start_date !== undefined ? { start_date: body.start_date } : {}),
      ...(body.end_date !== undefined ? { end_date: body.end_date } : {}),
      updated_at: NOW,
    }
    db.projects = db.projects.map((p) => (p.id === id ? next : p))
    return ok(next)
  }),

  http.delete(`${BASE}/projects/:id`, ({ params }) => {
    const id = Number(params.id)
    if (!db.projects.some((p) => p.id === id)) return fail('NOT_FOUND', '项目不存在', 404)
    // RESTRICT：存在巡查记录则拒绝删除（409 CONFLICT），与后端 inspections.project_id RESTRICT 一致。
    if (db.inspections.some((i) => i.project_id === id)) {
      return fail('CONFLICT', '该项目存在巡查记录，无法删除', 409)
    }
    db.projects = db.projects.filter((p) => p.id !== id)
    db.tasks = db.tasks.filter((tk) => tk.project_id !== id)
    return ok(null)
  }),

  // —— 任务 CRUD（§P3）：?project_id=&status=&assignee_id= ——
  http.get(`${BASE}/tasks`, ({ request }) => {
    const url = new URL(request.url)
    const projectId = url.searchParams.get('project_id')
    const status = url.searchParams.get('status')
    const assigneeId = url.searchParams.get('assignee_id')
    let list = db.tasks
    if (projectId) list = list.filter((tk) => tk.project_id === Number(projectId))
    if (status) list = list.filter((tk) => tk.status === status)
    if (assigneeId) list = list.filter((tk) => tk.assignee_id === Number(assigneeId))
    return okList(list)
  }),

  http.post(`${BASE}/tasks`, async ({ request }) => {
    const body = (await request.json()) as InspectionTaskCreate
    if (!body.project_id || !body.title) {
      return fail('VALIDATION_FAILED', '项目与任务标题必填', 422, [
        { field: !body.project_id ? 'project_id' : 'title', code: 'REQUIRED', message: '必填' },
      ])
    }
    db.idSeq += 1
    const created: InspectionTask = {
      id: db.idSeq,
      project_id: body.project_id,
      title: body.title,
      description: body.description,
      status: body.status ?? 'PENDING',
      assignee_id: body.assignee_id ?? null,
      planned_start: body.planned_start ?? null,
      planned_end: body.planned_end ?? null,
      created_at: NOW,
      updated_at: NOW,
    }
    db.tasks = [...db.tasks, created]
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  http.put(`${BASE}/tasks/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.tasks.findIndex((tk) => tk.id === id)
    if (idx < 0) return fail('NOT_FOUND', '任务不存在', 404)
    const body = (await request.json()) as InspectionTaskUpdate
    const prev = db.tasks[idx]
    const next: InspectionTask = {
      ...prev,
      ...(body.title !== undefined ? { title: body.title } : {}),
      ...(body.description !== undefined ? { description: body.description } : {}),
      ...(body.status !== undefined ? { status: body.status } : {}),
      ...(body.assignee_id !== undefined ? { assignee_id: body.assignee_id } : {}),
      ...(body.planned_start !== undefined ? { planned_start: body.planned_start } : {}),
      ...(body.planned_end !== undefined ? { planned_end: body.planned_end } : {}),
      updated_at: NOW,
    }
    db.tasks = db.tasks.map((tk) => (tk.id === id ? next : tk))
    return ok(next)
  }),

  http.delete(`${BASE}/tasks/:id`, ({ params }) => {
    const id = Number(params.id)
    if (!db.tasks.some((tk) => tk.id === id)) return fail('NOT_FOUND', '任务不存在', 404)
    db.tasks = db.tasks.filter((tk) => tk.id !== id)
    return ok(null)
  }),

  // —— 巡查记录（§P3，Web 只读）：?project_id=&inspector_id=&status=&from=&to=&page=&page_size= ——
  http.get(`${BASE}/inspections`, ({ request }) => {
    const url = new URL(request.url)
    const projectId = url.searchParams.get('project_id')
    const inspectorId = url.searchParams.get('inspector_id')
    const status = url.searchParams.get('status')
    const from = url.searchParams.get('from')
    const to = url.searchParams.get('to')
    const page = Number(url.searchParams.get('page') ?? '1')
    const pageSize = Number(url.searchParams.get('page_size') ?? '20')
    let list = db.inspections
    if (projectId) list = list.filter((i) => i.project_id === Number(projectId))
    if (inspectorId) list = list.filter((i) => i.inspector_id === Number(inspectorId))
    if (status) list = list.filter((i) => i.status === status)
    if (from) list = list.filter((i) => i.started_at >= from)
    if (to) list = list.filter((i) => i.started_at <= to)
    const total = list.length
    const start = (page - 1) * pageSize
    const pageItems = list.slice(start, start + pageSize)
    return HttpResponse.json({
      success: true,
      data: pageItems,
      error: null,
      meta: { total, page, page_size: pageSize },
    })
  }),

  http.get(`${BASE}/inspections/:id`, ({ params }) => {
    const id = Number(params.id)
    const inspection = db.inspections.find((i) => i.id === id)
    if (!inspection) return fail('NOT_FOUND', '巡查记录不存在', 404)
    return ok(inspection)
  }),

  // —— 轨迹（§P4，WGS84）：FINISHED 返回 route + points；IN_PROGRESS 的 route 为 null、points 空 ——
  http.get(`${BASE}/inspections/:id/trajectory`, ({ params }) => {
    const id = Number(params.id)
    const inspection = db.inspections.find((i) => i.id === id)
    if (!inspection) return fail('NOT_FOUND', '巡查记录不存在', 404)
    const trajectory: Trajectory = {
      inspection_id: id,
      route: inspection.route_geom,
      points: db.trajectoryPoints[id] ?? [],
    }
    return ok(trajectory)
  }),

  // —— 问题点地图（§P4，GeoJSON FeatureCollection，WGS84）：按 inspection_id / project_id / 字典项过滤 ——
  http.get(`${BASE}/problems/map`, ({ request }) => {
    const url = new URL(request.url)
    const projectId = url.searchParams.get('project_id')
    const inspectionId = url.searchParams.get('inspection_id')
    const type = url.searchParams.get('type')
    const status = url.searchParams.get('status')
    const category = url.searchParams.get('category')
    let list = db.problemFeatures
    if (projectId) list = list.filter((p) => p.project_id === Number(projectId))
    if (inspectionId) list = list.filter((p) => p.inspection_id === Number(inspectionId))
    if (type) list = list.filter((p) => p.feature.properties.type_item_id === Number(type))
    if (status) list = list.filter((p) => p.feature.properties.status_item_id === Number(status))
    if (category)
      list = list.filter((p) => p.feature.properties.category_item_id === Number(category))
    const features: ProblemFeature[] = list.map((p) => p.feature)
    const collection: ProblemFeatureCollection = { type: 'FeatureCollection', features }
    return ok(collection)
  }),

  // —— 问题列表（§P6，D1）：?project_id=&type=&status=&category=&from=&to=&inspector_id=&inspection_id=&page=&page_size= ——
  http.get(`${BASE}/problems`, ({ request }) => {
    const url = new URL(request.url)
    const projectId = url.searchParams.get('project_id')
    const type = url.searchParams.get('type')
    const status = url.searchParams.get('status')
    const category = url.searchParams.get('category')
    const inspectorId = url.searchParams.get('inspector_id')
    const inspectionId = url.searchParams.get('inspection_id')
    const from = url.searchParams.get('from')
    const to = url.searchParams.get('to')
    const page = Number(url.searchParams.get('page') ?? '1')
    const pageSize = Number(url.searchParams.get('page_size') ?? '20')
    let list = db.problems
    if (projectId) list = list.filter((p) => p.project_id === Number(projectId))
    if (type) list = list.filter((p) => p.type_item_id === Number(type))
    if (status) list = list.filter((p) => p.status_item_id === Number(status))
    if (category) list = list.filter((p) => p.category_item_id === Number(category))
    if (inspectorId) list = list.filter((p) => p.inspector_id === Number(inspectorId))
    if (inspectionId) list = list.filter((p) => p.inspection_id === Number(inspectionId))
    if (from) list = list.filter((p) => p.captured_at >= from)
    if (to) list = list.filter((p) => p.captured_at <= to)
    const total = list.length
    const start = (page - 1) * pageSize
    const pageItems = list.slice(start, start + pageSize)
    return HttpResponse.json({
      success: true,
      data: pageItems,
      error: null,
      meta: { total, page, page_size: pageSize },
    })
  }),

  http.get(`${BASE}/problems/:id`, ({ params }) => {
    const id = Number(params.id)
    const problem = db.problems.find((p) => p.id === id)
    if (!problem) return fail('NOT_FOUND', '问题不存在', 404)
    return ok(problem)
  }),

  // —— 改问题（§P6 / D3 强约束）：改 status_item_id 时，在「同一事务内」原子追加 STATUS_CHANGE 日志 ——
  http.put(`${BASE}/problems/:id`, async ({ request, params }) => {
    const id = Number(params.id)
    const idx = db.problems.findIndex((p) => p.id === id)
    if (idx < 0) return fail('NOT_FOUND', '问题不存在', 404)
    const body = (await request.json()) as ProblemUpdate
    const prev = db.problems[idx]
    const next: Problem = {
      ...prev,
      ...(body.inspection_id !== undefined ? { inspection_id: body.inspection_id } : {}),
      ...(body.type_item_id !== undefined ? { type_item_id: body.type_item_id } : {}),
      ...(body.status_item_id !== undefined ? { status_item_id: body.status_item_id } : {}),
      ...(body.category_item_id !== undefined ? { category_item_id: body.category_item_id } : {}),
      ...(body.title !== undefined ? { title: body.title } : {}),
      ...(body.description !== undefined ? { description: body.description } : {}),
      ...(body.note !== undefined ? { note: body.note } : {}),
      ...(body.cover_media_id !== undefined ? { cover_media_id: body.cover_media_id } : {}),
      updated_at: NOW,
    }
    db.problems = db.problems.map((p) => (p.id === id ? next : p))

    // D3：状态变化 → 后端在同一事务内追加 STATUS_CHANGE（含 from/to）。前端绝不 POST 此动作。
    if (body.status_item_id !== undefined && body.status_item_id !== prev.status_item_id) {
      db.idSeq += 1
      const statusLog: ProblemProcessingLog = {
        id: db.idSeq,
        problem_id: id,
        action: 'STATUS_CHANGE',
        from_status_item_id: prev.status_item_id ?? null,
        to_status_item_id: body.status_item_id ?? null,
        note: undefined,
        operator_id: 1,
        created_at: NOW,
      }
      const existing = db.problemLogs[id] ?? []
      db.problemLogs = { ...db.problemLogs, [id]: [...existing, statusLog] }
    }
    return ok(next)
  }),

  http.delete(`${BASE}/problems/:id`, ({ params }) => {
    const id = Number(params.id)
    if (!db.problems.some((p) => p.id === id)) return fail('NOT_FOUND', '问题不存在', 404)
    db.problems = db.problems.filter((p) => p.id !== id)
    return ok(null)
  }),

  // —— 处理记录（§P6）：追加式审计；GET 列表 + POST 仅接受 COMMENT/REASSIGN（D3） ——
  http.get(`${BASE}/problems/:id/logs`, ({ params }) => {
    const id = Number(params.id)
    if (!db.problems.some((p) => p.id === id)) return fail('NOT_FOUND', '问题不存在', 404)
    const logs = db.problemLogs[id] ?? []
    return okList(logs)
  }),

  http.post(`${BASE}/problems/:id/logs`, async ({ request, params }) => {
    const id = Number(params.id)
    if (!db.problems.some((p) => p.id === id)) return fail('NOT_FOUND', '问题不存在', 404)
    const body = (await request.json()) as ProblemLogCreate
    // D3：STATUS_CHANGE 是后端独写，前端 POST 必须拒绝（422）。仅接受 COMMENT/REASSIGN。
    if ((body.action as string) === 'STATUS_CHANGE') {
      return fail('VALIDATION_FAILED', '状态变更由系统自动记录，请通过修改状态触发', 422, [
        { field: 'action', code: 'FORBIDDEN_ACTION', message: '不允许直接提交状态变更日志' },
      ])
    }
    if (body.action !== 'COMMENT' && body.action !== 'REASSIGN') {
      return fail('VALIDATION_FAILED', '不支持的处理动作', 422, [
        { field: 'action', code: 'INVALID', message: '动作仅支持备注或改派' },
      ])
    }
    if (body.action === 'REASSIGN' && body.operator_id == null) {
      return fail('VALIDATION_FAILED', '改派需指定负责人', 422, [
        { field: 'operator_id', code: 'REQUIRED', message: '请选择改派的负责人' },
      ])
    }
    db.idSeq += 1
    const created: ProblemProcessingLog = {
      id: db.idSeq,
      problem_id: id,
      action: body.action,
      from_status_item_id: null,
      to_status_item_id: null,
      note: body.note,
      operator_id: body.operator_id ?? null,
      created_at: NOW,
    }
    const existing = db.problemLogs[id] ?? []
    db.problemLogs = { ...db.problemLogs, [id]: [...existing, created] }
    return HttpResponse.json(
      { success: true, data: created, error: null, meta: null },
      { status: 201 },
    )
  }),

  // —— 媒体（§P5，Web 只读）：GET /media/{id} 返回按 tier 的签名 CDN URL。每次返回新 signed_url，
  //    模拟「签名有时效、重拉取新」（PanoramaViewer.onExpired → refetch）。读端点不做鉴权（UI 守卫职责）。——
  http.get(`${BASE}/media/:id`, ({ params }) => {
    const id = Number(params.id)
    const found = db.media.find((m) => m.id === id)
    if (!found) return fail('NOT_FOUND', '媒体不存在', 404)
    // 已确认媒体每次注入一个带随机签名片段的新 URL（仅当原本就有 signed_url，即 CONFIRMED）。
    const media: MediaAsset =
      found.signed_url != null
        ? { ...found, signed_url: `${found.signed_url}&t=${db.counter++}` }
        : found
    return ok(media)
  }),

  // —— 统计概览（§P7 / D2）：?project_id=&from=&to=&inspector_id=（全可选）。聚合在后端，前端仅渲染。
  //    按筛选派生新对象：project_id/inspector_id 选中时收窄对应维度桶并按比例缩放核心指标，from/to 进一步
  //    缩放 —— 让测试可断言「筛选改变 → 查询串 + 重渲染」。counts_by_type 含退役类型桶（历史容忍）。——
  http.get(`${BASE}/stats/overview`, ({ request }) => {
    const url = new URL(request.url)
    const projectId = url.searchParams.get('project_id')
    const inspectorId = url.searchParams.get('inspector_id')
    const from = url.searchParams.get('from')
    const to = url.searchParams.get('to')

    const base = db.statsOverview
    // 缩放因子：每个生效筛选都让总量更小（模拟更窄的数据切片），便于断言 re-render。
    let factor = 1
    if (projectId) factor *= 0.5
    if (inspectorId) factor *= 0.6
    if (from) factor *= 0.85
    if (to) factor *= 0.85
    const scale = (n: number): number => Math.round(n * factor)

    const overview: StatsOverview = {
      inspection_count: scale(base.inspection_count),
      problem_count: scale(base.problem_count),
      total_mileage_meters: Math.round(base.total_mileage_meters * factor * 10) / 10,
      total_duration_seconds: scale(base.total_duration_seconds),
      avg_duration_seconds: base.avg_duration_seconds,
      counts_by_type: base.counts_by_type.map((b) => ({ ...b, count: scale(b.count) })),
      counts_by_status: base.counts_by_status.map((b) => ({ ...b, count: scale(b.count) })),
      // inspector 筛选时只保留命中的人员桶。
      counts_by_inspector: (inspectorId
        ? base.counts_by_inspector.filter((b) => b.inspector_id === Number(inspectorId))
        : base.counts_by_inspector
      ).map((b) => ({ ...b, count: scale(b.count) })),
      // project 筛选时只保留命中的项目桶。
      counts_by_project: (projectId
        ? base.counts_by_project.filter((b) => b.project_id === Number(projectId))
        : base.counts_by_project
      ).map((b) => ({ ...b, count: scale(b.count) })),
    }
    return ok(overview)
  }),

  // —— 数据导出（P8）——

  // POST /exports：入队异步导出任务，返回 201 PENDING。
  http.post(`${BASE}/exports`, async ({ request }) => {
    const body = (await request.json()) as ExportJobCreate
    const type = body.type as ExportType
    const job = createExportJob(type, body.params)
    return HttpResponse.json({ success: true, data: job, error: null, meta: null }, { status: 201 })
  }),

  // GET /exports/:job_uuid：轮询门面 —— 每次调用推进进度，重复 GET 直到 SUCCEEDED。
  http.get(`${BASE}/exports/:jobUuid`, ({ params }) => {
    const jobUuid = String(params.jobUuid)
    const job = advanceExportJob(jobUuid)
    if (!job) return fail('NOT_FOUND', '导出任务不存在', 404)
    return ok(job)
  }),

  // GET /exports/:job_uuid/events：手写 SSE（text/event-stream）。
  // 默认发两帧 progress + 一帧 done(SUCCEEDED, result_url)。
  // 测试可用 ?fail=1 强制流提前出错（500），以驱动轮询回退路径。
  http.get(`${BASE}/exports/:jobUuid/events`, ({ request, params }) => {
    const url = new URL(request.url)
    if (url.searchParams.get('fail') === '1') {
      return new HttpResponse(null, { status: 500 })
    }
    const jobUuid = String(params.jobUuid)
    const encoder = new TextEncoder()
    const frames = [
      'event: progress\ndata: {"progress":40,"processed_rows":40,"total_rows":100,"status":"RUNNING"}\n\n',
      'event: progress\ndata: {"progress":80,"processed_rows":80,"total_rows":100,"status":"RUNNING"}\n\n',
      `event: done\ndata: {"status":"SUCCEEDED","result_url":"${EXPORT_RESULT_URL}"}\n\n`,
    ]
    // 同步把 job 推进到 SUCCEEDED，使 SSE 与轮询监听同一对象（终态一致）。
    let job = db.exportJobs.get(jobUuid)
    while (job && job.status !== 'SUCCEEDED') job = advanceExportJob(jobUuid)

    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const frame of frames) controller.enqueue(encoder.encode(frame))
        controller.close()
      },
    })
    return new HttpResponse(stream, {
      status: 200,
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      },
    })
  }),
]

import type { components } from './generated/openapi-types'
import type { ApiErrorCode } from './apiError'

/**
 * 类型门面：所有接口类型从生成的 openapi-types 派生（禁止手写接口类型，§2）。
 * 业务层 import 这里的别名，而非直接深挖 generated。
 */
export type Schemas = components['schemas']

// —— 信封 / 通用 ——
export type Meta = Schemas['Meta']
export type ErrorObject = Schemas['ErrorObject']
export type ErrorDetail = Schemas['ErrorDetail']
export type ErrorCode = Schemas['ErrorCode']

export interface Envelope<T> {
  success: boolean
  data: T | null
  error: ErrorObject | null
  meta: Meta | null
}

/** 列表端点统一分页形状（拦截器/门面把 meta 规整为此结构）。 */
export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

// —— 身份 / RBAC ——
export type User = Schemas['User']
export type Role = Schemas['Role']
export type Permission = Schemas['Permission']
export type AuthTokens = Schemas['AuthTokens']
export type AuthMe = Schemas['AuthMe']
export type LoginRequest = Schemas['LoginRequest']
export type RefreshRequest = Schemas['RefreshRequest']
export type ChangePasswordRequest = Schemas['ChangePasswordRequest']

// —— RBAC 增删改（P2）——
export type UserCreate = Schemas['UserCreate']
export type UserUpdate = Schemas['UserUpdate']
export type ResetPasswordRequest = Schemas['ResetPasswordRequest']
export type RoleCreate = Schemas['RoleCreate']
export type RoleUpdate = Schemas['RoleUpdate']
export type SetRolePermissionsRequest = Schemas['SetRolePermissionsRequest']
export type SetUserRolesRequest = Schemas['SetUserRolesRequest']

// —— 项目 / 任务 / 巡查（P3）——
export type Project = Schemas['Project']
export type ProjectCreate = Schemas['ProjectCreate']
export type ProjectUpdate = Schemas['ProjectUpdate']
export type ProjectStatus = Schemas['ProjectStatus']
export type InspectionTask = Schemas['InspectionTask']
export type InspectionTaskCreate = Schemas['InspectionTaskCreate']
export type InspectionTaskUpdate = Schemas['InspectionTaskUpdate']
export type TaskStatus = Schemas['TaskStatus']
export type Inspection = Schemas['Inspection']
export type InspectionStatus = Schemas['InspectionStatus']

// —— 媒体 / 全景（P5）——
export type MediaAsset = Schemas['MediaAsset']
export type MediaTier = Schemas['MediaTier']
export type CaptureState = Schemas['CaptureState']
export type MediaOwnerType = Schemas['MediaOwnerType']

// —— 地图 / 轨迹 / 问题点（P4）——
export type Trajectory = Schemas['Trajectory']
export type TrajectoryPoint = Schemas['TrajectoryPoint']
export type ProblemFeatureCollection = Schemas['ProblemFeatureCollection']
export type ProblemFeature = Schemas['ProblemFeature']
export type ProblemFeatureProperties = Schemas['ProblemFeatureProperties']
export type GeoJSONLineString = Schemas['GeoJSONLineString']
export type GeoJSONPoint = Schemas['GeoJSONPoint']

// —— 问题管理（P6）——
export type Problem = Schemas['Problem']
export type ProblemCreate = Schemas['ProblemCreate']
export type ProblemUpdate = Schemas['ProblemUpdate']
export type ProblemProcessingLog = Schemas['ProblemProcessingLog']
export type ProblemLogCreate = Schemas['ProblemLogCreate']
/** 处理记录动作全集（含后端独写 STATUS_CHANGE）。 */
export type ProcessingAction = Schemas['ProcessingAction']
/** 前端可 POST 的动作子集（仅 COMMENT/REASSIGN，D3 强约束，类型层面排除 STATUS_CHANGE）。 */
export type ClientLogAction = Schemas['ClientLogAction']

// —— 字典 / 配置（P1）——
export type DictType = Schemas['DictType']
export type DictItem = Schemas['DictItem']
export type DictItemsPayload = Schemas['DictItemsPayload']
export type DictScope = Schemas['DictScope']
export type DictTypeCreate = Schemas['DictTypeCreate']
export type DictTypeUpdate = Schemas['DictTypeUpdate']
export type DictItemCreate = Schemas['DictItemCreate']
export type DictItemUpdate = Schemas['DictItemUpdate']
export type AppConfig = Schemas['AppConfig']
export type AppConfigUpdate = Schemas['AppConfigUpdate']

// —— 数据统计（P7，D2 §4.7 单一权威形状）——
export type StatsOverview = Schemas['StatsOverview']
/** 计数桶：item_id?/inspector_id?/project_id? + label + color?(dict_item.color，含退役项) + count。 */
export type CountBucket = Schemas['CountBucket']

// —— 数据导出（P8）——
/** 导出任务（export_jobs）：job_uuid 公开句柄 + status/progress/processed_rows/result_url 等。 */
export type ExportJob = Schemas['ExportJob']
/** 创建导出入参：{type, params?}（params = 当前页筛选条件，所见即所导）。 */
export type ExportJobCreate = Schemas['ExportJobCreate']
/** 导出类型全集（三种全含，PROJECT_STATS 不可省略）。 */
export type ExportType = Schemas['ExportType']
/** 任务状态机：PENDING|RUNNING|SUCCEEDED|FAILED|CANCELLED。 */
export type JobStatus = Schemas['JobStatus']

/**
 * 编译期断言：手写的 ApiErrorCode 必须与生成的 ErrorCode 完全一致（双向子集）。
 * 若后端 spec 改动错误码而前端 union 未同步，此处会编译失败。
 */
type Equal<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false
type AssertTrue<T extends true> = T
export type _ErrorCodeParity = AssertTrue<Equal<ApiErrorCode, ErrorCode>>

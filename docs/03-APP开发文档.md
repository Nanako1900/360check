# C5「360相机巡查标注系统」Android APP 开发文档

> 版本：v1.0（最后构建层）·语言：中文 ·目标平台：Android（Insta360 X3 + 腾讯地图）
> 状态：契约已冻结（OpenAPI / 数据字典 / 同步契约 / 坐标系约定均为只读引用）

---

## 1. 文档说明 / 范围 / 读者

### 1.1 文档目的
本文档是 C5 项目 Android 巡查 APP 的**唯一权威开发文档**，供 Android 团队按阶段（Phase）执行落地。APP 是 C5 三端（Backend → Web → App）中**最后构建、技术约束最重**的一层：它直接对接 Insta360 X3 相机硬件（私有 SDK）、消费已冻结的后端 OpenAPI 契约，并要求在弱网/离线现场稳定工作。

### 1.2 范围（本端覆盖的需求）
覆盖功能清单 APP 全部 7 项：

| 编号 | 功能 | 落位 Phase |
|---|---|---|
| 1 | 登录（账号密码/用户信息/退出） | P1 |
| 2 | 项目/巡查任务（列表/新建/详情/开始/结束巡查） | P2 |
| 3 | 相机对接（连接/状态/拍照/取全景/本地缓存/上传） | P3 / P6 |
| 4 | GPS 轨迹（开始/结束/路线/当前位置/结束后上传） | P4 |
| 5 | 问题拍照标注（发现/360拍照/自动定位/地图点位/描述/类型/上传） | P5 |
| 6 | 地图展示（当前位置/轨迹/问题点/点击详情/看全景/看描述） | P7 |
| 7 | 数据同步（上传照片/轨迹/问题/网络异常本地暂存/恢复重传） | P6 |

**明确不在本端范围**：Web 后台管理、Excel 导出（三种 Excel —— `INSPECTION_RECORDS` / `PROBLEM_LIST` / `PROJECT_STATS` —— 全部为 Web 端职责，APP 不产生 `export_jobs`）、统计图表、RBAC 策略编辑（仅消费 `auth/me` 的权限码做 UI 显隐，不做策略管理）。

**明确不在 v1 范围（五-2 决议）**：**实时预览 360 画面（live preview）** 不在 v1 范围（仅拍照、降低难度）。Demo 中的 record/live/直播分支一律剔除（P3 line "本项目仅拍照"）。拍照后的**本地** `InstaImagePlayerView` 回看（已落盘的全景 JPG / `.insp`）**不属于** live preview，是允许的（见 P3/P7）。录像、HDR-only 专用流程同样 OUT。

### 1.2.1 需求追溯表（客户功能清单 → 落位 Phase/任务）
> 客户《360相机巡查标注系统功能清单》为唯一权威来源；APP 全部 7 项功能逐项可追溯。

| 客户清单条目 | 子点 | 交付 Phase / 任务 |
|---|---|---|
| 二-1 用户登录 | 账号密码登录 / 用户信息展示 / 退出登录 | P1（`auth/login`、`auth/me`、`auth/logout`、`auth/password`） |
| 二-2 项目/巡查任务 | 项目列表 / 任务列表 / 新建任务 / 任务详情 / 开始巡查 / 结束巡查 | P2（`projects`、`tasks`、`inspections/start`、`inspections/{id}/finish` + 会话状态机） |
| 二-3 影石360对接 | 对接SDK / 搜索连接 / 显示连接状态 / 调用拍照 / 获取全景 / 本地缓存 | P3a（连接）+ P3b（拍照）+ P3c（下载`.insp`）+ P3d（拼接） |
| 二-3 影石360对接 | 照片上传到平台 | P6（STS+COS 分块可续传） |
| 二-4 巡查轨迹记录 | 记录GPS轨迹 / 开始时间 / 结束时间 / 巡查路线 / 当前位置 | P4（FusedLocation + `TrajectoryForegroundService`） |
| 二-4 巡查轨迹记录 | 巡查结束后上传轨迹数据 | P6（`sync/batch` 批量 upsert） |
| 二-5 问题拍照标注 | 发现问题 / 360拍摄 / 自动定位 / 地图生成点位 / 填写描述 / 选择类型 / 上传坐标/备注 | P5（问题采集 + 自动定位 + 地图落点 + 动态字典表单） |
| 二-5 问题拍照标注 | 上传问题照片/坐标/备注 | P6（媒体 STS+COS、问题 `sync/batch`） |
| 二-6 地图展示 | 当前位置 / 巡查轨迹 / 问题点位 / 点击查看详情 / 查看全景 / 查看描述备注 | P7（腾讯地图 SDK + `InstaImagePlayerView` 360 查看） |
| 二-7 数据同步 | 照片上传 / 轨迹上传 / 问题点上传 / 网络异常本地暂存 / 网络恢复重新上传 | P6（Room outbox + WorkManager + `client_uuid` 幂等 + 可续传 COS） |

### 1.2.2 MVP 范围标注（客户清单 四·简化版 MVP → Phase）
> 客户清单"四、简化版 MVP"的 APP 子集：用户登录 / 项目列表 / 创建巡查任务 / 开始巡查 / 结束巡查 / GPS轨迹记录 / 连接影石360相机 / 调用相机拍照 / 填写问题描述 / 自动记录问题坐标 / 上传照片轨迹问题数据。

| MVP 叶子功能 | 交付 Phase |
|---|---|
| 用户登录 | **P1** |
| 项目列表 / 创建巡查任务 / 开始巡查 / 结束巡查 | **P2** |
| 连接影石360相机 / 调用相机拍照（端上拼接） | **P3** |
| GPS 轨迹记录 | **P4** |
| 填写问题描述 / 自动记录问题坐标 | **P5** |
| 上传照片/轨迹/问题数据 | **P6** |

- **MVP = APP P1–P6**。P0 为横切 enabler（脚手架/网络分离，非清单功能）。
- **P7（地图富展示 / 360 交互查看 / 上架打磨）超出严格 MVP**，但 360 交互查看（五-5 决议）与地图展示属 v1 完整范围，列为 P7。
- live preview、录像、HDR-only 既不在 MVP 也不在 v1 范围。

### 1.3 读者
- Android 工程师（主开发）：按 Phase 任务清单执行。
- 技术负责人 / Reviewer：以各 Phase 的 DoD（验收标准）与测试要求做关卡评审。
- 后端 / Web 工程师：作为联调契约对照（第 4 节）。

### 1.4 阅读前置
读者须先通读项目根上下文中的 **数据字典（Data Dictionary）**、**API 契约（API Contract）**、**已验证约束（KEY VERIFIED CONSTRAINTS）**。本文档对其中的表名、接口名、状态机名一律**逐字引用**，不重新定义。

### 1.4.1 客户清单 五·需要确认问题 —— 已决议（APP 视角）
> 这 5 项不确定点已全部冻结，APP 据此实现，不再回头讨论。

| 编号 | 待确认项 | 决议（APP 落地） |
|---|---|---|
| 五-1 | Insta360 SDK 能力（连接/控制拍照/取图/取全景文件/指定型号） | **Insta360 SDK v1.10.1，X3，PHOTOS ONLY**。流程：`openCamera(WIFI)`→`setNetIdToCamera`→`switchPanoramaSensorMode`→`startNormalCapture(gpsBytes)`/`startHDRCapture`→`WorkUtils.getAllCameraWorks`→`.insp` 下载（socket 绑定）→`ExportUtils.exportImage(PANORAMA,2:1)`。**X3 跳过 `initCameraSupportConfig`（仅 X4+）**。 |
| 五-2 | 是否需要实时预览 360 画面 | **否，v1 不做 live preview**（仅拍照，降低难度）。P3 剔除 record/live/取景流；本地 `InstaImagePlayerView` 看已落盘全景**不算** live preview。 |
| 五-3 | 地图服务 高德/百度/腾讯 | **腾讯地图 Android SDK**（P7）。GCJ-02 仅渲染边界，WGS84 持久化；唯一转换器在 `:core:geo`，单测覆盖。 |
| 五-4 | 是否需要离线巡查 | **是，离线优先**（P6）。Room outbox + WorkManager + `client_uuid` 幂等 + STS/COS 可续传 + `/sync/batch` 逐项 SAVEPOINT（后端）。 |
| 五-5 | 网页端是否需要 360 拖动查看 | **是**（Web 用 PSV v5）。APP 端用原生 `InstaImagePlayerView`；两端消费同一 equirectangular + GPano（D5）。 |

### 1.5 参考工程
唯一参考物是 `Android-SDK-V1.10.1/sdkdemo`（Insta360 官方 SDK Demo，包名 `com.arashivision.sdk.demo`）。我们采用**净室（clean-room）方式**从中提取相机交互范式与工具链，但**不直接搬运其架构**（Demo 无 Hilt/Room/Retrofit，且使用了被本项目明令拒绝的 `bindProcessToNetwork(cameraNet)` 全进程绑定写法 —— 见 P0）。

---

## 2. 技术栈与版本

工具链版本以参考 Demo 实测为基线（已验证可编译运行），并在其上叠加生产所需库。

### 2.1 构建与语言（与 Demo 一致，已验证）
| 项 | 版本 | 来源 |
|---|---|---|
| AGP | 8.9.0 | `libs.versions.toml` |
| Gradle Wrapper | 8.11.1 | `gradle-wrapper.properties` |
| Kotlin | 2.0.21 | 同上 |
| Java / JVM Target | 17 | `app/build.gradle.kts` |
| compileSdk / targetSdk | 35 | 同上 |
| minSdk | 29 | 同上（受 SDK 限制，不可下调） |
| ABI | **arm64-v8a only** | `ndk.abiFilters`（X3 SDK 仅含 arm64 .so） |
| multiDex | enabled | 同上 |

### 2.2 Insta360 SDK（核心，私有 Maven，不可替换）
| 库 | 坐标 | 版本 |
|---|---|---|
| 相机控制 | `com.arashivision.sdk:sdkcamera` | 1.10.1 |
| 媒体/拼接/播放 | `com.arashivision.sdk:sdkmedia` | 1.10.1 |

> 包含 native `.so`（`libc++_shared.so` 需 `pickFirsts`，部分 SNPE skel `.so` 需 `excludes`，照搬 Demo 的 `packaging{}` 配置）。SDK 自带一个**内部 OkGo（`com.arashivision.sdkcamera.okgo`）**用于相机 HTTP 下载 —— 这是与业务 Retrofit/OkHttp 完全独立的栈。

### 2.3 架构与 DI / 持久化（本项目新增）
| 能力 | 库 | 版本 | 说明 |
|---|---|---|---|
| DI | Hilt + KSP | `com.google.dagger:hilt-android` 2.52 / KSP 2.0.21-1.0.28 | **用 KSP，不用 kapt**（Demo 用 kapt 仅为 Glide，本项目统一迁 KSP） |
| 本地库（离线 outbox） | Room | 2.6.1（+ KSP processor） | 巡查/轨迹/问题/媒体 outbox + 字典缓存 |
| 后台任务 | WorkManager | 2.9.1 | 可续传上传、同步重试 |
| 协程 | kotlinx-coroutines | 1.8.1（升级 Demo 的 1.6.4） | |
| Lifecycle / ViewModel | androidx.lifecycle | 2.8.x | |

### 2.4 网络（业务侧，独立于相机栈）
| 能力 | 库 | 版本 |
|---|---|---|
| HTTP | OkHttp | 4.12.0 |
| REST | Retrofit | 2.11.0 |
| JSON | Moshi + moshi-kotlin-codegen（KSP） | 1.15.1 |
| Retrofit 客户端代码 | **openapi-generator 生成的 Kotlin/Retrofit 客户端**（`-g kotlin --library jvm-retrofit2`，来自后端冻结的同一份 OpenAPI spec） | — |
| COS 上传 | `com.qcloud.cos:cos-android` + STS 临时密钥 | **5.9.20**（锁定；支持 multipart 续传） |

### 2.5 UI / 地图 / 图片
| 能力 | 库 | 版本 | 说明 |
|---|---|---|---|
| UI 范式 | **Views + Fragments + ViewBinding + Navigation**（单 Activity，**禁用 Compose**） | androidx.navigation 2.7.x | |
| Material | com.google.android.material | 1.12.0 | |
| 约束布局 | androidx.constraintlayout | 2.1.4 | |
| 地图 | **腾讯地图 Android SDK**（`com.tencent.map:tencent-map-vector-sdk`） | **5.4.0**（锁定，含定位组件 `tencent-location` 8.x） | 仅在渲染边界做 WGS84↔GCJ-02 |
| 图片 | Glide | 4.16.0（+ KSP） | |
| 360 查看 | **`InstaImagePlayerView`（sdkmedia 自带）** | 1.10.1 | APP 端用 SDK 原生播放器看本地 equirectangular JPG / `.insp` |
| 权限 | XXPermissions | 20.0 | |
| 安全存储 | EncryptedSharedPreferences（androidx.security:security-crypto） | 1.1.0-alpha06 | Token/凭据 |
| 日志 | XLog | 1.11.1（沿用 Demo，配 OkHttp/业务标签） | |

### 2.6 测试
JUnit5 + MockK + Turbine（Flow 测试）+ Robolectric（JVM 单元）+ Room in-memory + AndroidX Test/Espresso（仪器测试）+ MockWebServer（OkHttp 提供）。

---

## 3. 总体架构 / 模块划分

### 3.1 架构原则
- **单 Activity + Navigation**：`MainActivity` 承载 `NavHostFragment`，全部页面是 Fragment。相机交互页（预览/拍照）因 SDK `InstaCapturePlayerView`/`InstaImagePlayerView` 需要稳定 lifecycle，仍以 Fragment + ViewBinding 承载（Demo 用独立 Activity，本项目改为 Fragment 以统一导航栈，但持有 SDK 单例的逻辑下沉到 Repository，不依赖具体 UI 容器）。
- **MVVM + UDF**：ViewModel 持 `StateFlow<UiState>` + `Channel<UiEvent>`；Fragment 单向消费。**不可变**：UiState 用 `data class` + `copy()`，绝不原地修改。
- **Repository 收口副作用**：`InstaCameraManager.getInstance()` 是进程单例；所有对它的调用**只允许在 `CameraRepository` / `MediaRepository` 内部发生**，UI/ViewModel 不得直接 import `instaCameraManager`。这是为了把"网络分离""锁屏""监听器注册/反注册"等坑集中管理。
- **离线优先**：所有写操作先落 Room outbox（带 client UUID），再由 WorkManager 异步推送到后端/COS。UI 读取以本地为真相源。

### 3.2 模块（Gradle 模块，多模块）
高内聚低耦合，单文件 200–400 行（上限 800）。建议 Gradle 多模块以隔离 SDK：

```
:app                      // 单Activity宿主 + Navigation图 + DI入口(@HiltAndroidApp)
:core:common              // Result封装、错误模型、协程调度器、常量
:core:network             // OkHttp/Retrofit、openapi-generator 客户端、Envelope拦截器、JWT/refresh
:core:database            // Room: DAO + Entity + outbox + 字典缓存
:core:geo                 // ★坐标转换工具(WGS84↔GCJ-02) 唯一实现，单元测试覆盖
:core:designsystem        // 主题/通用View/Loading
:feature:auth             // 登录/用户/改密/退出
:feature:project          // 项目+巡查任务+巡查会话状态机
:feature:camera           // 相机连接/拍照/取全景/本地缓存  (依赖 :sdk-camera 封装)
:feature:trajectory       // GPS轨迹 + 前台服务
:feature:problem          // 问题拍照标注 + 自动定位 + 地图落点
:feature:map              // 腾讯地图展示 + 360查看
:feature:sync             // outbox + WorkManager 上传/重试 (依赖 :core:database/network)
:sdk-camera               // ★Insta360 SDK的唯一封装边界: CameraRepository/MediaRepository
```

> `:sdk-camera` 是唯一 `implementation(insta.camera)/(insta.media)` 的模块，其他模块只看到我们定义的领域接口，便于测试 Mock 与未来换机型。

### 3.2.1 Navigation 图 / 屏幕清单（单 Activity destinations）
> `MainActivity` 持 `NavHostFragment`，`nav_graph.xml` 列举全部 destination。下表为权威屏幕清单（落位 Phase 一一对应需求追溯表）。

| Destination（fragment id） | 屏幕 | 落位 Phase | 备注 |
|---|---|---|---|
| `loginFragment` | 登录 | P1 | 启动决策：有有效 refresh → 静默进 `projectListFragment` |
| `profileFragment` | 用户信息 / 退出 | P1 | display_name/avatar/角色 |
| `changePasswordFragment` | 改密 | P1 | `auth/password` |
| `projectListFragment`（start） | 项目列表 | P2 | 鉴权后起始页 |
| `projectDetailFragment` | 项目详情（custom_fields 动态渲染） | P2 | |
| `taskListFragment` | 巡查任务列表 | P2 | 默认 `assignee_id=当前用户` |
| `taskDetailFragment` | 任务详情 | P2 | |
| `taskCreateFragment` | 新建巡查任务 | P2 | 按权限码显隐 |
| `inspectionRunningFragment` | 进行中巡查（开始/结束、轨迹状态、待同步计数） | P2/P4 | 持活动巡查上下文 |
| `cameraConnectFragment` | 相机连接/状态 | P3a | 连接热点引导 + 连接状态 |
| `captureFragment` | 拍照（含本地全景回看 `InstaImagePlayerView`，非 live preview） | P3b/P3d | |
| `problemEditFragment` | 问题标注（自动定位 + 地图微调 + 动态表单） | P5 | |
| `mapFragment` | 地图综合展示（当前位置/轨迹/问题点） | P7 | |
| `problemDetailFragment` | 问题详情 / 看全景 / 看描述备注 | P7 | 点击 Marker 进入 |
| `panoramaViewerFragment` | 360 查看（`InstaImagePlayerView` 本地/远端 equirectangular） | P7 | |
| `syncStatusFragment` | 待同步项 / 重传状态 | P6 | outbox 可视化 |

### 3.2.2 AndroidManifest 权限表（合并清单）
> 集中声明，避免散落各模块；运行时权限申请见 §3.2.3。

| 权限 | 用途 | 类型 | 关联 Phase |
|---|---|---|---|
| `INTERNET` | 业务 API / COS / 地图 | 普通 | P0 |
| `ACCESS_NETWORK_STATE` | `NetworkStateTracker` 网络分离 | 普通 | P0 |
| `CHANGE_NETWORK_STATE` | `WifiNetworkSpecifier` 请求相机热点网络 | 普通 | P0/P3a |
| `ACCESS_WIFI_STATE` | 解析相机 WiFi（`cameraNet`，IP 比对） | 普通 | P0/P3a |
| `CHANGE_WIFI_STATE` | 连接相机热点 | 普通 | P3a |
| `NEARBY_WIFI_DEVICES`（API 33+，`neverForLocation`） | Android 13+ 扫描/连接相机热点免定位权限 | 运行时 | P3a |
| `ACCESS_FINE_LOCATION` | GPS 轨迹 + 问题自动定位 + 拍照 `GpsData` | 运行时 | P4/P5/P3b |
| `ACCESS_COARSE_LOCATION` | 与 fine 配对申请 | 运行时 | P4 |
| `ACCESS_BACKGROUND_LOCATION`（API 29+） | 息屏/后台持续采集轨迹 | 运行时（**两步**） | P4 |
| `FOREGROUND_SERVICE` | 前台服务基座 | 普通 | P0 |
| `FOREGROUND_SERVICE_LOCATION`（API 34+） | `TrajectoryForegroundService` | 普通 | P4 |
| `FOREGROUND_SERVICE_CONNECTED_DEVICE`（API 34+） | `CameraForegroundService` | 普通 | P3a |
| `POST_NOTIFICATIONS`（API 33+） | 前台服务通知可见 | 运行时 | P4/P3a |
| `WAKE_LOCK` | 上传/拼接期间保活 | 普通 | P3/P6 |
| `CAMERA`（仅当用本机相机扫码/取景时） | 一般不需（拍照走 X3 SDK，非系统相机） | 运行时（按需） | — |

> **不申请** `READ/WRITE_EXTERNAL_STORAGE`：全部媒体/`.insp` 落应用专属 scoped 目录（见 P0 FileProvider 收窄）。

### 3.2.3 运行时权限时序（关键合规路径）
1. **首启隐私政策同意门禁**：未同意前**不** `InstaCameraSDK.init` 之外触发任何定位/相机/网络扫描敏感行为（P7 合规）。
2. **定位两步（Android 11/API 30+）**：先申请 `ACCESS_FINE_LOCATION`（前台），用户授予后**再单独**申请 `ACCESS_BACKGROUND_LOCATION`（系统跳"始终允许"设置页，不可与 fine 同批申请）。两步均须有场景文案。
3. **通知权限（API 33+）**：启动前台服务前申请 `POST_NOTIFICATIONS`，拒绝时降级（服务仍运行，提示通知不可见影响保活观测）。
4. **相机热点连接**：API 33+ 走 `NEARBY_WIFI_DEVICES`（`neverForLocation`）+ `WifiNetworkSpecifier`；API ≤32 回退到需定位权限的扫描路径。`WifiNetworkSpecifier` 在 Android 10+ 系统弹窗确认连接，UI 须给出"请在系统弹窗点击连接"引导（见 §3.2.4）。
5. 任一运行时权限被拒 → 功能降级 + 引导去系统设置，不崩溃、不静默吞错。

### 3.2.4 WifiNetworkSpecifier 相机热点连接流程
1. 校验目标 SSID 前缀（`CameraWifiPrefix` / X3 机型前缀）。
2. 构建 `WifiNetworkSpecifier.Builder().setSsidPattern/ setSsid(...).setWpa2Passphrase(...)`，`NetworkRequest` 加 `TRANSPORT_WIFI` + `NET_CAPABILITY_*`。
3. `connectivityManager.requestNetwork(request, callback)` → 系统弹窗 → 用户确认 → `onAvailable(network)` 即 `cameraNet`。
4. **不** `bindProcessToNetwork(cameraNet)`；改 `setNetIdToCamera(cameraNet.networkHandle)` + 暴露 `cameraNet.socketFactory`（见 P0/P3）。
5. 退出连接：`unregisterNetworkCallback` 释放该 `NetworkRequest`，进程默认网络始终保持蜂窝。

### 3.3 关键目录结构（`:sdk-camera` 与 `:feature:camera` 为例）
```
sdk-camera/src/main/java/com/c5/sdkcamera/
├─ di/                 CameraModule.kt              (@Provides CameraRepository/MediaRepository)
├─ net/
│  ├─ CameraNetworkBinder.kt    // ★cameraNet解析 + setNetIdToCamera + 提供 socketFactory
│  └─ NetworkStateTracker.kt    // 复刻Demo NetworkManager: mobileNet/wifiNet/cameraNet
├─ connect/
│  ├─ CameraRepository.kt       // openCamera/closeCamera/状态/拍照/sensor模式
│  └─ CameraConnectionState.kt  // sealed: Disconnected/Connecting/Connected(type)/Error
├─ capture/
│  ├─ CaptureController.kt      // switchPanoramaSensorMode/startNormalCapture(gps)/startHDRCapture
│  └─ GpsDataMapper.kt          // Location -> GpsData bytes
├─ media/
│  ├─ MediaRepository.kt        // .insp下载(per-socket) + ExportUtils拼接PANORAMA
│  ├─ InspDownloader.kt         // OkGo + cameraNet.socketFactory（非全进程绑定）
│  └─ PanoramaStitcher.kt       // ExportImageParamsBuilder(PANORAMA,2:1) 封装
└─ service/
   └─ CameraForegroundService.kt // connectedDevice 前台服务 + 自动重连
```

---

## 4. 与其他端的契约引用

> 本节只**引用**冻结契约，不重述定义。所有名称逐字使用。

### 4.1 数据模型（Data Dictionary，逐字引用表名）
APP 在 Room 中建立"**镜像 + outbox**"两类表：
- **下行只读缓存**：`users`（当前用户）、`projects`、`inspection_tasks`、`dict_type`/`dict_item`（字典，带 `version`/`content_hash`）、`app_config`。
- **上行 outbox**（本地产生、待同步）：对应后端 `inspections`、`trajectory_points`、`problems`、`media_assets`，**每行均带 `client_uuid`**（客户端生成 UUID，幂等键）。
- 关键字段语义沿用后端定义：
  - `inspections`：`client_uuid`(UNIQUE)、`status`(`inspection_status` **= `IN_PROGRESS` | `FINISHED` | `ABANDONED`**，逐字钉死)、`started_at`/`ended_at`、`route_geom`（会话结束时由轨迹点生成，**后端计算** `mileage_meters` via `ST_Length(route_geom::geography)` → 米）。**APP 不在本地算里程**，只上传轨迹点与起止时间。
  - `trajectory_points`：`client_uuid`(UNIQUE)、`inspection_id`、`seq`(`UNIQUE(inspection_id,seq)`)、`geom`(Point,4326)、`recorded_at`、`speed/bearing/altitude/accuracy`。
  - `problems`：`client_uuid`(UNIQUE)、`geom`(Point,4326,自动定位)、`type_item_id`/`status_item_id`/`category_item_id`（软 FK→`dict_item`）、**`dict_version_used`（拍摄时锁定的 problem_type 字典版本）**、`description`/`note`、`captured_at`、`cover_media_id`。
  - `media_assets`：`client_uuid`(UNIQUE)、`owner_type`(`media_owner_type` = `problem` | `inspection` | `project` | `user`)/`owner_id`（多态软 FK，无 DB 外键）、`tier`(`media_tier` = `original` | `web` | `thumb`)、`cos_bucket`/`cos_key`、`etag`、**`capture_state`（`CAPTURED_RAW→STITCHED→QUEUED→UPLOADING→UPLOADED→CONFIRMED`，前向单向）**、`media_group`、`width`/`height`。**D4：APP 仅产生并上传 `tier=original`；`web`/`thumb` 由后端 asynq worker 在 original 到达 `CONFIRMED` 后派生（共享 `media_group` 的兄弟行）。`verified_at` 仅在 `CONFIRMED`（后端 HeadObject 通过）时写入。**

### 4.2 API（API Contract，逐字引用接口名）
统一封套 `{success,data,error,meta}`；一个端点一个具名 schema（生成强类型客户端）。APP 用到：
- 认证：`POST /api/v1/auth/login`、`POST /api/v1/auth/refresh`、`POST /api/v1/auth/logout`、`GET /api/v1/auth/me`、`PUT /api/v1/auth/password`。
- 字典/配置（ETag 拉取）：`GET /api/v1/dict/types`、`GET /api/v1/dict/types/{code}/items`（`If-None-Match` → 304）、`GET /api/v1/config/{key}`。
- 项目/任务：`GET /api/v1/projects`、`GET /api/v1/tasks?project_id=&status=&assignee_id=`。
- 巡查：`POST /api/v1/inspections/start`、`POST /api/v1/inspections/{id}/finish`、`GET /api/v1/inspections/{id}/trajectory`。
- 问题：`GET /api/v1/problems?project_id=&type=&status=&from=&to=&inspector_id=&inspection_id=`（**D1：允许 `inspection_id` 过滤** —— APP 按当前活动巡查上传/回看问题，须在 outbox/sync payload 携带 `inspection_id` 使 Web "按巡查记录列问题" 成立）、`POST /api/v1/problems`、`POST /api/v1/problems/{id}/logs`（**D3：仅可 POST `action=COMMENT|REASSIGN`；`STATUS_CHANGE` 由后端在改状态的同一事务内自动追加，客户端永不直接 POST `STATUS_CHANGE`**）、`GET /api/v1/problems/map`（GeoJSON，WGS84）。
- 媒体：`POST /api/v1/media/upload-credentials`（STS）、`POST /api/v1/media/confirm`（HeadObject 校验）、`GET /api/v1/media/{id}`（签名 CDN URL）。
- 同步：`POST /api/v1/sync/batch`（幂等批量 upsert，per-item accept/reject）。

### 4.3 坐标系（强约束）
- **存储与传输一律 WGS84 / SRID 4326**；GCJ-02 **永不持久化、永不上传**。
- 仅在**腾讯地图渲染边界**做转换：WGS84→GCJ-02（绘点/绘线）、地图点击 GCJ-02→WGS84（再落库/上传）。
- **唯一转换实现**位于 `:core:geo`（`CoordinateConverter`，单元测试覆盖往返误差），全 APP 不得有第二处转换代码。
- FusedLocation 返回的是 WGS84，**直接**作为 `trajectory_points.geom` / `problems.geom` 与相机 `GpsData` 的输入，不转换。

### 4.4 同步契约（Offline-sync）
- `POST /api/v1/sync/batch`：单一幂等封套，携带本批 inspections / trajectory_points / problems / media 引用，每项带 `client_uuid`；服务端 `INSERT ... ON CONFLICT (client_uuid)` + per-item SAVEPOINT；返回 `data.results:[{client_uuid,status:accepted|rejected|duplicate,server_id?,error?}]`。
- **大媒体绝不走 JSON**：走 STS 临时密钥 + COS 分块上传（可续传），后端 `HeadObject` 校验后 `confirm`。`media/confirm` 失败 → 409 `MEDIA_VERIFY_FAILED`，状态保持 `UPLOADING`。
- **历史字典容错**：上传带已退役 `type_item` + 旧 `dict_version_used` 的问题**不被拒绝**（`DICT_VERSION_RETIRED` 为保留码，**不用于**历史容错同步）。APP 拉取字典时缓存 `dict_version_used`，离线拍照时锁定当时版本。

### 4.5 错误码契约（逐字引用，全端统一）
> APP 的 `ApiException`/`AppError` 映射必须与契约 00 错误码表**逐字一致**（单一权威字面量）。`error.details[]` 形如 `[{field,code,message}]`。

| code | HTTP | APP 侧处理 |
|---|---|---|
| `VALIDATION_FAILED` | 422 | 边界校验失败（如 `ended_at<started_at`、未知自定义字段 key、几何无效）；表单回显 `details[]`。 |
| `UNAUTHENTICATED` | 401 | 缺失/无效 access token（**注意：不是 `UNAUTHORIZED`**）；拦截器走登录或刷新分支。 |
| `TOKEN_EXPIRED` | 401 | access JWT 过期；**刷新拦截器据此触发 `auth/refresh` 单飞（single-flight）**，与 `UNAUTHENTICATED` 区分处理。 |
| `FORBIDDEN` | 403 | casbin 拒绝；即使 UI 已隐藏控件也须兜底（前端权限仅 UI 显隐）。 |
| `NOT_FOUND` | 404 | 资源不存在或已软删除（`deleted_at`）。 |
| `CONFLICT` | 409 | 唯一冲突（username/code/cos_key）、删除 RESTRICT、非法状态机跃迁、`trajectory(inspection_id,seq)` 同步冲突。 |
| `MEDIA_VERIFY_FAILED` | 409 | `media/confirm` 的 HeadObject 不匹配；`capture_state` 保持 `UPLOADING`，客户端重传。 |
| `DICT_VERSION_RETIRED` | 409 | 保留码；正常带退役项的历史同步**不触发**此码（容错接受）。 |
| `RATE_LIMITED` | 429 | 热点端点（登录、`sync/batch`）限流；客户端**指数退避**后重试。 |
| `INTERNAL` | 500 | 后端未处理异常；不泄露栈/SQL，客户端通用提示 + 可重试。 |

### 4.6 Day-1 必须产出的阻塞型契约产物（引用，非本端编写）
- `api/openapi.yaml`（**已落盘**）：APP Kotlin/Retrofit 客户端的唯一来源；`:core:network` 客户端代码由 **openapi-generator**（`-g kotlin --library jvm-retrofit2`）从它生成。
- `db/migrations/000002_seed.up.sql`（**已落盘并经容器验证**）：枚举/casbin/角色权限/核心 `dict_type`/`dict_item`（含 `problem_status` 基线 `OPEN|PROCESSING|RESOLVED|CLOSED` 作为 dict_item，非 DB 枚举）/`app_config`（`export.image`、`capture.default`）的唯一真相源。
- `db/migrations/000001_init.up.sql` + `000001_init.down.sql`（已落盘并验证：16 表 / 9 枚举 / `ST_SRID(4326)` CHECK）。
> 上述 `.yaml`/`.sql` 不在本文档职责内，亦不得在本任务中创建/修改；此处仅作为联调前置 gate 引用。

---

## 5. 分阶段开发计划（文档核心）

> 阶段严格递进：P0 打地基（脚手架 + 网络分离 + DI/Room/Retrofit），P1–P2 业务骨架，P3 相机硬核，P4–P5 现场采集，P6 离线同步闭环，P7 展示与合规上架。
> 每个 Phase 任务可勾选；"坑"均引用本项目已验证约束。

---

### Phase 0 — 净室脚手架 + 生产加固 + 网络分离（P0）

#### 阶段目标
从 Demo 工具链净室搭出可编译的多模块工程；接通 Hilt/Room/Retrofit；**实现并验证"WiFi+蜂窝并存"网络分离**（这是整个 APP 的生死线，必须在做任何业务前打通并单测）。

#### 前置依赖
无（首个阶段）。需备齐：Insta360 私有 Maven 凭据、一台 X3、一部支持双网络的真机（Android 10+）。

#### 详细任务清单
- [ ] 用 AGP 8.9.0 / Gradle 8.11.1 / Kotlin 2.0.21 / JVM 17 新建多模块工程（第 3.2 节结构），`minSdk=29`、`targetSdk=35`、`abiFilters=["arm64-v8a"]`、`multiDexEnabled=true`。
- [ ] 复刻 Demo 的 `packaging{}`：`jniLibs.excludes`（SNPE skel `.so`、`lib/x86/*`）、`resources.pickFirsts += "lib/arm64-v8a/libc++_shared.so"`、`resources.excludes += "META-INF/rxjava.properties"`。
- [ ] 接入 `sdkcamera`/`sdkmedia` 1.10.1（私有 Maven），仅在 `:sdk-camera` 模块。
- [ ] `@HiltAndroidApp` Application：迁移 Demo 的 `InstaApp.onCreate` 初始化序列 —— `System.loadLibrary("cpu_monitor")`（如保留性能监控则需 CMake，否则去除该行与 cpp 目录）、`InstaCameraSDK.init(this)`、`InstaMediaSDK.init(this)`、`LogManager` 配置、启动 `NetworkStateTracker`。
- [ ] **R8/混淆加固**：`isMinifyEnabled=true`（release），`proguard-rules.pro` 写入 `-keep class com.arashivision.** { *; }`、`-keep class com.qcloud.cos.** { *; }`、`-keepattributes *Annotation*,Signature,InnerClasses`、Retrofit/Moshi/Hilt keep 规则。（Demo 当前 `isMinifyEnabled=false` 且无 keep，**不可照抄**。）
- [ ] **网络安全配置加固**：`network_security_config.xml` 仅对 `192.168.42.1`（相机）放行 cleartext，`base-config` `cleartextTrafficPermitted="false"`（Demo 当前 base 为 true + manifest `usesCleartextTraffic="true"`，**收紧**：移除 manifest 的 `usesCleartextTraffic`，仅靠 domain-config 放行相机 IP）。
- [ ] **FileProvider 收窄**：`file_paths.xml` 不用 Demo 的 `<external-path path="."/>`（暴露整卡），改为 scoped 路径（`files-path`/`cache-path`/具体子目录如 `panorama/`、`insp/`）。
- [ ] EncryptedSharedPreferences 封装 `SecureStore`（替换 Demo 的明文 `SPUtils`）用于 token/相机凭据。
- [ ] `:core:network`：OkHttp + Retrofit + openapi-generator 生成客户端；Envelope 拦截器（解包 `{success,data,error,meta}`，错误 `code` 映射到统一 `AppError`，字面量逐字对齐 §4.5 全表 —— 含 `UNAUTHENTICATED`/`TOKEN_EXPIRED`/`FORBIDDEN`/`RATE_LIMITED` 等）；JWT 注入 + 401 拦截器：**按 `code` 分支** —— `TOKEN_EXPIRED` → 触发 `auth/refresh` **单飞（single-flight）** 旋转并重放原请求；`UNAUTHENTICATED` → 直接登出回登录页。`RATE_LIMITED(429)` → 指数退避重试。opaque refresh 存 EncryptedSharedPreferences。**此 OkHttp 实例与相机栈完全隔离，使用进程默认网络（蜂窝）**。
- [ ] `:core:database`：Room DB（in-memory 测试就绪），建空 DAO 占位。
- [ ] **★`CameraNetworkBinder`（核心）**：
  - 复刻 `NetworkStateTracker`（= Demo `NetworkManager`）：`registerNetworkCallback` 同时监听 `TRANSPORT_CELLULAR + TRANSPORT_WIFI`，维护 `mobileNet`/`wifiNet`，并按 IP 比对解析出 `cameraNet`（相机热点那条 WiFi Network）。
  - **进程默认网络保持蜂窝**：连接相机成功后**不调用** `bindProcessToNetwork(cameraNet)`（Demo 的 `ConnectViewModel.onCameraStatusChanged` 里 `bindProcessToNetwork(NetworkManager.mobileNet)` 思路保留，但我们更进一步：**永不**把进程绑到相机网，只在需要时把进程绑回/保持在 mobileNet 或不绑）。
  - 连接时调用一次 `instaCameraManager.setNetIdToCamera(cameraNet.networkHandle)`（告诉 SDK 内部 socket 走相机 WiFi）。
  - 暴露 `cameraSocketFactory(): SocketFactory? = cameraNet?.socketFactory`，供 `.insp` 下载的 OkGo/OkHttp **按 socket 绑定**（见 P3）。
- [ ] **CI**：`./gradlew assembleDebug` + `lint` + 单测在 CI 跑通。

#### 涉及模块/接口/表
`:app`、`:core:network`、`:core:database`、`:sdk-camera`；接口：`auth/login`、`auth/refresh`、`auth/me`（先打通鉴权管道，业务在 P1）。

#### 关键实现要点与坑
- **【生死线】网络分离**：约束明确 —— 进程默认网络停留在**蜂窝**（上传/API/地图需公网），SDK socket 走相机 WiFi（`setNetIdToCamera(networkHandle)`）；相机 HTTP 下载**只绑定其 OkHttp 自身的 socket**（`cameraNet.socketFactory`），**不得**全进程 `bindProcessToNetwork(cameraNet)`。Demo 的 `CaptureViewModel`/`AlbumViewModel` 大量使用 `bindProcessToNetwork(cameraNet)`（甚至下载时全进程绑定再解绑），**本项目明令拒绝**——这会在下载/拼接期间切断公网，导致并发上传/地图请求失败。
- **setNetIdToCamera 的副作用**：它影响 SDK 内部命令通道走哪条网络；调用时机应在 `openCamera` 成功之后、首次 `fetchCameraOptions` 之前。
- Demo 的 `connectivityManager.allNetworks` 已 deprecated，可保留但需 `@Suppress`；`cameraNet` 解析依赖 WiFi IP 比对（`192.168.42.1` 网段），保留 Demo 算法。
- `WifiNetworkSpecifier` 连接相机热点（系统级）在 Android 10+ 弹窗确认，需在 UI 给出引导。

#### DoD（验收标准）
1. release 包混淆开启仍能正常 `InstaCameraSDK.init` 且不崩（验证 `-keep com.arashivision.**` 生效）。
2. **真机验证**：相机 WiFi 连接成功后，`adb` 抓包/日志证明：业务 Retrofit 请求走蜂窝（公网可达），相机命令/下载走相机 WiFi；**整个过程公网 API 不中断**。
3. 401 自动 refresh 链路打通（用 mock server 验证 access 过期→refresh→重放）。
4. `CoordinateConverter` 占位存在，CI 绿。

#### 测试要求
- 单元：`NetworkStateTracker` 的网络分类逻辑（Mock `NetworkCapabilities`）；Envelope 拦截器解包成功/失败分支；refresh 拦截器（MockWebServer 返回 401→200）。
- 仪器：真机网络分离 smoke test（连相机 + 并发 ping 公网）。
- 覆盖率门槛：`:core:network` ≥ 80%。

---

### Phase 1 — 登录 + 用户 + 字典离线缓存（P1）

#### 阶段目标
完成账号密码登录、用户信息展示、改密、退出；建立**字典 ETag 拉取 + 离线缓存 + 历史版本锁定**机制（后续问题标注依赖）。

#### 前置依赖
P0（网络管道、SecureStore、Room）。

#### 详细任务清单
- [ ] `:feature:auth`：登录页（用户名/密码，输入校验：非空、长度）。调 `POST /api/v1/auth/login` → 存 `access_token`(内存+短期)、`refresh_token`(EncryptedSharedPreferences)、`user`。
- [ ] 登录后调 `GET /api/v1/auth/me` 获取 user + roles + **effective permission codes**，存内存 `SessionState`；权限码用于 UI 显隐（如"新建项目"按钮）。
- [ ] 用户信息页（display_name、avatar via Glide、角色）。
- [ ] 改密：`PUT /api/v1/auth/password`（旧密码+新密码，前端校验强度）。
- [ ] 退出：`POST /api/v1/auth/logout`（吊销 refresh）→ 清 SecureStore + 内存 → 跳登录。
- [ ] **字典缓存（核心）**：`DictRepository` + Room 表 `dict_type`/`dict_item`。
  - `GET /api/v1/dict/types` 拉类型列表（含 `version`/`content_hash`）。
  - `GET /api/v1/dict/types/{code}/items` 带 `If-None-Match:"<content_hash>"`；304 → 用本地，200 → 覆盖并存新 `content_hash`/`version`。
  - 同样模式拉 `GET /api/v1/config/{key}`（如 `capture.default`、`export.image`）。
  - 提供 `dictVersionOf(scope=problem_type)` 给问题标注锁定 `dict_version_used`。
- [ ] App 启动时若有有效 refresh → 静默续期进首页；否则进登录。

#### 涉及模块/接口/表
接口：`auth/login`、`auth/me`、`auth/password`、`auth/logout`、`dict/types`、`dict/types/{code}/items`、`config/{key}`。表：`users`、`dict_type`、`dict_item`、`app_config`。

#### 关键实现要点与坑
- **ETag = content_hash**：必须用 `If-None-Match`，正确处理 304（不要把 304 当错误）。响应体含 `version`+`content_hash`，离线客户端据此 pin 版本。
- **历史字典容错的源头在此**：缓存 `dict_item` 时保留 `is_active=false` 的退役项（不要过滤掉），否则离线问题引用退役类型时本地渲染会丢失标签。
- token 绝不入明文 SP/日志（安全约束）。
- 时间戳：服务端均 timestamptz UTC，APP 渲染按 Asia/Shanghai。

#### DoD
1. 登录→首页→退出闭环；杀进程重开能凭 refresh 静默进入。
2. 字典首拉 200、二拉 304（抓包验证 `If-None-Match`）。
3. 断网时字典/用户信息仍可从 Room 读取展示。
4. 无权限码的按钮不渲染（基于 `auth/me` 权限码）。

#### 测试要求
- 单元：`AuthRepository`（登录/刷新/登出状态机）、`DictRepository`（304 命中、200 覆盖、退役项保留）。
- 仪器：登录 UI（Espresso，含校验报错）。
- 覆盖率：`:feature:auth`、字典缓存 ≥ 80%。

---

### Phase 2 — 项目 / 巡查任务 + 巡查会话状态机（P2）

#### 阶段目标
项目列表/详情、任务列表/详情/新建、**开始/结束巡查**会话；建立巡查会话状态机（与后端 `inspection_status` 对齐），为 P4 轨迹、P5 问题提供"当前巡查上下文"。

#### 前置依赖
P1（鉴权 + 字典：`project_field` 自定义字段字典）。

#### 详细任务清单
- [ ] `:feature:project`：项目列表 `GET /api/v1/projects`（分页 `meta`），详情含 `custom_fields`（由 `dict_scope='project_field'` 驱动动态渲染）。
- [ ] 任务列表 `GET /api/v1/tasks?project_id=&status=&assignee_id=`（默认筛选 `assignee_id=当前用户`），详情、新建 `POST /api/v1/tasks`（按权限码显隐新建）。
- [ ] **巡查会话**：
  - "开始巡查"→生成 `client_uuid`（本地 UUID）→写 `inspections` outbox（`status=IN_PROGRESS`，逐字对齐 `inspection_status` 字面量），`started_at`=now（UTC）；同时调 `POST /api/v1/inspections/start`（在线时取回 server_id；离线时仅本地）。
  - 维护**单一活动巡查上下文**（`ActiveInspectionStore`，进程内 + Room 持久化，杀进程可恢复）。
  - "结束巡查"→`ended_at`=now，`status=FINISHED` → 触发 P4 轨迹收尾 + `POST /api/v1/inspections/{id}/finish`（**里程/时长由后端算**）。离线则进 outbox 待 P6 同步。
  - "丢弃巡查"（用户主动放弃/异常作废）→ `status=ABANDONED`，不上传轨迹收尾。
- [ ] **会话状态机（本地 FSM）→ 服务端 `inspection_status` 映射（逐字钉死）**：
  - 本地 FSM：`IDLE → STARTING → ACTIVE → ENDING → ENDED`。
  - 映射：`ACTIVE → IN_PROGRESS`（`/inspections/start` 之后）；`ENDED → FINISHED`（`/finish` 之后）；用户丢弃 → `ABANDONED`。`STARTING`/`ENDING` 为本地过渡态，不持久化到服务端 `status`。
  - 服务端 `inspection_status` 仅三值：`IN_PROGRESS | FINISHED | ABANDONED`。
  - 非法跃迁拒绝（如 `ACTIVE` 中再次 `STARTING`、未 `ENDED` 即开第二个会话）；并发会话被单一活动巡查上下文挡住。

#### 涉及模块/接口/表
接口：`projects`、`tasks`、`inspections/start`、`inspections/{id}/finish`。表：`projects`、`inspection_tasks`、`inspections`（outbox）、`dict_item`(project_field)。

#### 关键实现要点与坑
- **client_uuid 在开始巡查时即生成**（不是上传时）——它是 `inspections` 的幂等键（UNIQUE），离线创建必须有。
- `ended_at >= started_at`（后端 CHECK），UI 防止时钟回拨导致非法。
- **APP 不计算 mileage/duration**：`mileage_meters` 用 `ST_Length(route_geom::geography)` 在后端算（geometry(4326) 直接算是度，不是米）；APP 只传轨迹点与起止时间。
- 单一活动巡查：防止并发会话污染轨迹/问题归属。
- `route_geom` NULL until 结束——APP 在 `finish` 时不传 geom，由后端从 trajectory_points 构建。

#### DoD
1. 项目/任务 CRUD-读 与新建可用，分页正确。
2. 开始→结束巡查闭环；离线开始的会话进 Room outbox 且 `client_uuid` 持久。
3. 杀进程后活动巡查可恢复（不丢起始时间）。
4. 状态机非法跃迁被拒并提示。

#### 测试要求
- 单元：会话状态机（全跃迁矩阵）、`InspectionRepository`（在线/离线分支）。
- 仪器：开始/结束巡查 UI 流。
- 覆盖率：`:feature:project` ≥ 80%。

---

### Phase 3 — 相机连接 + 拍照 + 下载 .insp + 端上拼接 PANORAMA + GPano（P3）

> **P3 是最硬核阶段，拆为 4 个子里程碑串行验收**，每个子里程碑独立 DoD：
> - **P3a 连接**：WiFi+蜂窝并存连接 X3 + 前台服务 + 状态 UI。
> - **P3b 拍照**：切全景 sensor + 带 GPS 拍照（普通/HDR）+ X3 机型分支。
> - **P3c 下载 `.insp`**：socket 级绑定下载（公网不中断）。
> - **P3d 拼接 + GPano**：`ExportUtils` 拼 equirectangular 2:1 JPG + 写 GPano XMP（D5）+ 本地回看。
>
> **【五-2 决议·NO LIVE PREVIEW】v1 不做 360 实时预览/取景流。** Demo 的 record/live/直播分支一律剔除；`captureFragment` 仅"按下拍照→拿到结果→可选本地回看"。拍照后用 `InstaImagePlayerView` 看**已落盘**的全景**不是** live preview。

#### 阶段目标
打通 X3 全链路：连接（WiFi+蜂窝并存）→ 切全景 sensor → 带 GPS 拍照 → 下载 `.insp` → 端上拼接 equirectangular 2:1 JPG → 写 GPano。**APP 仅产生 `tier=original`**（D4），`web`/`thumb` 由后端派生。

#### 前置依赖
P0（网络分离、`CameraNetworkBinder`）、P1（`capture.default` 配置、`export.image` 尺寸/质量）、P4 可并行（GPS 提供 Location）。

#### 详细任务清单
**【P3a】连接（CameraRepository）**
- [ ] `CameraForegroundService`（`foregroundServiceType="connectedDevice"`，权限 `FOREGROUND_SERVICE_CONNECTED_DEVICE`）：相机连接期间常驻通知（复刻 Demo `ConnectService`），承载自动重连。
- [ ] 连接流程（净室自 Demo `ConnectViewModel`）：
  1. 引导用户连相机热点 SSID（`CameraWifiPrefix`/`CameraType` 校验，X3 前缀）。必要时 `WifiNetworkSpecifier` 系统级连接。
  2. 解析 `cameraNet`；**`setNetIdToCamera(cameraNet.networkHandle)`**；**进程默认网络保持蜂窝**（不 `bindProcessToNetwork(cameraNet)`）。
  3. `instaCameraManager.openCamera(InstaCameraManager.CONNECT_TYPE_WIFI)`。
  4. 注册 `ICameraChangedCallback`：`onCameraStatusChanged(enabled, CONNECT_TYPE_WIFI)` → 更新 `CameraConnectionState`，启动前台服务；断连 → 状态 + 自动重连（指数退避）。
- [ ] 连接状态 UI（已连/断开/电量/SD 剩余 `getRemaining`/`cameraStorageFreeSpace`）。

**【P3b】拍照（CaptureController）**
- [ ] 进拍摄页：`setCameraLockScreen(true)`（防用户在机身改参数致不同步）。
- [ ] `fetchCameraOptions()`（绑定相机网络通信，用 socket 级而非全进程；命令通道已由 `setNetIdToCamera` 路由）。
- [ ] **检查并切全景**：若 `!isCameraDualSensorMode` / `currentSensorMode != PANORAMA` → `switchPanoramaSensorMode(ICameraOperateCallback)`。
- [ ] **X3 跳过 `initCameraSupportConfig`**（约束：该步骤仅 X4+ 需要；X3 调用会失败/无意义）。以机型判断分支：`if (cameraType is X4+) initCameraSupportConfig() else skip`。
- [ ] 应用拍摄预设（来自 `capture.default` / `capture_preset` 字典：分辨率、HDR 开关）。
- [ ] 拍照（单击动作）：
  - 普通：`GpsDataMapper` 将当前 FusedLocation→`GpsData`（`latitude/longitude/groundSpeed/groundCrouse/geoidUndulation/utcTimeMs/isVaild=true`）→ `GpsData.GpsData2ByteArray(listOf(gps))` → **`instaCameraManager.startNormalCapture(gpsBytes)`**；无定位则 `startNormalCapture()`。
  - HDR：`CaptureMode.HDR_CAPTURE` → `instaCameraManager.startHDRCapture()`。
  - 监听 `ICaptureStatusListener.onCaptureFinish(paths)` 拿到机内/缓存路径。
- [ ] **本项目仅拍照（PHOTOS ONLY，五-1/五-2 决议）**：禁用所有录像/直播/实时预览分支（Demo 有大量 record/live/preview，剔除）。不渲染 `InstaCapturePlayerView` 取景流。

**【P3c】下载 .insp（MediaRepository / InspDownloader）**
- [ ] `WorkUtils.getAllCameraWorks()` 列举机内作品（`WorkWrapper`），`workWrapper.allUrls` 取 `.insp`（X3 全景为双鱼眼 raw）URL。
- [ ] **按 socket 绑定下载（关键，替换 Demo 全进程绑定）**：用 SDK 内部 OkGo 或我们自建 OkHttp，**设置 `socketFactory = CameraNetworkBinder.cameraSocketFactory()`**，使下载只走相机 WiFi，公网不受影响。`FileCallback` 回调进度，落 scoped `insp/` 目录。
- [ ] 写本地 `media_assets` outbox：`capture_state=CAPTURED_RAW`，`media_group=新UUID`，`owner_type/owner_id` 指向问题或巡查。

**【P3d】端上拼接 + GPano（PanoramaStitcher / XmpWriter）**
- [ ] `WorkWrapper(arrayOf(inspPath))` + `ExportImageParamsBuilder`：
  - `exportMode = ExportUtils.ExportMode.PANORAMA`、`setScreenRatio(2, 1)`；
  - `width/height` 来自 `export.image` 配置（如 4096×2048，admin 可配尺寸/质量）；
  - `targetPath = panorama/<ts>.jpg`（scoped）。
  - `ExportUtils.exportImage(workWrapper, builder, IExportCallback)`（`onProgress/onSuccess/onFail`）。
- [ ] **写 GPano XMP（D5，精确模板）**：拼接产物为 equirectangular 2:1。**优先**采用 Insta360 `ExportUtils` 已内嵌的 pano 元数据（若存在）；**否则**由 `:sdk-camera` 的 `XmpWriter` 助手手写一段 GPano XMP APP1 包，字段如下（W=图宽、H=图高）：

  | 字段 | 值 |
  |---|---|
  | `ProjectionType` | `equirectangular` |
  | `UsePanoramaViewer` | `True` |
  | `FullPanoWidthPixels` | `W` |
  | `FullPanoHeightPixels` | `H` |
  | `CroppedAreaImageWidthPixels` | `W` |
  | `CroppedAreaImageHeightPixels` | `H` |
  | `CroppedAreaLeftPixels` | `0` |
  | `CroppedAreaTopPixels` | `0` |

  - **DoD**：`exiftool` 输出含 `XMP-GPano:*` 标签（至少 `ProjectionType=equirectangular`、`UsePanoramaViewer=True`），**且** Web 端 Photo Sphere Viewer 能将其渲染为球面。
  - 说明：PSV 的 equirectangular adapter 并不**强制**要求 GPano（Web 端结论），但 APP **仍必须**内嵌 GPano 以保证健壮性与跨端互操作。
- [ ] **D4 媒体分层归属（逐字钉死）**：APP **仅产生并上传 `tier=original`**（端上 `ExportUtils` 的 PANORAMA equirectangular，尺寸/质量来自 `app_config.export.image`，含 GPano XMP）。**APP 绝不产生/上传 `web`/`thumb`**；待 `original` 到达 `CONFIRMED` 后，由后端 asynq worker（`derive-media-tiers` 任务）派生 `web`+`thumb` 兄弟行（共享 `media_group`）。
- [ ] 拼接成功 → `media_assets` outbox 更新 `capture_state=STITCHED`，记录 `width/height`。
- [ ] 本地 360 回看（**非 live preview**）：`InstaImagePlayerView.setLifecycle(lifecycle)` + `prepare(workWrapper, ImageParamsBuilder)` + `play()` 查看刚拍的**已落盘**全景（APP 端用 SDK 播放器，Web 端用 PSV，二者消费同一份 equirectangular + GPano）。

#### 涉及模块/接口/表
模块：`:sdk-camera`、`:feature:camera`。表：`media_assets`(outbox)。SDK：`InstaCameraManager`、`WorkUtils`、`WorkWrapper`、`ExportUtils`、`ExportImageParamsBuilder`、`InstaImagePlayerView`、`GpsData`。

#### 关键实现要点与坑
- **X3 SKIPS initCameraSupportConfig**（约束逐字）：只有 X4+ 走该 JSON 配置流程；X3 直接 `switchPanoramaSensorMode → startNormalCapture/startHDRCapture`。务必机型分支。
- **下载严禁全进程绑定**：Demo `AlbumViewModel.downloadFile` 用 `bindProcessToNetwork(cameraNet)` 再 `bindProcessToNetwork(null)`——会在下载期间使公网 API/上传/地图全断。本项目用 `cameraNet.socketFactory` 仅绑该下载 socket。
- **GpsData 字段**：`groundCrouse`（注意 SDK 拼写）、`utcTimeMs`、`isVaild`（SDK 拼写为 Vaild）——照 SDK 实际签名，别"纠错"。
- 拍照后 SDK 可能内部切换 H264/H265 致预览异常（Demo `onCaptureFinishEnd` 有处理）；本项目只拍照不需预览流编码切换，但 `fetchCameraOptions` 后刷新 SD 剩余。
- `.insp` 是双鱼眼 raw，**Web 无 SDK 无法渲染**——必须在 APP 端拼成 equirectangular JPG 再上传；这是 P3 与后续上传契约的衔接点。
- 拼接耗时大，放 `Dispatchers.IO` / 后台，给进度；失败可重试。

#### DoD
1. 真机：连接 X3（WiFi+蜂窝并存，公网不断）→ 拍一张全景带 GPS。
2. `.insp` 下载期间，并发的公网请求（如轨迹上传/地图）**不中断**（抓包验证 socket 级绑定）。
3. 端上拼接出 2:1 equirectangular JPG（尺寸/质量来自 `export.image`），含 GPano XMP（用 exiftool 验证 `XMP-GPano:ProjectionType=equirectangular`）。
4. `InstaImagePlayerView` 本地可看刚拍全景；产物可被 Web PSV 正常打开（交叉验证）。
5. `media_assets` outbox 状态推进 `CAPTURED_RAW→STITCHED`，且**仅有 `tier=original` 一行**（无任何 APP 端 web/thumb）。
6. 子里程碑独立验收：P3a 连接稳定+前台服务；P3b 切全景+带 GPS 拍照成功（X3 跳过 `initCameraSupportConfig`）；P3c socket 级下载公网不断；P3d 拼接+GPano（exiftool 通过）。**全程无任何 live preview/取景流代码（五-2）**。

#### 测试要求
- 单元：`GpsDataMapper`（Location→GpsData 字段映射 + 边界：无定位/0 速度）、`PanoramaStitcher` 参数构造（PANORAMA/2:1/尺寸）、机型分支（X3 跳过 config）、**`XmpWriter`（D5 八字段 GPano XMP 生成，exiftool 断言 `XMP-GPano:ProjectionType=equirectangular`/`UsePanoramaViewer=True`）**、**D4 断言（产物仅 `tier=original`）**。
- 仪器（需真机+相机，纳入手动/夜间真机集）：连接→拍照→下载→拼接 E2E。
- 覆盖率：`:sdk-camera` 纯逻辑部分 ≥ 80%（SDK 调用以接口 Mock）。

---

### Phase 4 — FusedLocation GPS 轨迹 + 前台服务（P4）

#### 阶段目标
巡查期间持续采集 GPS 轨迹（前台服务保活），记录起止时间/路线/当前位置，结束后随同步上传。

#### 前置依赖
P2（活动巡查上下文）、P0（前台服务范式）。

#### 详细任务清单
- [ ] **用 `FusedLocationProviderClient`**（不用 Demo 的平台 `LocationManager`——精度/功耗更差）：`requestLocationUpdates`（高精度，间隔/位移按巡查场景配，如 5s/5m）。
- [ ] `TrajectoryForegroundService`（`foregroundServiceType` 含 `location`，权限 `ACCESS_FINE_LOCATION` + `FOREGROUND_SERVICE_LOCATION`）：巡查 ACTIVE 期间运行，断点续采（杀进程恢复）。
- [ ] 每个 fix → 写 `trajectory_points` outbox：`client_uuid`(UUID)、`inspection_id`、`seq`（自增、`UNIQUE(inspection_id,seq)`）、`geom`(Point,WGS84 直接来自 Fused)、`recorded_at`(fix time)、`speed/bearing/altitude/accuracy`。
- [ ] 异常点过滤：`accuracy` 超阈值丢弃、明显跳变/静止抖动抑制（不污染里程，里程后端算）。
- [ ] 当前位置实时回传 UI（地图 P7 用），渲染时才转 GCJ-02。
- [ ] 结束巡查（P2）→ 停止采集 → 轨迹点已在 outbox，待 P6 同步；后端 `finish` 从点构 `route_geom` 并算里程/时长。

#### 涉及模块/接口/表
模块：`:feature:trajectory`。表：`trajectory_points`(outbox)、`inspections`。接口：`inspections/{id}/finish`、`inspections/{id}/trajectory`（回看）。

#### 关键实现要点与坑
- **坐标永远 WGS84**：Fused 给的就是 WGS84，**直接落库**，绝不在采集时转 GCJ-02。
- `seq` 单调递增且与 `inspection_id` 唯一——保证后端 `(inspection_id,seq)` 去重重排序。
- `trajectory_points` 无软删除（随父巡查清除）；高频写入需批量事务，避免 Room 抖动。
- 前台服务通知必须有（Android 后台定位合规），且 minSdk 29+ 注意后台位置权限引导。
- 同一 fix 同时供给 P3 拍照的 `GpsData`（自动定位问题点）。

#### DoD
1. 巡查中息屏/切后台，轨迹仍连续采集（前台服务保活）。
2. 轨迹点正确入 outbox，`seq` 连续、坐标 WGS84。
3. 结束后 `finish` 成功，后端返回的里程/时长合理（与实际步行对比）。
4. 异常点被过滤，轨迹平滑。

#### 测试要求
- 单元：异常点过滤算法、`seq` 生成、Location→Entity 映射。
- 仪器：前台服务保活、息屏采集（真机）。
- 覆盖率：`:feature:trajectory` ≥ 80%。

---

### Phase 5 — 问题拍照标注 + 自动定位 + 地图落点（P5）

#### 阶段目标
现场发现问题 → 360 拍照（复用 P3）→ 自动定位 → 地图微调落点 → 选类型/分类/填描述备注 → 关联媒体，全部进 outbox。

#### 前置依赖
P3（拍照+拼接）、P4（定位）、P1（problem_type/status/category 字典 + 版本）、P2（活动巡查）。

#### 详细任务清单
- [ ] "发现问题"入口 → 触发 P3 360 拍照流程，产物 `media_assets`（`owner_type=problem`）。
- [ ] **自动定位**：取当前 FusedLocation（WGS84）作为 `problems.geom`(Point,4326)；记 `captured_at`。
- [ ] **地图微调落点**：腾讯地图上显示自动点位（渲染时 WGS84→GCJ-02），允许拖动；保存时点击坐标 GCJ-02→WGS84 回写 `geom`（用 `:core:geo` 唯一转换器）。
- [ ] **动态表单**：类型/状态/分类下拉来自 P1 缓存的 `dict_item`（按 scope），含退役项（历史容错）；`extra` jsonb 驱动附加字段。
- [ ] **锁定 `dict_version_used`**：选 problem_type 时记录当前 `dict_type(problem_type).version`（约束：离线后即使该类型被退役，上传也不被拒）。
- [ ] 描述 `description` / 备注 `note`（输入校验：长度上限）。
- [ ] 写 `problems` outbox：`client_uuid`、`project_id`、`inspection_id`、`inspector_id`、`geom`、`type_item_id/status_item_id/category_item_id`、`dict_version_used`、`cover_media_id`(拼接产物)、`captured_at`。
- [ ] 在线时可直接 `POST /api/v1/problems`；离线进 outbox（P6 同步）。
- [ ] 问题处理记录（可选）：`POST /api/v1/problems/{id}/logs`（现场补充）。**D3：APP 仅可 POST `action=COMMENT` 或 `REASSIGN`；问题状态变更走后端 `PUT /problems/{id}`，`STATUS_CHANGE` 日志由后端在同一事务自动追加，APP 永不直接 POST `STATUS_CHANGE`。**

#### 涉及模块/接口/表
模块：`:feature:problem`、`:feature:map`(落点)、`:core:geo`。表：`problems`(outbox)、`media_assets`、`dict_item`。接口：`problems`、`problems/{id}/logs`。

#### 关键实现要点与坑
- **GCJ-02 永不持久化**：仅地图渲染/点击瞬间转换，落库/上传一律 WGS84。任何把 GCJ-02 写进 `geom` 的代码都是 BUG。
- **dict_version_used 是历史容错关键**：拍照当时锁版本，离线数月后类型退役仍可上传（后端按 soft FK + 旧版本接受）。
- `type_item_id` 等是软 FK→`dict_item`（后端 `ON DELETE RESTRICT`），APP 不得发已不存在的 item id；用缓存里实际存在的（含退役）。
- 问题与媒体通过 `media_group`/`cover_media_id` 关联，上传顺序：先媒体（STS/COS）后问题引用（P6 编排）。

#### DoD
1. 发现问题→拍照→自动定位→地图微调→填表→保存 闭环（在线 + 离线）。
2. 落点坐标往返转换无明显漂移（`:core:geo` 单测保证），`geom` 始终 WGS84。
3. 选退役类型仍能保存且锁定旧版本。
4. 动态表单字段与字典一致。

#### 测试要求
- 单元：`CoordinateConverter` 往返误差（多组 WGS84↔GCJ-02 基准点）、`dict_version_used` 锁定逻辑、表单校验。
- 仪器：完整标注流（真机，可 Mock 相机产物）。
- 覆盖率：`:feature:problem`、`:core:geo` ≥ 80%（geo 模块要求 ≥ 90%）。

---

### Phase 6 — Room outbox + WorkManager 可续传上传 + 幂等 + capture state machine（P6）

#### 阶段目标
打通离线同步闭环：业务数据走 `sync/batch` 幂等批量；大媒体走 STS+COS 分块可续传 + HeadObject 确认；网络异常本地暂存、恢复重传。

#### 前置依赖
P2–P5（各类 outbox 已就绪）、P0（网络管道）。

#### 详细任务清单
**业务批量同步**
- [ ] `SyncWorker`（WorkManager，`NetworkType.CONNECTED` 约束 + 指数退避）：聚合 outbox（inspections/trajectory_points/problems/media 引用）→ `POST /api/v1/sync/batch`。
- [ ] 处理响应 `data.results[]`：`accepted`/`duplicate` → 标记本地已同步 + 存 `server_id`；`rejected` → 记 error，不阻塞其他项（per-item）。
- [ ] **幂等**：每项带 `client_uuid`（创建时已生成，P2–P5）；重放安全（后端 ON CONFLICT）。
- [ ] 轨迹点按 `(inspection_id,seq)` 去重；分批（避免超大 payload）。

**大媒体上传（STS + COS 分块可续传）**
- [ ] `MediaUploadWorker`（每个 media_group 一个工作单元）：
  1. `media_assets.capture_state=QUEUED` → 调 `POST /api/v1/media/upload-credentials`（body：`owner_type,owner_id,tier,content_type,byte_size,client_uuid`）→ 得 `{bucket,region,key,credentials,prefix}`（STS 仅授权 `prefix` + 6 个 multipart action）。
  2. 用 `cos-android` + STS 临时密钥**分块上传**（`capture_state=UPLOADING`）；**走公网（蜂窝），不经相机 WiFi**。断点续传：保存 uploadId/分块进度，恢复时续传（**不能用 presigned URL，因其无法续传 multipart**——约束）。
  3. 完成 → `POST /api/v1/media/confirm`（body：`client_uuid,key,etag,byte_size,width,height`）→ 后端 HeadObject 校验 ETag/size → 成功置 `capture_state=CONFIRMED`、`verified_at`；失败 409 `MEDIA_VERIFY_FAILED` → 保持 `UPLOADING`，重试。
- [ ] **capture state machine 全程驱动**（逐字）：`CAPTURED_RAW→STITCHED→QUEUED→UPLOADING→UPLOADED→CONFIRMED`（前向单向，绝不回退）；状态持久在 Room，崩溃可续。
- [ ] 编排顺序：媒体先 CONFIRMED → 再在 `sync/batch` 提交引用了该 `cover_media_id`/media 的 problem（保证引用有效）。
- [ ] 网络异常：进 outbox，WorkManager 退避重试；用户可见"待同步 N 项"。

**可续传 multipart 的 Room 持久化模型（断点续传关键）**
- [ ] `media_upload`（每个待传 media_asset 一行，与 `media_assets` outbox 通过 `client_uuid` 一对一）：

  | 列 | 类型 | 说明 |
  |---|---|---|
  | `client_uuid` | TEXT (PK) | 幂等键，关联 `media_assets` |
  | `local_path` | TEXT | scoped `panorama/<ts>.jpg` 绝对路径 |
  | `owner_type` / `owner_id` | TEXT | 多态归属 |
  | `tier` | TEXT | 固定 `original`（D4） |
  | `byte_size` | INTEGER | 文件总字节 |
  | `content_type` | TEXT | `image/jpeg` |
  | `width` / `height` | INTEGER | 拼接产物尺寸（confirm 用） |
  | `cos_bucket` / `cos_region` / `cos_key` | TEXT | upload-credentials 返回 |
  | `sts_tmp_secret_id` / `sts_tmp_secret_key` / `sts_token` | TEXT | STS 临时密钥（**不落明文，加密列**，过期即弃） |
  | `sts_expires_at` | INTEGER | 临时密钥过期时间（过期则重新换证再续传） |
  | `upload_id` | TEXT | COS multipart 的 `uploadId`（续传锚点；为空表示尚未 init） |
  | `part_size` | INTEGER | 分块大小（如 1MB/5MB，固定保证 part-number 对齐） |
  | `etag` | TEXT | confirm 时整体 ETag（完成后写回） |
  | `capture_state` | TEXT | 镜像 `media_assets.capture_state` |
  | `attempt_count` / `last_error` | INTEGER / TEXT | 退避与诊断 |
  | `created_at` / `updated_at` | INTEGER | |

- [ ] `media_upload_part`（已完成分块清单，续传时跳过已传块）：

  | 列 | 类型 | 说明 |
  |---|---|---|
  | `client_uuid` | TEXT | 外键→`media_upload` |
  | `part_number` | INTEGER | COS multipart part 序号（1-based） |
  | `part_etag` | TEXT | 该块上传返回 ETag（complete 时回传） |
  | `part_byte_offset` | INTEGER | 该块在文件中的起始偏移 |
  | `part_byte_size` | INTEGER | 该块实际字节 |
  | `uploaded_at` | INTEGER | 完成时间 |
  | — | — | **PK = (`client_uuid`,`part_number`)** |

- [ ] 续传算法：恢复时读 `media_upload.upload_id`；若存在则 `cos.listParts`/本地 `media_upload_part` 对账，仅上传缺失 part；全部 part 完成后 `completeMultipartUpload(partList=已收集的 part_number+part_etag)` → 拿整体 `etag` → `media/confirm`。STS 过期（`sts_expires_at`）→ 重新 `upload-credentials` 换证，**`upload_id` 与已传 part 不变**继续。

#### 涉及模块/接口/表
模块：`:feature:sync`、`:core:database`、`:core:network`。接口：`sync/batch`、`media/upload-credentials`、`media/confirm`。表：全 outbox + `media_assets`(capture_state)。

#### 关键实现要点与坑
- **大媒体绝不走 JSON API**（约束）；二进制只走 COS multipart。
- **STS 而非 presigned**：presigned 无法 resume multipart；STS 临时密钥支持续传。STS 权限 scoped 到 per-upload `prefix` + 仅 6 个 multipart action（最小权限）。
- **HeadObject 在后端**：APP 不假定上传成功即生效，必须等 `confirm` 200 才置 CONFIRMED；409 要能重传。
- per-item savepoint 在后端，APP 侧只需正确解析 per-item 结果，部分失败不回滚整批。
- 上传走蜂窝公网；若此时相机仍连着，注意 socket 路由不被相机网影响（P0 保证进程默认网络是蜂窝）。
- 历史字典容错：`sync/batch` 提交带旧 `dict_version_used` 的问题，预期 `accepted`，不要本地预先拒绝。

#### DoD
1. 全程离线采集（巡查+轨迹+问题+全景）→ 恢复网络 → 一键/自动同步全部成功。
2. 杀进程/断网中途的媒体上传可续传（不从头）。
3. media 状态机正确推进至 CONFIRMED；problem 引用有效。
4. 重复同步（重放）幂等，无重复数据（验证 `duplicate` 分支）。
5. `confirm` 失败（人为篡改 etag）→ 409 → 自动重传成功。

#### 测试要求
- 单元：状态机推进、`sync/batch` 结果解析（accepted/rejected/duplicate）、续传断点逻辑（基于 `media_upload`/`media_upload_part` 的 part 对账、STS 过期换证不丢 `upload_id`/已传 part）、编排顺序。
- 集成：MockWebServer 模拟 STS/confirm/sync 全链路（含 409、部分 rejected）。
- 仪器：真机离线→在线恢复重传 E2E。
- 覆盖率：`:feature:sync` ≥ 85%（核心可靠性模块）。

---

### Phase 7 — 腾讯地图展示 + 360 查看 + 字典动态表单 + 上架合规（P7）

#### 阶段目标
地图综合展示（当前位置/轨迹/问题点/详情/看全景/看描述）；本地 360 查看；动态表单收尾；完成上架合规打包。

#### 前置依赖
P3–P6（数据与媒体齐备）、P1（字典）。

#### 详细任务清单
**地图展示**
- [ ] 腾讯地图 Android SDK：当前位置（蓝点）、巡查轨迹 Polyline、问题点 Marker（聚合）。**所有 WGS84 数据渲染前经 `:core:geo` 转 GCJ-02**。
- [ ] 轨迹来源：本地 outbox + `GET /api/v1/inspections/{id}/trajectory`（回看历史，WGS84）。
- [ ] 问题点来源：本地 + `GET /api/v1/problems/map`（GeoJSON FeatureCollection，WGS84）→ 转 GCJ-02 落点。
- [ ] 点击 Marker → 详情卡（类型/状态/描述/时间）→ "看全景""看描述"。

**360 查看**
- [ ] `InstaImagePlayerView`：本地已拼 equirectangular JPG / `.insp` 用 `prepare(workWrapper, ImageParamsBuilder)` + `play()`；陀螺仪/手势。
- [ ] 远端全景：`GET /api/v1/media/{id}` 取签名 CDN URL，下载 equirectangular 后本地播放器渲染（APP 不依赖 PSV）。

**动态表单收尾 + 合规**
- [ ] 问题表单、项目自定义字段全部由字典/配置驱动（前几阶段已建，收口校验）。
- [ ] **上架合规**：
  - 隐私政策弹窗（首启同意，未同意不初始化定位/相机）；权限最小化与运行时申请文案（定位/相机/蓝牙/前台服务）。
  - **ICP 备案**：服务域名 ICP 备案（Day-1 运维项，APP 内嵌的 H5/域名须备案）。
  - 个人信息收集清单、第三方 SDK（Insta360/腾讯地图/腾讯云 COS）合规披露。
  - 目标应用商店（华为/小米/OPPO/vivo/应用宝）资质与隐私合规检查。
  - release 签名（独立 keystore，**不使用 Demo 的 `sdk.jks`/`insta360` 弱口令**）。

#### 涉及模块/接口/表
模块：`:feature:map`、`:feature:problem`、`:core:geo`。接口：`inspections/{id}/trajectory`、`problems/map`、`media/{id}`。

#### 关键实现要点与坑
- **唯一转换边界**：地图是唯一允许出现 GCJ-02 的地方；任何回写/上传前转回 WGS84。`:core:geo` 单测覆盖。
- SSE 不在本端核心（导出是 Web 端），APP 无需 SSE；如需作业进度用轮询。
- 360 查看复用 SDK 播放器（APP 有 SDK），与 Web 用 PSV 是两套，但**消费同一份 equirectangular + GPano**，需保证 P3 产物两端都能开。
- 签名/混淆/合规缺一不可过审。

#### DoD
1. 地图正确叠加当前位置+轨迹+问题点，点击详情/看全景/看描述可用，坐标无漂移。
2. 本地与远端全景均可 360 查看。
3. 隐私合规弹窗 + 权限文案齐全；release 用独立强口令签名。
4. 通过至少一个目标商店的合规预检；域名 ICP 备案完成。

#### 测试要求
- 单元：`:core:geo` 往返精度（≥90%）、GeoJSON→Marker 映射。
- 仪器：地图交互、360 播放（真机）。
- 合规：隐私弹窗门禁、权限拒绝降级。
- 覆盖率：整体 ≥ 80%。

---

## 6. 测试策略

### 6.1 分层与目标
| 层级 | 工具 | 范围 | 覆盖率目标 |
|---|---|---|---|
| 单元（JVM） | JUnit5 + MockK + Robolectric + Turbine | Repository / ViewModel / Mapper / 状态机 / `:core:geo` | ≥ 80%（geo/sync ≥ 85–90%） |
| 集成 | MockWebServer + Room in-memory | 网络封套/refresh、`sync/batch`、STS/confirm、字典 ETag/304 | 关键路径全覆盖 |
| 仪器 / E2E | AndroidX Test + Espresso（真机集，含相机的纳入夜间/手动） | 登录、开始/结束巡查、连接→拍照→下载→拼接、离线→恢复重传、地图/360 | 关键用户流 100% |

### 6.2 TDD
对状态机（巡查会话、capture state machine）、坐标转换、同步结果解析、ETag 缓存等**纯逻辑先写测试（RED→GREEN→REFACTOR）**。

### 6.3 隔离要点
- SDK 调用经 `CameraRepository`/`MediaRepository` 接口 Mock（`InstaCameraManager` 单例不可在 JVM 测）。
- 网络分离、相机连接、拍照、拼接 → 真机集（带 X3）；CI 跑 JVM + 仪器（无相机部分用 Mock）。
- Room 用 in-memory；时间用可注入 `Clock`（UTC）。

### 6.4 关键回归用例（必须长期保留）
- 下载 `.insp` 期间公网 API 不中断（socket 级绑定回归）。
- 离线创建→重放同步幂等无重复。
- 退役字典类型仍可上传。
- 坐标往返不漂移、绝无 GCJ-02 入库。

---

## 7. 安全与合规要点

- **机密管理**：access 仅内存；refresh + 相机凭据存 EncryptedSharedPreferences（替换 Demo 明文 SP）；STS 临时密钥不落盘、用完即弃；**release 签名独立强口令**（弃用 Demo `insta360` 弱口令）。
- **传输**：业务 API 全 HTTPS（公网/蜂窝）；cleartext **仅**对相机 `192.168.42.1` 放行（network_security_config domain-config），base-config 关闭 cleartext，移除 manifest 全局 `usesCleartextTraffic`。
- **混淆**：R8 开启 + `-keep com.arashivision.**`、`-keep com.qcloud.cos.**`、Retrofit/Moshi/Hilt keep。
- **存储**：FileProvider scoped 路径（不暴露整卡）；scoped storage；媒体/`.insp` 落应用专属目录。
- **权限最小化**：定位（前台服务+运行时）、相机 WiFi/蓝牙、前台服务（connectedDevice/location）；隐私政策同意门禁。
- **输入校验**：登录/改密/描述/表单在边界校验；不信任外部数据（API 响应用 schema 客户端 + 封套校验）。
- **数据合规**：GCJ-02 永不持久化/上传（地图坐标合规）；个人信息清单 + 第三方 SDK 披露；**ICP 备案**（Day-1）。
- **STS 最小权限**：scoped per-upload `prefix` + 仅 6 个 multipart action。
- **错误处理**：错误不泄露敏感信息；现场日志脱敏（XLog 不打 token/坐标精确值到导出日志）。

---

## 8. 部署 / 发布 / 上线清单

- [ ] 构建：`./gradlew :app:assembleRelease`（R8 开启、独立 keystore、arm64-v8a）。
- [ ] 校验 SDK native `.so` 打包正确（`pickFirsts libc++_shared.so`、SNPE skel `excludes`），无 x86。
- [ ] 验证混淆后 `InstaCameraSDK.init`/拍照/拼接/COS 上传/腾讯地图均正常（混淆回归）。
- [ ] 版本号策略（versionCode 单调递增、versionName 语义化），渠道包（如需）。
- [ ] 隐私政策、权限说明、第三方 SDK 清单就位。
- [ ] **ICP 备案**完成（域名）；后端/COS/CDN 环境就绪（签名 URL、CAM/STS 角色）。
- [ ] 目标商店资质与合规预检（华为/小米/OPPO/vivo/应用宝）。
- [ ] 灰度/内测分发（如 Firebase App Distribution 或腾讯/蒲公英），真机回归网络分离与离线同步。
- [ ] 崩溃监控/日志收集接入；版本回滚方案。
- [ ] 发版 changelog 与已知限制（X3 only、PHOTOS only）。

---

## 9. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 网络分离未彻底（误用全进程绑定） | 下载/拼接期间公网 API/上传/地图中断 | P0 强制 socket 级 `cameraNet.socketFactory`；保留回归用例；Code review 禁止 `bindProcessToNetwork(cameraNet)` |
| X3 与 X4+ 流程差异（initCameraSupportConfig） | 拍照初始化失败 | 机型分支，X3 跳过；真机验证 |
| `.insp`→equirectangular 拼接耗时/失败 | 上传阻塞、电量 | 后台 IO + 进度 + 重试；尺寸/质量走 `export.image` 可配 |
| GPano 缺失致 Web PSV 无法识别 | 全景在 Web 不可看 | P3 写 XMP，exiftool 验证 + 两端交叉验证 |
| COS multipart 续传复杂/中断 | 大媒体上传失败 | STS（非 presigned）+ 持久化 uploadId/分块；HeadObject confirm 把关 |
| 离线幂等/重放出错 | 数据重复或丢失 | client_uuid + per-item 结果解析 + 集成测试覆盖 duplicate/rejected |
| GCJ-02 误入库 | 坐标合规/数据污染 | `:core:geo` 唯一转换 + 单测 + review 红线 |
| 后台定位/前台服务被系统杀 | 轨迹断裂 | 前台服务 + 保活引导 + 恢复续采；杀进程恢复活动巡查 |
| SDK 私有 Maven/版本锁定（1.10.1） | 不可随意升级 | 封装在 `:sdk-camera`，接口隔离；锁版本 |
| 合规/备案延误 | 无法上线 | ICP 备案 Day-1 启动；隐私合规并行推进 |
| 弱口令签名/无混淆（沿用 Demo） | 安全/过审风险 | P0 强制独立 keystore + R8 keep 规则 |

---

附：关键文件与产物路径（绝对路径，供对照参考工程）
- 网络分离参考：`/Users/nanako/Dropbox/SProject/C5/Android-SDK-V1.10.1/sdkdemo/app/src/main/java/com/arashivision/sdk/demo/util/NetworkManager.kt`、`.../ui/connect/ConnectViewModel.kt`（`setNetIdToCamera` 调用点）
- 拍照+GPS 参考：`.../ui/shot/ShotViewModel.kt`（`startNormalCapture(gpsBytes)`/`startHDRCapture`、`GpsData` 映射）、`.../ui/capture/CaptureViewModel.kt`（`switchPanoramaSensorMode`、`initCameraSupportConfig` X4+ 流程）
- 下载+拼接参考：`.../ui/album/AlbumViewModel.kt`（`WorkUtils.getAllCameraWorks`/OkGo 下载——**本项目改 socket 级绑定**）、`.../ui/play/WorkPlayViewModel.kt`（`ExportUtils.exportImage` + `ExportImageParamsBuilder` PANORAMA/2:1）、`.../ui/play/WorkPlayActivity.kt`（`InstaImagePlayerView.prepare/play`）
- 工具链/打包参考：`/Users/nanako/Dropbox/SProject/C5/Android-SDK-V1.10.1/sdkdemo/gradle/libs.versions.toml`、`.../app/build.gradle.kts`、`.../app/src/main/AndroidManifest.xml`、`.../res/xml/network_security_config.xml`、`.../res/xml/file_paths.xml`
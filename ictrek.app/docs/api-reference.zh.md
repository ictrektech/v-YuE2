# v-YuE2 VOS API

## 地址和身份

VOS 同源前缀 `/app/com.ictrek.v-yue2`；容器 `http://v-yue2:8791`，兼容别名 v-yue2-backend / v-yue2-frontend。没有独立宿主机映射端口。稳定端口是Go VOS网关，不是每个用户动态启动的Rust worker端口。侧栏 API 文档提供本页。

业务请求带 `Authorization: Bearer $VOS_TOKEN`，或先用该请求头 POST `/vos/session` 换取1小时HttpOnly/SameSite Strict的 v_yue2_session cookie。此POST不是传access_token的JSON接口；响应包含 sub、can_manage_models、accepted、revision。之后cookie仍按VOS userinfo核验，不是永久访问Key。写请求如有Origin必须匹配Host。

没有个人访问 API Key CRUD接口，V-Scheduler Key/厂商Key不能当作本应用管理身份。用户工作区按不可变sub隔离；模型准备要求 system_admin/platform_admin 或 com.ictrek.v-yue2:model:manage 权限。

## 许可和最小接入流程

首次运行必须明确同意当前模型许可。读取 POST `/vos/session` 返回的revision和accepted后，POST `/vos/license` JSON `{revision:"RETURNED_REVISION",accepted:true}` 保存本用户同意，成功200 `{accepted:true}`。不要在不了解许可时自动代用户勾选。`GET /vos/about` 返回当前应用/镜像等版本。容器根路径的 `GET /health` 是公开网关健康；通过VOS应用前缀的health会走用户身份和运行时流程，不能把两者当成同一就绪信号。

```bash
BASE=http://v-yue2:8791
curl -X POST "$BASE/vos/session" -H "Authorization: Bearer $VOS_TOKEN"
curl "$BASE/vos/about" -H "Authorization: Bearer $VOS_TOKEN"
# 用户同意许可后查看准备状态与模型能力
curl "$BASE/setup/status" -H "Authorization: Bearer $VOS_TOKEN"
curl "$BASE/v1/capabilities" -H "Authorization: Bearer $VOS_TOKEN"
```

## Providers、模型与凭据修改

VOS的写作、封面、语音模型通过V-Scheduler发现和调用，并沿用当前VOS用户的模型可见性。`GET /v1/provider/settings` 返回配置状态（configured/provider），不是密钥值；`/v1/provider/*` 是VOS兼容映射。模型、上游、public/private和Key的新增修改在V-Scheduler进行。`PUT/POST /v1/provider/settings` 被403拒绝，旧 `/v1/openrouter/*` 返回410；不要根据桌面原生路由目录在VOS直接保存OpenRouter Key。

共享权重由V-Scheduler权重管理；`/setup/download` 和 `/setup/cancel` 要求模型管理权限，`/setup/remove` 返回409并要求到V-Scheduler处理共享权重。准备模型、安装文件、校准通过和可推理是不同状态；安装好仍可能等待执行器批准。普通用户不能通过下载接口删除其他用户共享权重。

## 音乐任务、资源与停止

POST `/v1/music/jobs` 输入CreateMusicJobRequest（完整字段在后文），例如 `{style:"ambient piano",lyrics:"[Instrumental]",duration_seconds:30,seed:1234}`。GET `/v1/music/jobs` 列活动任务，GET `/v1/music/jobs/ended` 列已结束任务；GET `/v1/music/jobs/{job_id}` 查询，POST同一路径取消，DELETE同一路径清理任务。不要把DELETE当作通用模型卸载接口。普通JSON返回原生对象，没有统一data包装。

MusicJob字段包括 id、client_ref?、submitted_at（Unix毫秒）、engine_id、status、dispatch、phase、style、lyrics、duration_seconds、generation_settings、song?、songs、message、playlist_id?等；可选字段可能省略。status为queued/running/completed/failed/cancelled；dispatch和phase描述排队/执行阶段，不是另一份最终结果。完成后从song/音乐库读取媒体，媒体接口返回原始音频/图片而不是JSON。

当前VOS的受管本地计算要求固定的YuE2 Q8权重组合及对应执行校准，缺绑定、文件、配置匹配或批准会503，不是“已下载就免预算”。忙时可能429；停止请求后网关等待原生进程停止和许可回执，不能仅凭断开HTTP判定资源已释放。中断/重启后终态任务保持私有，读取可以恢复；未知结果的POST不会被自动重放。

本应用当前支持任务头 `X-VScheduler-Priority: background|interactive|high`，默认background；请求许可时固定 preemptible=true、allow_preemption=false、force_admission=false，等待预算最多120秒。它没有把通用强制放行/抢占/等待头全部实现为任意可调参数，不能照搬其他应用的请求头说明。最终排队、预算及停止由调度器判定。

## 发现、绑定和校准 API

`GET /api/v1/scheduler/discovery` 返回非秘密scheduler元数据：protocol_version、app_id=com.ictrek.v-yue2、executor_base_url/upstream_url、binding_endpoint、execution_settings、sync_models、preemption_modes=[restart]。只有引擎和固定权重齐全才列 yue2-q8。GET binding返回 configured、binding_revision、approved_model_count，不回传凭证；这两个GET是网关早期只读元数据路径，不代表取得用户文件权限。

POST `/api/v1/scheduler/binding` 要求V-Scheduler VOS服务身份，不是普通管理员用户。请求字段 protocol_version=1、binding_revision（正整数）、app_id、url（固定controller地址）、token、executor_token、upstream_url、models（模型→executor）、model_limits、execution_settings。地址、当前引擎设置和执行器须严格匹配；JSON上限256KiB。非法422，活动执行/旧revision409，执行器验证失败503；同revision不能被当作随意覆盖配置的接口。应用自行维护注册/心跳/停止，不要求用户部署agent。

## VOS和桌面接口的范围

VOS明确拒绝 `/mcp*`（403），不能用桌面MCP文档宣称安装后可直接连接VOS MCP。桌面引擎路径选择、任意网络暴露/本机保存、训练软件安装、VST、部分本地ASR/分离/MIDI等写操作也被网关拒绝；某些未校准本地操作503。后文完整Rust目录是原生参数查找参考，**网关规则优先，列出不等于VOS放行**。独立桌面自动化看仓库 [MCP指南](../../docs/mcp-skill.md)。

网关错误通常为 `{error:string}`；400输入、401VOS身份、403权限/桌面操作、409许可/共享权重或绑定冲突、410旧Provider地址、429资源忙、502worker不可达、503绑定/校准不可用。Rust原生错误及上传、SSE、二进制接口保留各自格式。库内容、取消、删除、覆写、模型安装须分别处理，200不是整条生成流程已完成。

## 原生 API 参数查找

目录从实际Rust路由及serde类型生成，包含203项命名处理器HTTP操作和214项序列化声明。函数签名说明Path/Query/Json/Multipart/Bytes输入和返回类型；下方声明保留serde重命名、默认/可选字段及枚举。Rust声明是字段参考，不是可以直接发出的JSON。动态Value返回值不被伪造为固定schema；合并的Hub路由和VOS网关专用API由前述各自规则管理。

### GET /health

[health](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8892)

```rust
async fn health(&self) -> bool
```

### GET /v1/capabilities

[capabilities](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7659)

```rust
async fn capabilities(State(state): State<AppState>) -> Json<CapabilitiesResponse>
```

### GET /v1/configuration

[configuration](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4269)

```rust
async fn configuration(State(state): State<AppState>) -> Json<StudioConfiguration>
```

### PUT /v1/configuration

[update_configuration](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4273)

```rust
async fn update_configuration(
    State(state): State<AppState>,
    Json(update): Json<StudioConfiguration>,
) -> Result<Json<StudioConfiguration>, (StatusCode, Json<ApiError>)>
```

### GET /engine/options

[engine_options](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4367)

```rust
async fn engine_options(State(state): State<AppState>) -> Json<Value>
```

### PUT /engine/options

[update_engine_options](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4404)

```rust
async fn update_engine_options(
    State(state): State<AppState>,
    Json(mut request): Json<EngineOptions>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /engine/pick-executable

[pick_engine_executable](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4461)

```rust
async fn pick_engine_executable() -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /engine/restart

[restart_local_engine](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4452)

```rust
async fn restart_local_engine(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/engine/logs

[engine_logs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5304)

```rust
async fn engine_logs(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/engine/progress

[engine_progress](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5318)

```rust
async fn engine_progress(State(state): State<AppState>) -> Sse<impl futures_util::Stream<Item = Result<Event, std::convert::Infallible>>>
```

### GET /v1/system/resources

[system_resources](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5324)

```rust
async fn system_resources() -> Json<Value>
```

### GET /v1/proxy/image

[proxy_image](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5340)

```rust
async fn proxy_image(
    axum::extract::Query(request): axum::extract::Query<ProxyImageRequest>,
) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### GET /v1/openrouter/settings

[openrouter_settings](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7379)

```rust
async fn openrouter_settings() -> Json<Value>
```

### PUT /v1/openrouter/settings

[update_openrouter_settings](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7389)

```rust
async fn update_openrouter_settings(
    State(state): State<AppState>,
    Json(request): Json<OpenRouterSettingsRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/openrouter/logs

[openrouter_logs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7375)

```rust
async fn openrouter_logs() -> Json<Value>
```

### POST /v1/files/save

[saving::choose](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/saving.rs#L57)

```rust
pub async fn choose(Json(request): Json<Destination>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/files/save/{id}

[saving::write](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/saving.rs#L112)

```rust
pub async fn write(Path(id): Path<String>, headers: HeaderMap, body: Body) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/files/reveal

[saving::reveal](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/saving.rs#L215)

```rust
pub async fn reveal(Json(file): Json<Written>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### GET /v1/skins

[skins::list](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/skins.rs#L49)

```rust
pub async fn list() -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/skins

[skins::add](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/skins.rs#L67)

```rust
pub async fn add(headers: HeaderMap, body: Bytes) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/skins/file/{name}

[skins::file](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/skins.rs#L87)

```rust
pub async fn file(Path(name): Path<String>) -> Result<Response, (StatusCode, Json<ApiError>)>
```

### GET /v1/network/proxy

[read_proxy](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4217)

```rust
async fn read_proxy() -> Json<Value>
```

### PUT /v1/network/proxy

[update_proxy](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4226)

```rust
async fn update_proxy(
    State(state): State<AppState>,
    Json(settings): Json<net::ProxySettings>,
) -> Result<Json<net::ProxySettings>, (StatusCode, Json<ApiError>)>
```

### POST /v1/network/proxy/test

[test_proxy](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4238)

```rust
async fn test_proxy(Json(settings): Json<net::ProxySettings>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/assistant/status

[assistant_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5398)

```rust
async fn assistant_status(State(state): State<AppState>) -> Json<Value>
```

### PUT /v1/assistant/status

[update_assistant_settings](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5568)

```rust
async fn update_assistant_settings(
    State(state): State<AppState>,
    Json(incoming): Json<AssistantSettingsRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/assistant/local-models

[assistant_local_models](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5489)

```rust
async fn assistant_local_models(
    axum::extract::Query(query): axum::extract::Query<LocalModelsQuery>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/assistant/cli-agents

[assistant_cli_agents](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5518)

```rust
async fn assistant_cli_agents() -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/local-key

[set_local_server_key](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5541)

```rust
async fn set_local_server_key(Json(request): Json<LocalServerKeyRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/write

[assistant_write](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6572)

```rust
async fn assistant_write(
    State(state): State<AppState>,
    Json(request): Json<assistant::AssistRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/write/stream

[assistant_write_stream](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6217)

```rust
async fn assistant_write_stream(
    State(state): State<AppState>,
    Json(request): Json<assistant::AssistRequest>,
) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/sections

[assistant_sections](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6588)

```rust
async fn assistant_sections(
    State(state): State<AppState>,
    Json(request): Json<LyricsSectionsRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/assistant/runtime

[assistant_runtime_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5641)

```rust
async fn assistant_runtime_status(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/assistant/runtime/install

[assistant_runtime_install](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5674)

```rust
async fn assistant_runtime_install(
    State(state): State<AppState>,
    Json(request): Json<AssistantAssetRequest>,
) -> Result<Json<assistant_runtime::RuntimeStatus>, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/runtime/cancel

[cancel_assistant_download](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1758)

```rust
async fn cancel_assistant_download(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/assistant/runtime/remove

[assistant_runtime_remove](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1732)

```rust
async fn assistant_runtime_remove(
    State(state): State<AppState>,
    Json(request): Json<AssistantAssetRequest>,
) -> Result<Json<assistant_runtime::RuntimeStatus>, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/runtime/start

[assistant_runtime_start](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5726)

```rust
async fn assistant_runtime_start(
    State(state): State<AppState>,
    Json(request): Json<AssistantModelRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/assistant/runtime/stop

[assistant_runtime_stop](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5741)

```rust
async fn assistant_runtime_stop(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/karaoke/status

[karaoke_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5766)

```rust
async fn karaoke_status(State(state): State<AppState>) -> Json<Value>
```

### PUT /v1/karaoke/status

[update_karaoke_settings](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5822)

```rust
async fn update_karaoke_settings(
    State(state): State<AppState>,
    Json(request): Json<KaraokeSettingsRequest>,
) -> Result<Json<lyrics_sync::SyncStatus>, (StatusCode, Json<ApiError>)>
```

### POST /v1/karaoke/install

[karaoke_install](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5928)

```rust
async fn karaoke_install(
    State(state): State<AppState>,
    Json(request): Json<AssistantAssetRequest>,
) -> Result<Json<lyrics_sync::SyncStatus>, (StatusCode, Json<ApiError>)>
```

### POST /v1/karaoke/cancel

[cancel_karaoke_download](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1724)

```rust
async fn cancel_karaoke_download(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/karaoke/remove

[karaoke_remove](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5850)

```rust
async fn karaoke_remove(
    State(state): State<AppState>,
    Json(request): Json<AssistantAssetRequest>,
) -> Result<Json<lyrics_sync::SyncStatus>, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/karaoke

[create_song_karaoke](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6080)

```rust
async fn create_song_karaoke(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<KaraokeRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/library/songs/{id}/karaoke

[delete_song_karaoke](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6129)

```rust
async fn delete_song_karaoke(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/openrouter/catalog

[openrouter_catalog](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4765)

```rust
async fn openrouter_catalog(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/openrouter/catalog/refresh

[refresh_openrouter_catalog](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4879)

```rust
async fn refresh_openrouter_catalog(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/openrouter/transcriptions

[create_openrouter_transcription](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4969)

```rust
async fn create_openrouter_transcription(
    State(state): State<AppState>,
    Json(input): Json<OpenRouterTranscriptionRequest>,
) -> Result<Json<OpenRouterResponse>, (StatusCode, Json<ApiError>)>
```

### POST /v1/openrouter/covers

[create_openrouter_cover](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4993)

```rust
async fn create_openrouter_cover(
    State(state): State<AppState>,
    Json(input): Json<OpenRouterCoverRequest>,
) -> Result<Json<OpenRouterResponse>, (StatusCode, Json<ApiError>)>
```

### GET /editor/{*path}

[editor_asset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1460)

```rust
async fn editor_asset(Path(path): Path<String>) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### GET /v1/separation/runtime

[separation_assets](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1631)

```rust
async fn separation_assets(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/separation/runtime/install

[install_separation_asset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1763)

```rust
async fn install_separation_asset(
    State(state): State<AppState>,
    Json(request): Json<InstallSeparationAssetRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/separation/runtime/cancel

[cancel_separation_download](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1718)

```rust
async fn cancel_separation_download(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/separation/status

[separation_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1874)

```rust
async fn separation_status(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/separation/settings

[read_separation_settings](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1853)

```rust
async fn read_separation_settings(State(state): State<AppState>) -> Json<separation::SeparationConfig>
```

### PUT /v1/separation/settings

[write_separation_settings](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1857)

```rust
async fn write_separation_settings(
    State(state): State<AppState>,
    Json(config): Json<separation::SeparationConfig>,
) -> Result<Json<separation::SeparationConfig>, (StatusCode, Json<ApiError>)>
```

### POST /v1/separation/install

[install_separation_model](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1931)

```rust
async fn install_separation_model(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/separation/remove

[remove_separation_model](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5873)

```rust
async fn remove_separation_model(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/adapters

[list_adapters](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1949)

```rust
async fn list_adapters(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/adapters/import

[import_adapter](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2025)

```rust
async fn import_adapter(
    State(state): State<AppState>,
    mut multipart: Multipart,
) -> Result<(StatusCode, Json<adapters::AdapterMeta>), (StatusCode, Json<ApiError>)>
```

### POST /v1/adapters/import-folder

[import_adapter_folder](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2059)

```rust
async fn import_adapter_folder(State(state): State<AppState>, body: axum::body::Bytes) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/adapters/cancel

[cancel_adapter_download](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2018)

```rust
async fn cancel_adapter_download(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/adapters/install

[install_catalog_adapters](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1973)

```rust
async fn install_catalog_adapters(
    State(state): State<AppState>,
    Json(input): Json<InstallAdaptersRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/adapters/hub

[search_hub_adapters](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1989)

```rust
async fn search_hub_adapters(State(state): State<AppState>, Query(query): Query<HubQuery>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/adapters/hub/files

[list_hub_files](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1996)

```rust
async fn list_hub_files(State(state): State<AppState>, Query(query): Query<HubQuery>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/adapters/hub/install

[install_hub_adapters](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2009)

```rust
async fn install_hub_adapters(State(state): State<AppState>, Json(input): Json<InstallHubRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### PATCH /v1/adapters/{id}

[update_adapter](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2111)

```rust
async fn update_adapter(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(patch): Json<adapters::Patch>,
) -> Result<Json<adapters::AdapterMeta>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/adapters/{id}

[delete_adapter](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2119)

```rust
async fn delete_adapter(State(state): State<AppState>, Path(id): Path<String>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/process

[start_processing](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2126)

```rust
async fn start_processing(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<processing::ProcessRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### PUT /v1/library/songs/{id}/version

[select_song_version](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2402)

```rust
async fn select_song_version(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(input): Json<SelectVersionRequest>,
) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/library/songs/{id}/versions/{version}

[remove_song_version](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2416)

```rust
async fn remove_song_version(
    State(state): State<AppState>,
    Path((id, version)): Path<(String, String)>,
) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### GET /v1/processing

[read_processing](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2271)

```rust
async fn read_processing(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/processing/vst

[read_vst](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2239)

```rust
async fn read_vst() -> Json<Value>
```

### POST /v1/processing/vst/scan

[scan_vst](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2244)

```rust
async fn scan_vst() -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/processing/vst/editor

[open_vst_editor](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2263)

```rust
async fn open_vst_editor(Json(input): Json<VstEditorRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/processing/preview

[processing_preview](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2275)

```rust
async fn processing_preview(State(state): State<AppState>, request: Request) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### POST /v1/processing/keep

[keep_processing](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2291)

```rust
async fn keep_processing(
    State(state): State<AppState>,
    Json(input): Json<KeepProcessingRequest>,
) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### POST /v1/processing/discard

[discard_processing](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2360)

```rust
async fn discard_processing(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/processing/reference

[upload_processing_reference](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2371)

```rust
async fn upload_processing_reference(
    State(state): State<AppState>,
    mut multipart: Multipart,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/training

[read_training](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2533)

```rust
async fn read_training(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/training/pack/install

[install_training_pack](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2687)

```rust
async fn install_training_pack(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/pack/cancel

[cancel_training_pack](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2726)

```rust
async fn cancel_training_pack(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/training/datasets

[create_training_dataset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2741)

```rust
async fn create_training_dataset(State(state): State<AppState>, Json(input): Json<DatasetInput>) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/datasets/import

[import_training_dataset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2777)

```rust
async fn import_training_dataset(State(state): State<AppState>, mut multipart: Multipart) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/datasets/{id}/reveal

[reveal_training_dataset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2801)

```rust
async fn reveal_training_dataset(State(state): State<AppState>, Path(id): Path<String>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### PATCH /v1/training/datasets/{id}

[update_training_dataset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2813)

```rust
async fn update_training_dataset(State(state): State<AppState>, Path(id): Path<String>, Json(input): Json<DatasetInput>) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/training/datasets/{id}

[delete_training_dataset](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2817)

```rust
async fn delete_training_dataset(State(state): State<AppState>, Path(id): Path<String>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/datasets/{id}/songs

[add_training_songs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2832)

```rust
async fn add_training_songs(State(state): State<AppState>, Path(id): Path<String>, Json(input): Json<DatasetSongs>) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/datasets/{id}/files

[upload_training_files](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2859)

```rust
async fn upload_training_files(State(state): State<AppState>, Path(id): Path<String>, mut multipart: Multipart) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### PATCH /v1/training/datasets/{id}/items/{item}

[update_training_item](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2913)

```rust
async fn update_training_item(
    State(state): State<AppState>,
    Path((id, item)): Path<(String, String)>,
    Json(patch): Json<training::ItemPatch>,
) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/training/datasets/{id}/items/{item}

[delete_training_item](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2921)

```rust
async fn delete_training_item(State(state): State<AppState>, Path((id, item)): Path<(String, String)>) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### GET /v1/training/datasets/{id}/items/{item}/audio

[training_item_audio](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1406)

```rust
async fn training_item_audio(State(state): State<AppState>, Path((id, item)): Path<(String, String)>, request: Request) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/datasets/{id}/prepare

[prepare::start](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs#L178)

```rust
pub async fn start(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<PrepareRequest>,
) -> Result<Json<PrepareStatus>, (StatusCode, Json<ApiError>)>
```

### POST /v1/lyrics/find

[find_lyrics](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7358)

```rust
async fn find_lyrics(Json(request): Json<FindLyricsRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/lyrics/syllables

[lyric_syllables](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2100)

```rust
async fn lyric_syllables(Json(request): Json<SyllablesRequest>) -> Json<Value>
```

### POST /v1/videos

[store_video](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7333)

```rust
async fn store_video(Query(query): Query<VideoName>, body: axum::body::Bytes) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/writing/guide

[writing_guide](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7305)

```rust
async fn writing_guide(Query(query): Query<GuideQuery>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/writing/examples

[writing_examples](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7321)

```rust
async fn writing_examples(Query(query): Query<ExamplesQuery>) -> Json<Value>
```

### GET /v1/library/songs/{id}/files

[library_song_files](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7276)

```rust
async fn library_song_files(State(state): State<AppState>, Path(id): Path<String>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/midi

[midi_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6826)

```rust
async fn midi_status(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/midi/runtime

[midi_runtime](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2664)

```rust
async fn midi_runtime(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/training/pack/runtime

[training_pack_runtime](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2646)

```rust
async fn training_pack_runtime(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/training/listen/runtime

[listen_pack_runtime](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2656)

```rust
async fn listen_pack_runtime(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/midi/notes

[midi_live_notes](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6848)

```rust
async fn midi_live_notes(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/midi/install

[install_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6860)

```rust
async fn install_midi(State(state): State<AppState>, Json(input): Json<MidiSizeRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/midi/remove

[remove_midi_model](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6877)

```rust
async fn remove_midi_model(State(state): State<AppState>, Json(input): Json<MidiSizeRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/midi/cancel

[cancel_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6884)

```rust
async fn cancel_midi(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/midi/transcribe

[start_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6906)

```rust
async fn start_midi(State(state): State<AppState>, Json(input): Json<MidiRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}/midi

[read_song_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7193)

```rust
async fn read_song_midi(State(state): State<AppState>, Path(id): Path<String>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### PUT /v1/library/songs/{id}/midi

[write_song_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7229)

```rust
async fn write_song_midi(State(state): State<AppState>, Path(id): Path<String>, Json(body): Json<MidiUpload>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/library/songs/{id}/midi

[delete_song_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7255)

```rust
async fn delete_song_midi(State(state): State<AppState>, Path(id): Path<String>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}/midi/file

[song_midi_file](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7210)

```rust
async fn song_midi_file(State(state): State<AppState>, Path(id): Path<String>) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### GET /v1/training/datasets/{id}/items/{item}/files

[dataset_song_files](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7292)

```rust
async fn dataset_song_files(State(state): State<AppState>, Path((id, item)): Path<(String, String)>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/prepare/cancel

[prepare::cancel](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs#L416)

```rust
pub async fn cancel(State(state): State<AppState>) -> Json<serde_json::Value>
```

### POST /v1/training/datasets/{id}/take-as-is

[prepare::take_as_is](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs#L360)

```rust
pub async fn take_as_is(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<TakeAsIsRequest>,
) -> Result<Json<training::Dataset>, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/describe-style

[prepare::describe_song_style](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs#L301)

```rust
pub async fn describe_song_style(State(state): State<AppState>, Path(song_id): Path<String>) -> Result<Json<serde_json::Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/system/gpus

[system_gpus](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5171)

```rust
async fn system_gpus(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/network

[remote::status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/remote.rs#L131)

```rust
pub async fn status(ConnectInfo(peer): ConnectInfo<SocketAddr>, headers: axum::http::HeaderMap) -> Json<Value>
```

### PUT /v1/network

[remote::change](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/remote.rs#L153)

```rust
pub async fn change(
    axum::extract::State(state): axum::extract::State<crate::AppState>,
    ConnectInfo(peer): ConnectInfo<SocketAddr>,
    headers: axum::http::HeaderMap,
    Json(change): Json<AccessChange>,
) -> Result<Json<Value>, (StatusCode, Json<crate::ApiError>)>
```

### GET /v1/adapters/{id}/comfyui

[export_adapter_comfyui](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8122)

```rust
async fn export_adapter_comfyui(
    State(state): State<AppState>,
    Path(id): Path<String>,
    axum::extract::Query(query): axum::extract::Query<ComfyExportQuery>,
) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### POST /v1/adapters/{id}/comfyui

[save_adapter_comfyui](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8182)

```rust
async fn save_adapter_comfyui(State(state): State<AppState>, Path(id): Path<String>, Json(request): Json<ComfySaveRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/score/midi

[score::api::midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs#L118)

```rust
pub async fn midi(Json(request): Json<MidiRequest>) -> Answer
```

### POST /v1/score/instrumental

[score::api::instrumental](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs#L129)

```rust
pub async fn instrumental(Json(request): Json<MidiRequest>) -> Answer
```

### POST /v1/score/vocal-octave

[score::api::vocal_octave](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs#L169)

```rust
pub async fn vocal_octave(Json(request): Json<VocalOctaveRequest>) -> Answer
```

### POST /v1/score/chord-bed

[score::api::chord_bed](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs#L151)

```rust
pub async fn chord_bed(Json(request): Json<ChordBedRequest>) -> Answer
```

### POST /v1/score/sections

[match_score_sections](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L6017)

```rust
async fn match_score_sections(
    State(state): State<AppState>,
    mut multipart: axum::extract::Multipart,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/score/from-midi

[score::api::from_midi](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs#L80)

```rust
pub async fn from_midi(Json(request): Json<FromMidiRequest>) -> Answer
```

### POST /v1/song/tokenize

[song_tokenizer::tokenize](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/song_tokenizer.rs#L56)

```rust
pub async fn tokenize(State(state): State<AppState>, Json(request): Json<Request>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/score/mark

[score::api::mark](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs#L46)

```rust
pub async fn mark(Json(request): Json<MarkRequest>) -> Answer
```

### POST /v1/training/prepare/train-after

[prepare::set_train_after](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs#L290)

```rust
pub async fn set_train_after(State(state): State<AppState>, Json(request): Json<TrainAfterRequest>) -> Json<serde_json::Value>
```

### POST /v1/training/listen/install

[install_listen_pack](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2709)

```rust
async fn install_listen_pack(State(state): State<AppState>) -> Json<Value>
```

### POST /v1/training/runs

[start_training](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L2938)

```rust
async fn start_training(State(state): State<AppState>, Json(input): Json<StartTraining>) -> Result<Json<training::Run>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/runs/{id}/cancel

[cancel_training](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3084)

```rust
async fn cancel_training(State(state): State<AppState>, Path(id): Path<String>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/runs/{id}/continue

[continue_training](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3073)

```rust
async fn continue_training(State(state): State<AppState>, Path(id): Path<String>, Json(input): Json<ContinueTraining>) -> Result<Json<training::Run>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/training/runs/{id}

[delete_training_run](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3089)

```rust
async fn delete_training_run(State(state): State<AppState>, Path(id): Path<String>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### POST /v1/training/runs/{id}/checkpoints/{step}/install

[install_training_checkpoint](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3101)

```rust
async fn install_training_checkpoint(
    State(state): State<AppState>,
    Path((id, step)): Path<(String, u32)>,
    Json(input): Json<InstallCheckpoint>,
) -> Result<Json<adapters::AdapterMeta>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}/stems

[read_stems](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3123)

```rust
async fn read_stems(State(state): State<AppState>, Path(id): Path<String>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/stems

[start_separation](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3157)

```rust
async fn start_separation(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}/stems/{stem}

[read_stem_audio](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3138)

```rust
async fn read_stem_audio(
    State(state): State<AppState>,
    Path((id, stem)): Path<(String, String)>,
    request: Request,
) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/cover/auto

[draw_cover_now](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3281)

```rust
async fn draw_cover_now(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/activity

[read_activity](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3333)

```rust
async fn read_activity(State(state): State<AppState>) -> Json<Value>
```

### GET /v1/cover-templates

[read_cover_templates](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3567)

```rust
async fn read_cover_templates(State(state): State<AppState>) -> Json<Value>
```

### PUT /v1/cover-templates

[write_cover_templates](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3576)

```rust
async fn write_cover_templates(
    State(state): State<AppState>,
    Json(request): Json<CoverTemplatesRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/file-tags

[read_file_tags](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3606)

```rust
async fn read_file_tags(State(state): State<AppState>) -> Json<Value>
```

### PUT /v1/library/file-tags

[write_file_tags](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3611)

```rust
async fn write_file_tags(State(state): State<AppState>, Json(request): Json<FileTagsRequest>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/cover-templates/render

[render_cover_template](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3633)

```rust
async fn render_cover_template(
    State(state): State<AppState>,
    Json(request): Json<RenderCoverPromptRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/openrouter/completions

[create_openrouter_completion](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L5016)

```rust
async fn create_openrouter_completion(
    State(state): State<AppState>,
    Json(input): Json<OpenRouterCompletionRequest>,
) -> Result<Json<OpenRouterResponse>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs

[library_songs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1329)

```rust
async fn library_songs(State(state): State<AppState>) -> Result<Json<Vec<library::Song>>, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs

[create_library_song](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4105)

```rust
async fn create_library_song(State(state):State<AppState>,Json(input):Json<library::SongInput>)->Result<(StatusCode,Json<library::Song>),(StatusCode,Json<ApiError>)>
```

### POST /v1/library/import

[import_library_audio](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4106)

```rust
async fn import_library_audio(State(state): State<AppState>, mut multipart: Multipart) -> Result<(StatusCode, Json<library::Song>), (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}

[library_song](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1398)

```rust
async fn library_song(State(state): State<AppState>,Path(id):Path<String>)->Result<Json<library::Song>,(StatusCode,Json<ApiError>)>
```

### PUT /v1/library/songs/{id}

[update_library_song](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4132)

```rust
async fn update_library_song(State(state):State<AppState>,Path(id):Path<String>,Json(input):Json<library::SongInput>)->Result<Json<library::Song>,(StatusCode,Json<ApiError>)>
```

### DELETE /v1/library/songs/{id}

[delete_library_song](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4155)

```rust
async fn delete_library_song(State(state): State<AppState>, Path(id): Path<String>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### PUT /v1/library/songs/{id}/liked

[set_library_song_liked](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1353)

```rust
async fn set_library_song_liked(State(state): State<AppState>, Path(id): Path<String>, Json(input): Json<LikeInput>) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### PUT /v1/library/songs/{id}/note

[set_library_song_note](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1342)

```rust
async fn set_library_song_note(State(state): State<AppState>, Path(id): Path<String>, Json(input): Json<NoteInput>) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/liked

[library_liked](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1363)

```rust
async fn library_liked(State(state): State<AppState>) -> Result<Json<Vec<library::Song>>, (StatusCode, Json<ApiError>)>
```

### GET /v1/journal

[read_journal](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1369)

```rust
async fn read_journal(State(state): State<AppState>) -> Result<Json<Vec<library::JournalEntry>>, (StatusCode, Json<ApiError>)>
```

### POST /v1/journal

[write_journal](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1375)

```rust
async fn write_journal(State(state): State<AppState>, Json(entry): Json<library::JournalEntry>) -> Result<Json<library::JournalEntry>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/journal

[clear_journal](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1387)

```rust
async fn clear_journal(State(state): State<AppState>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/journal/{id}

[remove_journal_entry](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1391)

```rust
async fn remove_journal_entry(State(state): State<AppState>, Path(id): Path<i64>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/media/{song_id}

[library_media](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L1399)

```rust
async fn library_media(State(state): State<AppState>, Path(song_id): Path<String>, request: Request) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}/cover

[library_cover](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3661)

```rust
async fn library_cover(State(state): State<AppState>, Path(id): Path<String>) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### PUT /v1/library/songs/{id}/cover

[store_library_cover](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4087)

```rust
async fn store_library_cover(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<StoreCoverRequest>,
) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### GET /v1/covers/look

[read_cover_look](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3778)

```rust
async fn read_cover_look(State(state): State<AppState>) -> Json<Value>
```

### PUT /v1/covers/look

[write_cover_look](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3784)

```rust
async fn write_cover_look(
    State(state): State<AppState>,
    Json(request): Json<CoverLookRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/songs/{id}/cover/placeholder

[placeholder_cover](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L3950)

```rust
async fn placeholder_cover(State(state): State<AppState>, Path(id): Path<String>) -> Result<axum::response::Response, (StatusCode, Json<ApiError>)>
```

### GET /v1/media/scenes

[media_scenes](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4006)

```rust
async fn media_scenes(Query(query): Query<MediaStyle>) -> Json<Value>
```

### GET /v1/media/photos

[media_photos](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4011)

```rust
async fn media_photos(Query(query): Query<MediaSearch>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/media/videos

[media_videos](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4024)

```rust
async fn media_videos(Query(query): Query<MediaSearch>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/cover/photo

[choose_cover_photo](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4044)

```rust
async fn choose_cover_photo(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<ChosenCoverPhoto>,
) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### POST /v1/library/songs/{id}/cover/pattern

[choose_cover_pattern](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4067)

```rust
async fn choose_cover_pattern(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(request): Json<ChosenCoverPattern>,
) -> Result<Json<library::Song>, (StatusCode, Json<ApiError>)>
```

### GET /v1/library/playlists

[library_playlists](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4209)

```rust
async fn library_playlists(State(state):State<AppState>)->Result<Json<Vec<library::Playlist>>,(StatusCode,Json<ApiError>)>
```

### POST /v1/library/playlists

[create_library_playlist](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4210)

```rust
async fn create_library_playlist(State(state):State<AppState>,Json(input):Json<library::PlaylistInput>)->Result<(StatusCode,Json<library::Playlist>),(StatusCode,Json<ApiError>)>
```

### GET /v1/library/playlists/{id}

[library_playlist](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4211)

```rust
async fn library_playlist(State(state):State<AppState>,Path(id):Path<String>)->Result<Json<library::Playlist>,(StatusCode,Json<ApiError>)>
```

### PUT /v1/library/playlists/{id}

[update_library_playlist](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4212)

```rust
async fn update_library_playlist(State(state):State<AppState>,Path(id):Path<String>,Json(input):Json<library::PlaylistInput>)->Result<Json<library::Playlist>,(StatusCode,Json<ApiError>)>
```

### DELETE /v1/library/playlists/{id}

[delete_library_playlist](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L4213)

```rust
async fn delete_library_playlist(State(state):State<AppState>,Path(id):Path<String>)->Result<StatusCode,(StatusCode,Json<ApiError>)>
```

### GET /setup/status

[setup_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7416)

```rust
async fn setup_status(State(state): State<AppState>) -> Json<Value>
```

### GET /setup/catalog

[setup_catalog](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7568)

```rust
async fn setup_catalog(State(state): State<AppState>) -> Json<model_manager::Catalog>
```

### POST /setup/download

[setup_download](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7572)

```rust
async fn setup_download(
    State(state): State<AppState>,
    Json(request): Json<SetupDownloadRequest>,
) -> Result<(StatusCode, Json<model_manager::DownloadJob>), (StatusCode, Json<ApiError>)>
```

### POST /setup/remove

[setup_remove](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7426)

```rust
async fn setup_remove(
    State(state): State<AppState>,
    Json(request): Json<SetupDownloadRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /setup/adopt

[setup_adopt](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7486)

```rust
async fn setup_adopt(State(state): State<AppState>, body: axum::body::Bytes) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /v1/open-data-directory

[open_data_directory](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7553)

```rust
async fn open_data_directory() -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /setup/select

[setup_select](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7618)

```rust
async fn setup_select(
    State(state): State<AppState>,
    Json(request): Json<SetupSelectRequest>,
) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### POST /setup/cancel

[setup_cancel](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7649)

```rust
async fn setup_cancel(State(state): State<AppState>) -> Result<Json<Value>, (StatusCode, Json<ApiError>)>
```

### GET /v1/local-models/music

[local_music_model_catalog](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8627)

```rust
async fn local_music_model_catalog(
    State(state): State<AppState>,
) -> Result<Json<LocalMusicModelCatalog>, (StatusCode, Json<ApiError>)>
```

### POST /v1/music/jobs

[create_music_job](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7789)

```rust
async fn create_music_job(
    State(state): State<AppState>,
    Json(request): Json<CreateMusicJobRequest>,
) -> (StatusCode, Json<MusicJob>)
```

### GET /v1/music/jobs

[list_active_music_jobs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8084)

```rust
async fn list_active_music_jobs(State(state): State<AppState>) -> Json<Vec<MusicJob>>
```

### POST /v1/music/replay

[replay_music_job](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L7896)

```rust
async fn replay_music_job(
    State(state): State<AppState>,
    Json(request): Json<ReplayMusicJobRequest>,
) -> Result<(StatusCode, Json<MusicJob>), (StatusCode, Json<ApiError>)>
```

### POST /v1/transcriptions

[create_transcription](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8756)

```rust
async fn create_transcription(
    State(state): State<AppState>,
    mut multipart: Multipart,
) -> Result<(StatusCode, Json<ScoreJob>), (StatusCode, Json<ApiError>)>
```

### GET /v1/transcriptions/{job_id}

[score_job_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8852)

```rust
async fn score_job_status(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<ScoreJob>, (StatusCode, Json<ApiError>)>
```

### POST /v1/transcriptions/{job_id}

[cancel_score_job](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8872)

```rust
async fn cancel_score_job(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<ScoreJob>, (StatusCode, Json<ApiError>)>
```

### POST /v1/scores

[compose_score](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8738)

```rust
async fn compose_score(
    State(state): State<AppState>,
    Json(request): Json<ComposeScoreRequest>,
) -> Result<(StatusCode, Json<ScoreJob>), (StatusCode, Json<ApiError>)>
```

### GET /v1/scores/{job_id}

[score_job_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8852)

```rust
async fn score_job_status(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<ScoreJob>, (StatusCode, Json<ApiError>)>
```

### POST /v1/scores/{job_id}

[cancel_score_job](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8872)

```rust
async fn cancel_score_job(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<ScoreJob>, (StatusCode, Json<ApiError>)>
```

### GET /v1/music/jobs/ended

[list_ended_music_jobs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8052)

```rust
async fn list_ended_music_jobs(State(state): State<AppState>) -> Json<Vec<MusicJob>>
```

### GET /v1/music/jobs/{job_id}

[music_job_status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8098)

```rust
async fn music_job_status(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<MusicJob>, (StatusCode, Json<ApiError>)>
```

### POST /v1/music/jobs/{job_id}

[cancel_music_job](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8565)

```rust
async fn cancel_music_job(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<MusicJob>, (StatusCode, Json<ApiError>)>
```

### DELETE /v1/music/jobs/{job_id}

[dismiss_music_job](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs#L8066)

```rust
async fn dismiss_music_job(State(state): State<AppState>, Path(job_id): Path<String>) -> Result<StatusCode, (StatusCode, Json<ApiError>)>
```

### POST /mcp

[mcp::handle](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs#L2679)

```rust
pub async fn handle(headers: HeaderMap, body: axum::body::Bytes) -> Response
```

### GET /mcp/status

[mcp::status](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs#L853)

```rust
pub async fn status() -> Json<Value>
```

### GET /mcp/window

[mcp::window_events](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs#L644)

```rust
pub async fn window_events(headers: HeaderMap) -> Response
```

### POST /mcp/window/result

[mcp::window_result](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs#L674)

```rust
pub async fn window_result(headers: HeaderMap, Json(answer): Json<WindowAnswer>) -> StatusCode
```

### POST /mcp/window/focus

[mcp::window_focus](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs#L698)

```rust
pub async fn window_focus(headers: HeaderMap, Json(focus): Json<WindowFocus>) -> StatusCode
```

## 原生字段与枚举

### Text

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
enum Text {
    Plain(String),
    Localized(BTreeMap<String, String>),
}
```

### CatalogFile

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct CatalogFile {
    url: String,
    file: String,
    bytes: u64,
}
```

### CatalogEntry

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct CatalogEntry {
    id: String,
    engine: String,
    kind: String,
    #[serde(default)]
    author: Option<String>,
    #[serde(default)]
    page: Option<String>,
    #[serde(default)]
    trigger: Option<String>,
    name: Text,
    description: Text,
    scales: BTreeMap<String, f64>,
    #[serde(default)]
    range: Option<[f64; 2]>,
    /// The DiT width the weights were trained on: `2b` or `xl`.
    #[serde(default)]
    model: Option<String>,
    #[serde(default)]
    likes: u64,
    #[serde(default)]
    downloads: u64,
    files: Vec<CatalogFile>,
}
```

### CatalogFileFormat

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Deserialize)]
struct CatalogFileFormat {
    adapters: Vec<CatalogEntry>,
}
```

### Origin

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case", tag = "type")]
enum Origin {
    Catalog { catalog_id: String },
    Imported,
    /// A checkpoint of a training run.
    Trained { #[serde(default)] run: String, #[serde(default)] step: u32 },
    /// A file of a Hugging Face repository, pinned to the commit it came from.
    Hub { repo: String, revision: String, file: String },
}
```

### AdapterMeta

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct AdapterMeta {
    pub id: String,
    pub engine: String,
    pub name: Text,
    #[serde(default)]
    pub description: Text,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub trigger: Option<String>,
    #[serde(default)]
    pub author: Option<String>,
    #[serde(default)]
    pub page: Option<String>,
    /// The strength each slot starts at when the adapter is picked.
    #[serde(default)]
    pub scales: BTreeMap<String, f64>,
    /// The slider range, when the adapter is only meaningful inside one.
    #[serde(default)]
    pub range: Option<[f64; 2]>,
    /// The DiT width the weights fit, `2b` or `xl`, when it is known.
    #[serde(default)]
    pub model: Option<String>,
    /// The slots the engine found weights for, remembered from its last answer
    /// so the page can show them while the engine is not running.
    #[serde(default)]
    pub slots: Vec<String>,
    pub origin: Origin,
    #[serde(default)]
    pub created_at: String,
}
```

### Installed

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Installed {
    #[serde(flatten)]
    pub meta: AdapterMeta,
    pub bytes: u64,
    /// The engine's complaint about the files, when it has one.
    pub error: Option<String>,
}
```

### Offered

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Offered {
    pub id: String,
    pub name: Text,
    pub description: Text,
    pub kind: String,
    pub author: Option<String>,
    pub page: Option<String>,
    pub trigger: Option<String>,
    pub slots: Vec<String>,
    pub bytes: u64,
    pub installed: bool,
    pub model: Option<String>,
    pub likes: u64,
    pub downloads: u64,
}
```

### Patch

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize)]
struct Patch {
    pub name: Option<String>,
    pub trigger: Option<String>,
    pub scales: Option<BTreeMap<String, f64>>,
}
```

### HubConfig

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize)]
struct HubConfig {
    #[serde(default)]
    tags: Vec<String>,
    /// Further tag sets, searched besides `tags`.
    #[serde(default)]
    also: Vec<Vec<String>>,
    /// Words searched in repository names, for the many adapters published
    /// with no tag at all; what they find is kept only when it looks like an
    /// adapter and the typed words are in its name.
    #[serde(default)]
    search: Vec<String>,
}
```

### HubSection

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Deserialize)]
    struct HubSection {
        #[serde(default)]
        hub: BTreeMap<String, HubConfig>,
    }
```

### HubRepo

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct HubRepo {
    pub repo: String,
    pub author: String,
    pub likes: u64,
    pub downloads: u64,
    pub updated: Option<String>,
    pub tags: Vec<String>,
}
```

### HubFile

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct HubFile {
    pub path: String,
    pub bytes: u64,
    pub adapter_id: String,
    pub installed: bool,
    /// The decoder companion every render already merges, in some precision.
    pub built_in: bool,
    #[serde(flatten)]
    pub weights: AdapterWeights,
}
```

### HubListing

[crates/music-server/src/adapters.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/adapters.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct HubListing {
    pub repo: String,
    pub revision: String,
    pub page: String,
    pub files: Vec<HubFile>,
    /// Every file of the commit with its size, for the configs that travel along.
    #[serde(skip)]
    all: BTreeMap<String, u64>,
}
```

### AssistTarget

[crates/music-server/src/assistant.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
enum AssistTarget {
    /// Write the lyrics and the style together.
    All,
    /// Rewrite only the lyrics, coherent with the current style.
    Lyrics,
    /// Rewrite only the style, coherent with the current lyrics.
    Style,
    /// Edit the ABC score as the instruction asks.
    Score,
    /// Lay out a recording's recognised words as a lyric sheet.
    Transcript,
    /// Lay out a published lyric sheet in sections, its words untouched.
    Sheet,
}
```

### AssistRequest

[crates/music-server/src/assistant.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct AssistRequest {
    pub target: AssistTarget,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub instruction: String,
    #[serde(default)]
    pub lyrics: String,
    #[serde(default)]
    pub style: String,
    #[serde(default)]
    pub abc: String,
    #[serde(default = "default_duration")]
    pub duration_seconds: f64,
}
```

### AssistDraft

[crates/music-server/src/assistant.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant.rs)

```rust
#[derive(Debug, Clone, Default, Serialize)]
struct AssistDraft {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub lyrics: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub style: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub abc: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cover_prompt: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub duration_seconds: Option<u32>,
}
```

### AssetKind

[crates/music-server/src/assistant_runtime.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant_runtime.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum AssetKind {
    /// A GGUF text model.
    Model,
    /// The llama.cpp binaries, delivered as a zip.
    Runtime,
}
```

### AssetStatus

[crates/music-server/src/assistant_runtime.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant_runtime.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct AssetStatus {
    pub id: &'static str,
    pub label: &'static str,
    pub kind: AssetKind,
    pub bytes: u64,
    pub vram_gb: Option<u32>,
    pub note: &'static str,
    pub installed: bool,
    /// Bytes already on disk, so a resumed download reports honestly.
    pub downloaded_bytes: u64,
}
```

### RuntimeStatus

[crates/music-server/src/assistant_runtime.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant_runtime.rs)

```rust
#[derive(Debug, Clone, Default, Serialize)]
struct RuntimeStatus {
    pub root: String,
    /// True when llama-server and at least one model are present.
    pub ready: bool,
    pub server_path: Option<String>,
    pub installed_models: Vec<String>,
    pub running_model: Option<String>,
    pub base_url: Option<String>,
    pub assets: Vec<AssetStatus>,
    pub active_download: Option<DownloadProgress>,
}
```

### DownloadProgress

[crates/music-server/src/assistant_runtime.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/assistant_runtime.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct DownloadProgress {
    pub asset_id: String,
    pub downloaded_bytes: u64,
    pub total_bytes: u64,
    pub done: bool,
    pub error: Option<String>,
}
```

### Facts

[crates/music-server/src/audio_facts.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/audio_facts.rs)

```rust
#[derive(Clone, Debug, Serialize, PartialEq)]
struct Facts {
    /// Beats per minute, rounded.
    pub bpm: u32,
}
```

### CoverLook

[crates/music-server/src/cover_art.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/cover_art.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
struct CoverLook {
    /// The stock photo for the track; off, its pattern.
    #[serde(default = "on")]
    pub photo: bool,
    #[serde(default = "default_pattern")]
    pub pattern: String,
    /// Each track draws the pattern from its own words and style instead of its id.
    #[serde(default)]
    pub unique: bool,
    /// The placeholder is written into the track as its cover, and into an
    /// MP3's tags, so a player shows it after the file is saved.
    #[serde(default = "on")]
    pub keep: bool,
}
```

### Photo

[crates/music-server/src/cover_art.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/cover_art.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize)]
struct Photo {
    /// The file's name on Commons, without "File:".
    pub title: String,
    /// The file's page: where it came from and under what terms.
    pub page: String,
    /// The picture at the width a cover is kept at.
    pub image: String,
    /// The picture at the width a choice is shown at.
    pub preview: String,
    /// The picture as wide as a full-HD frame, or the original when narrower.
    pub large: String,
}
```

### Clip

[crates/music-server/src/cover_art.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/cover_art.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize)]
struct Clip {
    pub title: String,
    pub page: String,
    /// A still of the clip, for choosing it.
    pub poster: String,
    /// The clip as WebM, at a height a background needs.
    pub video: String,
    pub width: u64,
    pub height: u64,
}
```

### CoverTemplate

[crates/music-server/src/cover_prompt.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/cover_prompt.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
struct CoverTemplate {
    pub id: String,
    pub name: String,
    pub template: String,
}
```

### CredentialSource

[crates/music-server/src/credentials.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/credentials.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum CredentialSource {
    Environment,
    LocalStore,
}
```

### AssetKind

[crates/music-server/src/downloads.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/downloads.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum AssetKind {
    /// Model weights.
    Model,
    /// Executables and their libraries, delivered as a zip.
    Runtime,
}
```

### AssetStatus

[crates/music-server/src/downloads.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/downloads.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct AssetStatus {
    pub id: &'static str,
    pub label: &'static str,
    pub kind: AssetKind,
    pub bytes: u64,
    pub vram_gb: Option<u32>,
    pub note: &'static str,
    pub installed: bool,
    /// Bytes already on disk, so a resumed download reports honestly.
    pub downloaded_bytes: u64,
    /// The program (yue2, dub...) it was taken from in this run instead of being downloaded.
    pub taken_from: Option<String>,
}
```

### DownloadProgress

[crates/music-server/src/downloads.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/downloads.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct DownloadProgress {
    pub asset_id: String,
    /// Which panel asked for this. The karaoke recogniser and the separator
    /// share one downloader - they share the ONNX runtime and the CUDA
    /// libraries on disk, so they must - and without this the separator's panel
    /// showed the recogniser's download as its own: its button disappeared and
    /// a percentage of somebody else's gigabytes appeared in its place.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub scope: Option<String>,
    pub downloaded_bytes: u64,
    pub total_bytes: u64,
    pub done: bool,
    pub error: Option<String>,
}
```

### Taken

[crates/music-server/src/downloads.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/downloads.rs)

```rust
#[derive(Clone, Debug, Serialize)]
struct Taken {
    pub app: String,
    pub file: String,
}
```

### Hardware

[crates/music-server/src/hardware.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/hardware.rs)

```rust
#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct Hardware {
    pub gpu_name: Option<String>,
    pub total_vram_gb: f64,
    pub total_ram_gb: f64,
    /// An NVIDIA card: the engine runs on CUDA there, which needs cuBLAS.
    /// Every other card runs on Vulkan and needs nothing downloaded.
    pub nvidia: bool,
    /// The CUDA build of the engine this card and its driver run, none when
    /// neither does: such a card computes on Vulkan.
    pub cuda: Option<CudaBuild>,
    /// Whether the card and its driver run CUDA at all - what the ONNX Runtime's
    /// CUDA build needs - whichever build of the engine this platform ships.
    pub runs_cuda: bool,
    /// The CUDA build the card and its driver run, on every platform: what a trainer or a recogniser built for this
    /// machine needs, whatever the engine computes on.
    pub card_cuda: Option<CudaBuild>,
    /// The NVIDIA card's compute capability, `[7, 5]` for Turing.
    pub compute_capability: Option<(u32, u32)>,
    /// The model set id that fits this card, none when no set does.
    pub recommended: Option<&'static str>,
}
```

### CudaBuild

[crates/music-server/src/hardware.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/hardware.rs)

```rust
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
enum CudaBuild {
    Cuda12,
    Cuda13,
}
```

### Harmony

[crates/music-server/src/harmony.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/harmony.rs)

```rust
#[derive(Debug, Clone, Default, PartialEq, Deserialize, Serialize)]
struct Harmony {
    /// `root` counts C, Cmaj7 and C/E as one chord, `spelling` tells them apart.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub identity: Option<String>,
    /// Logits a chord loses for its share of the recent changes, 0 to 64.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub strength: Option<f64>,
    /// Recent chord changes remembered, 1 to 512.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub window: Option<u32>,
    /// Chord symbols in a row one root holds for free, 0 to 64; 0 is no limit.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hold_limit: Option<u32>,
    /// Root identity: logits a change to a root outside the key gains, 0 to 20.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub outside_bonus: Option<f64>,
    /// The share of recent changes outside the key that stops the bonus, 0 to 1.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub outside_limit: Option<f64>,
    /// Logits taken from a chord that would repeat how the previous section opened, 0 to 64.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub section_strength: Option<f64>,
    /// Opening chords compared between sections, 1 to 16.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub section_open: Option<u32>,
    /// Hold the plan to the lyrics' sections, in order.
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub follow_lyrics: bool,
}
```

### SamplingPreset

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize, Serialize, PartialEq)]
struct SamplingPreset {
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    top_p: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    top_k: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    repetition_penalty: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    penalty_window: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    min_tokens: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_tokens: Option<u32>,
}
```

### CreateMusicJobRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
struct CreateMusicJobRequest {
    /// The window's own mark for this request, handed back on the job so the
    /// window knows the job as its own before the response reaches it.
    client_ref: Option<String>,
    /// Comma-separated style tags, verbatim under `[Tags]`.
    #[serde(default)]
    style: String,
    /// Lyrics with their structural tags, verbatim under `[Lyrics]`.
    #[serde(default)]
    lyrics: String,
    /// ABC score to realise; empty lets the model write one.
    abc: Option<String>,
    /// Move a supplied score before singing; zero keeps its pitches.
    transpose: Option<i32>,
    /// Reveal each section's words as a supplied score reaches it; on unless false.
    #[serde(default)]
    lyric_timing: Option<bool>,
    #[serde(default)]
    vocals_only: bool,
    /// Seconds the studio fades the made song in and out over; none or 0 leaves it as sung.
    #[serde(default)]
    fade_in: Option<f32>,
    #[serde(default)]
    fade_out: Option<f32>,
    /// The playlist the made songs are added to.
    #[serde(default)]
    playlist_id: Option<String>,
    /// Chain-of-thought mode: `full`, `melody` or `off`.
    cot: Option<String>,
    /// Target length in seconds; the model may end the song earlier.
    duration_seconds: Option<f64>,
    lm_seed: Option<i64>,
    seed: Option<i64>,
    steps: Option<u32>,
    lm_batch_size: Option<u32>,
    synth_batch_size: Option<u32>,
    cfg_scale: Option<f64>,
    /// Strength of the engine's realaudio decoder companion; 0 decodes with the checkpoint alone.
    companion_scale: Option<f64>,
    /// Comma-separated semantic codes; present means the AR stage is skipped.
    semantic_tokens: Option<String>,
    abc_sampling: Option<SamplingPreset>,
    semantic_sampling: Option<SamplingPreset>,
    /// Chord variety and section order of the score the model plans.
    #[serde(default)]
    harmony: Option<harmony::Harmony>,
    /// `abc` is the opening of a score, and the model writes the rest of the song on from it.
    #[serde(default)]
    abc_continue: bool,
    /// The acoustic ODE solver: `midpoint` (the reference) or `ab2` (about half the evaluations).
    #[serde(default)]
    solver: Option<String>,
    output_format: Option<String>,
    mp3_bitrate: Option<u32>,
    /// Library title only, never sent to the engine.
    title: Option<String>,
    /// What the cover should show, when the assistant described it.
    cover_prompt: Option<String>,
    /// Installed adapters to merge for this song, in order.
    #[serde(default)]
    adapters: Vec<AdapterUse>,
    /// The library song whose melody this song sings (a cover); the new song
    /// names it as the track it was made from.
    #[serde(default)]
    cover_of: Option<String>,
}
```

### AdapterUse

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
struct AdapterUse {
    id: String,
    #[serde(default)]
    scales: std::collections::BTreeMap<String, f64>,
}
```

### MusicJobStatus

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
enum MusicJobStatus {
    Queued,
    Running,
    Completed,
    Failed,
    Cancelled,
}
```

### MusicJobDispatch

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
enum MusicJobDispatch {
    NotConfigured,
    Local,
    Cancelled,
}
```

### MusicJobPhase

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
enum MusicJobPhase {
    Queued,
    Running,
    ExtractingVocals,
    Completed,
    Failed,
    Cancelled,
}
```

### MusicJob

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct MusicJob {
    /// Where a track made from another comes from (a re-render).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    derived: Option<Value>,
    id: String,
    /// The mark the submitting window gave the request; absent for an agent's.
    #[serde(skip_serializing_if = "Option::is_none")]
    client_ref: Option<String>,
    /// When the request came in, in Unix milliseconds: a window opened later
    /// places the job among the ones it made itself.
    submitted_at: u64,
    engine_id: String,
    /// What the assistant said this track's cover should show, if anything.
    #[serde(skip_serializing_if = "Option::is_none")]
    cover_prompt: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    title: Option<String>,
    status: MusicJobStatus,
    dispatch: MusicJobDispatch,
    phase: MusicJobPhase,
    style: String,
    lyrics: String,
    duration_seconds: f64,
    generation_settings: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    song: Option<CompletedSong>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    songs: Vec<CompletedSong>,
    message: String,
    /// The playlist the made songs go into, a project the user works in.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    playlist_id: Option<String>,
    /// How the lyrics were laid along a score that came without sections.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    laid: Option<Value>,
}
```

### CompletedSong

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct CompletedSong {
    id: String,
    song: library::Song,
    audio_url: String,
}
```

### LocalMusicModelCatalog

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize)]
struct LocalMusicModelCatalog {
    engine_id: String,
    catalog: Value,
}
```

### CapabilitiesResponse

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize)]
struct CapabilitiesResponse {
    engines: Vec<EngineDescriptor>,
}
```

### ApiError

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize)]
struct ApiError {
    error: String,
}
```

### EngineSubmitResponse

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct EngineSubmitResponse {
    id: String,
}
```

### EngineJobResponse

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct EngineJobResponse {
    status: String,
}
```

### SetupSelectRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct SetupSelectRequest {
    #[serde(default)]
    profile_id: Option<String>,
    #[serde(default)]
    component_ids: Option<Vec<String>>,
}
```

### SetupDownloadRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct SetupDownloadRequest {
    // The panel calls this field `component_ids`. Reading only `ids` meant a
    // download request arrived empty, and an empty request quietly fell back to
    // the default set - which is how pressing "download" on the 11.9 GB set
    // started fetching the 26.6 GB one.
    #[serde(default, alias = "component_ids")]
    ids: Vec<String>,
    profile_id: Option<String>,
}
```

### ReplayMusicJobRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Deserialize, Serialize)]
struct ReplayMusicJobRequest {
    song_id: Option<String>,
    replay_request: Option<Value>,
    steps: Option<u32>,
    /// The acoustic ODE solver: `midpoint` or `ab2`.
    #[serde(default)]
    solver: Option<String>,
    seed: Option<i64>,
    synth_batch_size: Option<u32>,
    output_format: Option<String>,
    mp3_bitrate: Option<u32>,
    /// A title for the re-render; the source track's own name otherwise.
    title: Option<String>,
    /// Seconds to compose on after the track's last frame; its start stays as it is.
    extend_seconds: Option<f64>,
    /// The window's own mark, as on a new song.
    client_ref: Option<String>,
}
```

### PersistedStudioSettings

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize, Deserialize)]
struct PersistedStudioSettings {
    #[serde(default)]
    engine_options: EngineOptions,
    #[serde(default)]
    assistant: AssistantConfig,
    lyrics_sync: lyrics_sync::LyricsSyncConfig,
    configuration: StudioConfiguration,
    selected_profile_id: Option<String>,
    #[serde(default)]
    selected_component_ids: Option<Vec<String>>,
    #[serde(default)]
    cover_templates: Option<Vec<cover_prompt::CoverTemplate>>,
    #[serde(default)]
    cover_template_default: Option<String>,
    #[serde(default)]
    separation: Option<separation::SeparationConfig>,
    /// Whether a finished track gets its cover drawn without being asked.
    #[serde(default)]
    cover_auto: Option<bool>,
    #[serde(default)]
    cover_look: Option<cover_art::CoverLook>,
    #[serde(default)]
    file_tags_settings: Option<bool>,
    #[serde(default)]
    proxy: Option<net::ProxySettings>,
    /// Access from other computers, off unless turned on.
    #[serde(default)]
    network: Option<remote::NetworkAccess>,
}
```

### OpenRouterTranscriptionRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct OpenRouterTranscriptionRequest {
    model_id: String,
    audio_base64: String,
    audio_format: String,
    language: Option<String>,
}
```

### EngineOptions

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct EngineOptions {
    backend: music_engine::yue_server::ComputeBackend,
    keep_loaded: bool,
    max_batch: Option<u32>,
    max_seq: Option<u32>,
    vae_core: Option<u32>,
    vae_halo: Option<u32>,
    disable_flash_attention: bool,
    clamp_fp16: bool,
    /// The NVIDIA card, by its nvidia-smi index, that every CUDA process of
    /// the studio computes on. None: the first. Taken at the studio's start.
    gpu: Option<u32>,
    /// Every other launch flag, as the line it was typed on. It reaches the
    /// engine last, so it can also override one of the flags above - and it
    /// restarts the engine like they do, because the engine reads it once at
    /// startup.
    extra_args: String,
    /// The engine executable when it is not the one the studio ships: a build
    /// of yue2.cpp compiled by hand. None is the bundled engine, and
    /// `YUE_ENGINE_BIN` still wins over whatever is saved here, so a developer
    /// build needs no settings at all.
    engine_bin: Option<String>,
}
```

### AssistantConfig

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct AssistantConfig {
    provider: AssistantProvider,
    /// Base URL of an OpenAI-compatible server (llama.cpp, LM Studio, Ollama).
    local_base_url: Option<String>,
    local_model: Option<String>,
    openrouter_model: Option<String>,
    /// Id of a model downloaded through the assistant runtime, run as a
    /// sidecar by Studio itself.
    managed_model: Option<String>,
    /// A GGUF already on this machine, run by the same sidecar. Machines that
    /// already keep a Gemma around for another tool do not need a second copy.
    managed_path: Option<String>,
    /// How hard a reasoning model should think, in OpenRouter's own terms:
    /// minimal, low, medium, high, xhigh, max - or none.
    reasoning_effort: Option<String>,
    /// Which agent CLI answers; empty means the first one detected.
    cli_agent: Option<String>,
    /// Model override for that CLI; empty means its configured model.
    cli_model: Option<String>,
    /// Explicit executable for that CLI; empty means PATH.
    cli_path: Option<String>,
}
```

### AssistantProvider

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum AssistantProvider {
    #[default]
    None,
    Local,
    OpenRouter,
    /// A model Studio downloaded and runs itself with llama.cpp.
    Managed,
    /// The agent connected over MCP: the studio asks it what it would ask its
    /// own model, with the same instructions and answer schema.
    Agent,
    /// An agent CLI on this machine, run headless for one draft.
    Cli,
}
```

### ProxyImageRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ProxyImageRequest {
    url: String,
}
```

### OpenRouterSettingsRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct OpenRouterSettingsRequest {
    /// `None` or an empty string clears the locally stored credential.
    api_key: Option<String>,
}
```

### OpenRouterCompletionRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct OpenRouterCompletionRequest {
    model_id: String,
    prompt: String,
}
```

### OpenRouterCoverRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct OpenRouterCoverRequest {
    model_id: String,
    prompt: String,
}
```

### OpenRouterResponse

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize)]
struct OpenRouterResponse {
    body: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    generation_id: Option<String>,
}
```

### LikeInput

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Deserialize)]
struct LikeInput {
    liked: bool,
}
```

### NoteInput

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Deserialize)]
struct NoteInput {
    note: String,
}
```

### StoreCoverRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct StoreCoverRequest {
    /// Raw base64 image bytes, without a data-URL prefix.
    image_base64: String,
    media_type: String,
}
```

### CoverTemplatesRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct CoverTemplatesRequest {
    templates: Vec<cover_prompt::CoverTemplate>,
    /// Draw a cover as soon as a track finishes.
    #[serde(default)]
    auto: Option<bool>,
    /// Which of them a new cover starts from. `None` leaves it as it was.
    #[serde(default)]
    default_id: Option<String>,
}
```

### RenderCoverPromptRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct RenderCoverPromptRequest {
    template: String,
    /// The track the prompt is for. Without it the placeholders have nothing
    /// to stand in for, which is only useful for previewing the wording.
    song_id: Option<String>,
    #[serde(default)]
    title: Option<String>,
    #[serde(default)]
    style: Option<String>,
    #[serde(default)]
    lyrics: Option<String>,
}
```

### SeparationRun

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct SeparationRun {
    song_id: String,
    /// Between 0 and 1.
    progress: f64,
    done: bool,
    error: Option<String>,
    stems: Vec<String>,
    /// Whether the graphics card did the work, once the run is over.
    used_gpu: Option<bool>,
    /// The library tracks the stems became.
    library_songs: Vec<String>,
}
```

### InstallSeparationAssetRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct InstallSeparationAssetRequest {
    asset_id: String,
}
```

### InstallAdaptersRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct InstallAdaptersRequest {
    /// Catalogue entries to fetch as one download.
    ids: Vec<String>,
}
```

### HubQuery

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct HubQuery {
    #[serde(default)]
    q: String,
    #[serde(default)]
    repo: String,
}
```

### InstallHubRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct InstallHubRequest {
    repo: String,
    paths: Vec<String>,
}
```

### SyllablesRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct SyllablesRequest {
    lyrics: String,
}
```

### VstEditorRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct VstEditorRequest {
    path: String,
    #[serde(default)]
    state_id: Option<String>,
}
```

### KeepProcessingRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct KeepProcessingRequest {
    /// What the processing is called on the new track; its stages when left out.
    #[serde(default)]
    label: String,
}
```

### SelectVersionRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct SelectVersionRequest {
    /// `original`, or the id of one of the track's versions.
    version: String,
}
```

### DatasetInput

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct DatasetInput {
    #[serde(default)]
    name: Option<String>,
    #[serde(default)]
    trigger: Option<String>,
}
```

### DatasetSongs

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct DatasetSongs {
    song_ids: Vec<String>,
}
```

### StartTraining

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct StartTraining {
    dataset_id: String,
    #[serde(default)]
    name: String,
    recipe: training::Recipe,
}
```

### ContinueTraining

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Deserialize)]
struct ContinueTraining {
    /// The steps the run is to reach in all.
    steps: u32,
}
```

### InstallCheckpoint

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct InstallCheckpoint {
    #[serde(default)]
    name: Option<String>,
}
```

### Activity

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Activity {
    song_id: String,
    title: String,
    /// "cover" or "karaoke".
    kind: &'static str,
    /// "running", "done" or "failed".
    state: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    detail: Option<String>,
}
```

### FileTagsRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Deserialize)]
struct FileTagsRequest {
    settings: bool,
}
```

### CoverLookRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct CoverLookRequest {
    #[serde(default)]
    photo: Option<bool>,
    #[serde(default)]
    pattern: Option<String>,
    #[serde(default)]
    unique: Option<bool>,
    #[serde(default)]
    keep: Option<bool>,
}
```

### MediaSearch

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct MediaSearch {
    #[serde(default)]
    q: String,
    #[serde(default)]
    from: usize,
}
```

### MediaStyle

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct MediaStyle {
    #[serde(default)]
    style: String,
}
```

### ChosenCoverPhoto

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ChosenCoverPhoto {
    image: String,
    #[serde(default)]
    page: Option<String>,
}
```

### ChosenCoverPattern

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ChosenCoverPattern {
    pattern: String,
    seed: String,
}
```

### AssistantSettingsRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct AssistantSettingsRequest {
    provider: Option<AssistantProvider>,
    local_base_url: Option<Option<String>>,
    local_model: Option<Option<String>>,
    openrouter_model: Option<Option<String>>,
    managed_model: Option<Option<String>>,
    managed_path: Option<Option<String>>,
    reasoning_effort: Option<Option<String>>,
    cli_agent: Option<Option<String>>,
    cli_model: Option<Option<String>>,
    cli_path: Option<Option<String>>,
}
```

### LocalModelsQuery

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct LocalModelsQuery {
    base: String,
}
```

### LocalServerKeyRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct LocalServerKeyRequest {
    api_key: Option<String>,
}
```

### AssistantAssetRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct AssistantAssetRequest {
    asset_id: String,
    /// The model to install along with the runtime, when the capability has a
    /// choice of them. A runtime with no model does nothing.
    #[serde(default)]
    model_id: Option<String>,
}
```

### AssistantModelRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct AssistantModelRequest {
    #[serde(default)]
    model_id: String,
    /// A GGUF already on this machine, used instead of a downloaded one.
    #[serde(default)]
    model_path: Option<String>,
}
```

### KaraokeRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct KaraokeRequest {
    /// Overrides the language guess for this one track.
    #[serde(default)]
    language: Option<String>,
}
```

### KaraokeSettingsRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct KaraokeSettingsRequest {
    enabled: Option<bool>,
    provider: Option<lyrics_sync::AsrProvider>,
    whisper_model: Option<Option<String>>,
    openrouter_model: Option<Option<String>>,
    runtime: Option<lyrics_sync::OnnxFlavour>,
}
```

### LyricsSectionsRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Deserialize)]
struct LyricsSectionsRequest {
    lyrics: String,
}
```

### MidiSizeRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct MidiSizeRequest {
    /// `model_id` when the models page sends it, as for every optional part.
    #[serde(default, alias = "model_id")]
    size: Option<String>,
}
```

### MidiRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct MidiRequest {
    /// A library track: its MIDI is kept beside it.
    #[serde(default)]
    song_id: Option<String>,
    /// Or any audio file on this computer: its MIDI goes to the studio's folder.
    #[serde(default)]
    path: Option<String>,
    /// Or several library tracks, one after another.
    #[serde(default)]
    song_ids: Option<Vec<String>>,
    #[serde(default)]
    size: Option<String>,
}
```

### MidiUpload

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Deserialize)]
struct MidiUpload {
    data: String,
}
```

### GuideQuery

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct GuideQuery {
    #[serde(default)]
    topic: String,
}
```

### ExamplesQuery

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ExamplesQuery {
    #[serde(default)]
    brief: String,
}
```

### VideoName

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct VideoName {
    name: String,
}
```

### FindLyricsRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct FindLyricsRequest {
    #[serde(default)]
    artist: String,
    title: String,
    #[serde(default)]
    seconds: f64,
}
```

### ComfyExportQuery

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ComfyExportQuery {
    /// Strength of the planner half, folded into the file; 1 when left out.
    ar: Option<f32>,
    /// Strength of the sound half.
    nar: Option<f32>,
}
```

### ComfySaveRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ComfySaveRequest {
    /// Where the file goes, a full path ending in .safetensors.
    path: String,
    ar: Option<f32>,
    nar: Option<f32>,
}
```

### ScoreJob

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize)]
struct ScoreJob {
    id: String,
    status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    abc: Option<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    plans: Vec<ScorePlan>,
    /// The token seed a composition drew its score with.
    #[serde(skip_serializing_if = "Option::is_none")]
    lm_seed: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}
```

### ScorePlan

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Serialize)]
struct ScorePlan { abc: String, lm_seed: Option<i64> }
```

### ComposeScoreRequest

[crates/music-server/src/lib.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lib.rs)

```rust
#[derive(Debug, Deserialize)]
struct ComposeScoreRequest {
    #[serde(default)]
    style: String,
    #[serde(default)]
    lyrics: String,
    #[serde(default)]
    cot: Option<String>,
    #[serde(default)]
    lm_seed: Option<i64>,
    #[serde(default)]
    abc_sampling: Option<SamplingPreset>,
    #[serde(default)]
    harmony: Option<harmony::Harmony>,
    /// The opening of a score the plan goes on from.
    #[serde(default)]
    abc: Option<String>,
    #[serde(default)]
    lm_batch_size: Option<u32>,
}
```

### Song

[crates/music-server/src/library.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/library.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct Song { pub id:String, pub title:String, pub audio_path:Option<String>, pub caption:String, pub lyrics:String, pub metadata:serde_json::Value, pub generation_settings:serde_json::Value, pub engine_id:String, pub profile_id:Option<String>, pub replay_request:Option<serde_json::Value>, pub audio_codes:Option<serde_json::Value>, pub source:String, pub created_at:String, pub updated_at:String }
```

### SongInput

[crates/music-server/src/library.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/library.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct SongInput { pub title:String, pub audio_path:Option<String>, #[serde(default)] pub caption:String, #[serde(default)] pub lyrics:String, #[serde(default)] pub metadata:serde_json::Value, #[serde(default)] pub generation_settings:serde_json::Value, pub engine_id:String, pub profile_id:Option<String>, pub replay_request:Option<serde_json::Value>, pub audio_codes:Option<serde_json::Value>, #[serde(default="manual_source")] pub source:String }
```

### ImportedSong

[crates/music-server/src/library.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/library.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct ImportedSong { pub song: Song, pub audio_filename: String }
```

### Playlist

[crates/music-server/src/library.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/library.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)] struct Playlist { pub id:String, pub name:String, pub description:Option<String>, pub song_ids:Vec<String>, pub created_at:String, pub updated_at:String }
```

### JournalEntry

[crates/music-server/src/library.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/library.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)] struct JournalEntry { #[serde(default)] pub id:i64, #[serde(default)] pub at:String, pub source:String, #[serde(default)] pub tone:String, #[serde(default)] pub verb:String, #[serde(default)] pub kind:String, #[serde(default)] pub target:String, #[serde(default)] pub text:String }
```

### PlaylistInput

[crates/music-server/src/library.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/library.rs)

```rust
#[derive(Debug, Clone, Deserialize)] struct PlaylistInput { pub name:String, pub description:Option<String>, #[serde(default)] pub song_ids:Vec<String> }
```

### AsrProvider

[crates/music-server/src/lyrics_sync.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lyrics_sync.rs)

```rust
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum AsrProvider {
    #[default]
    None,
    Whisper,
    Parakeet,
    OpenRouter,
}
```

### LyricsSyncConfig

[crates/music-server/src/lyrics_sync.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lyrics_sync.rs)

```rust
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct LyricsSyncConfig {
    /// The karaoke switch. Off means the buttons do not appear at all.
    pub enabled: bool,
    pub provider: AsrProvider,
    /// Which downloaded Whisper model to run.
    pub whisper_model: Option<String>,
    /// Which OpenRouter speech-to-text model to call.
    pub openrouter_model: Option<String>,
    /// What the local recogniser runs on. It decides which runtime is
    /// downloaded as much as which one is loaded, so it belongs to the setting
    /// rather than to a guess made at load time.
    #[serde(default)]
    pub runtime: OnnxFlavour,
}
```

### OnnxFlavour

[crates/music-server/src/lyrics_sync.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lyrics_sync.rs)

```rust
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum OnnxFlavour {
    /// The graphics card if its runtime is installed, otherwise the processor.
    Auto,
    /// The default: this studio exists for machines with an NVIDIA card, and
    /// the processor path is minutes where the card is seconds.
    #[default]
    Cuda,
    Cpu,
}
```

### OnnxCard

[crates/music-server/src/lyrics_sync.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/lyrics_sync.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum OnnxCard {
    Cuda,
    DirectMl,
}
```

### WindowAnswer

[crates/music-server/src/mcp.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs)

```rust
#[derive(serde::Deserialize)]
struct WindowAnswer {
    id: String,
    #[serde(default)]
    result: Value,
    #[serde(default)]
    error: Option<String>,
}
```

### WindowFocus

[crates/music-server/src/mcp.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/mcp.rs)

```rust
#[derive(serde::Deserialize)]
struct WindowFocus {
    window: u64,
}
```

### Source

[crates/music-server/src/midi.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/midi.rs)

```rust
#[derive(Deserialize)]
struct Source {
    commit: String,
    shipped_as: String,
    release_tag: String,
    asset: String,
    bytes: u64,
}
```

### Size

[crates/music-server/src/midi.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/midi.rs)

```rust
#[derive(Debug, Clone, Copy, Serialize)]
struct Size {
    pub id: &'static str,
    pub params: &'static str,
    /// The mirror's revision the files were measured at.
    #[serde(skip)]
    revision: &'static str,
    #[serde(skip)]
    config_bytes: u64,
    pub bytes: u64,
}
```

### Note

[crates/music-server/src/midi.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/midi.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
struct Note {
    pub pitch: u8,
    pub start: f64,
    pub end: f64,
    pub instrument: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub velocity: Option<u8>,
}
```

### Run

[crates/music-server/src/midi.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/midi.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Run {
    /// The library track, or none for a file named by its path.
    pub song_id: Option<String>,
    pub title: String,
    pub size: &'static str,
    /// preparing, downloading, reading, transcribing, done.
    pub stage: &'static str,
    pub chunks_done: u32,
    pub chunks_total: u32,
    pub notes: usize,
    pub done: bool,
    pub error: Option<String>,
    /// The MIDI file on this computer, once written.
    pub file: Option<String>,
    /// The set this track belongs to, when several were asked for at once.
    pub batch: Option<Batch>,
}
```

### Batch

[crates/music-server/src/midi.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/midi.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Batch {
    /// How many of the set are finished, made or failed.
    pub done: usize,
    pub total: usize,
    /// The tracks still waiting, in order.
    pub queued: Vec<String>,
    /// Each track that failed, by its title, with the reason.
    pub failed: Vec<String>,
}
```

### Sidecar

[crates/music-server/src/midi.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/midi.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct Sidecar {
    pub size: String,
    pub made_at: String,
    pub instruments: Vec<String>,
    pub notes: Vec<Note>,
}
```

### Catalog

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Catalog {
    pub engine_id: &'static str,
    pub repository: &'static str,
    pub revision: &'static str,
    pub recommended_profile_id: &'static str,
    pub profiles: Vec<Profile>,
    pub components: Vec<Component>,
}
```

### Profile

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Profile {
    pub id: &'static str,
    pub label: &'static str,
    pub backend: &'static str,
    pub installable: bool,
    pub recommended: bool,
    pub components: Vec<&'static str>,
    pub total_bytes: u64,
    /// Memory this machine needs for the set: less when its card holds the weights.
    pub ram_needed_gb: f64,
}
```

### Component

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Component {
    pub id: &'static str,
    pub kind: &'static str,
    pub filename: &'static str,
    pub bytes: u64,
    pub sha256: &'static str,
    /// Where this file comes from. The lighter quantisations are published by
    /// someone else, and a single hard-coded repository is what kept them out.
    pub repository: &'static str,
    pub revision: &'static str,
}
```

### InstallRequest

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct InstallRequest {
    pub profile_id: Option<String>,
    #[serde(default)]
    pub component_ids: Vec<String>,
}
```

### ManagerStatus

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct ManagerStatus {
    pub engine_id: &'static str,
    pub model_root: String,
    pub first_run: bool,
    pub ready: bool,
    pub download_pending: u64,
    pub recommended_profile_id: String,
    pub active: Option<DownloadJob>,
    pub components: Vec<ComponentStatus>,
    pub installed_components: Vec<String>,
    /// The files the selected set resolves to, named rather than implied.
    pub profile_files: Option<ProfileModelFiles>,
}
```

### ComponentStatus

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct ComponentStatus {
    pub id: &'static str,
    pub installed: bool,
    pub bytes: u64,
}
```

### DownloadJob

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct DownloadJob {
    pub id: String,
    pub profile_id: Option<String>,
    pub component_ids: Vec<String>,
    pub status: DownloadStatus,
    pub downloaded_bytes: u64,
    pub total_bytes: u64,
    pub error: Option<String>,
}
```

### ProfileModelFiles

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Serialize, Debug, Clone, PartialEq, Eq)]
struct ProfileModelFiles {
    pub backbone: String,
    pub vae: String,
    /// SheetSage2 is optional: without it the studio generates but cannot
    /// read a recording into a score.
    pub transcriber: Option<String>,
    /// The decoder adapter paired with the tokenizer head of the training codes.
    pub companion: String,
}
```

### DownloadStatus

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum DownloadStatus {
    Downloading,
    Completed,
    Cancelled,
    Failed,
}
```

### PersistentState

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
struct PersistentState {
    active: Option<DownloadJob>,
}
```

### RemovalReport

[crates/music-server/src/model_manager.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/model_manager.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct RemovalReport {
    pub removed: Vec<String>,
    pub freed_bytes: u64,
}
```

### ProxyMode

[crates/music-server/src/net.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/net.rs)

```rust
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum ProxyMode {
    /// Straight to the internet, whatever Windows or the environment say.
    Off,
    /// As Windows is set (Settings - Network - Proxy), or HTTP_PROXY,
    /// HTTPS_PROXY and ALL_PROXY when those are given.
    #[default]
    System,
    /// The address the user gave.
    Custom,
}
```

### ProxyKind

[crates/music-server/src/net.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/net.rs)

```rust
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum ProxyKind {
    #[default]
    Http,
    Https,
    /// Names resolved by the proxy (socks5h): the lookups do not leave
    /// through the user's own connection, and a name blocked there resolves.
    Socks5,
    Socks4,
}
```

### ProxySettings

[crates/music-server/src/net.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/net.rs)

```rust
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
struct ProxySettings {
    pub mode: ProxyMode,
    /// As the user wrote it.
    pub address: Option<String>,
    pub kind: ProxyKind,
    /// Public Hugging Face downloads through HF-Mirror (hf-mirror.com).
    pub huggingface_mirror: bool,
}
```

### Fill

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Copy, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
enum Fill {
    /// Not at all.
    None,
    /// Only where the field is empty; what the user wrote stays.
    #[default]
    Missing,
    /// Every chosen song, written over.
    All,
}
```

### TrainAfter

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Deserialize, Serialize)]
struct TrainAfter {
    #[serde(default)]
    pub name: String,
    pub recipe: training::Recipe,
}
```

### PrepareRequest

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Deserialize)]
struct PrepareRequest {
    /// The songs to prepare; all of the dataset's when absent.
    #[serde(default)]
    pub items: Option<Vec<String>>,
    #[serde(default)]
    pub lyrics: Fill,
    #[serde(default)]
    pub style: Fill,
    /// The language sung, when the user knows it.
    #[serde(default)]
    pub language: Option<String>,
    /// Start this run once every song is ready.
    #[serde(default)]
    pub train: Option<TrainAfter>,
    /// Who lays the lyrics out and writes the styles: the studio's assistant,
    /// or an agent connected over MCP, which finds the songs left `found` and
    /// `heard` with everything it needs and writes them itself.
    #[serde(default)]
    pub writer: Writer,
}
```

### Writer

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Copy, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
enum Writer {
    #[default]
    Studio,
    Agent,
}
```

### Failure

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Failure {
    pub item: String,
    pub step: &'static str,
    pub error: String,
}
```

### Job

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Deserialize, Serialize)]
struct Job {
    dataset: String,
    items: Option<Vec<String>>,
    lyrics: bool,
    style: bool,
    language: Option<String>,
    train: Option<TrainAfter>,
    #[serde(default)]
    writer: Writer,
}
```

### PrepareStatus

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct PrepareStatus {
    pub dataset: String,
    /// The stages at work now, the latest last.
    pub stages: Vec<StageStatus>,
    /// Songs still to be touched by any stage.
    pub pending: Vec<String>,
    pub failures: Vec<Failure>,
    /// Why a whole step was left out, as a message key the page translates.
    pub notices: Vec<&'static str>,
    pub finished: bool,
    pub cancelled: bool,
    /// The run started after, when one was asked for.
    pub run: Option<String>,
    /// What the captioner said about the device it ran on.
    pub device: Option<String>,
    /// A run starts once every song is ready.
    pub train_after: bool,
}
```

### StageStatus

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct StageStatus {
    pub name: &'static str,
    pub done: usize,
    pub total: usize,
    /// The song the stage works on now.
    pub current: Option<String>,
    /// The songs of the stage, in the order it takes them.
    #[serde(skip)]
    order: Vec<String>,
}
```

### TrainAfterRequest

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Deserialize)]
struct TrainAfterRequest {
    #[serde(default)]
    pub train: Option<TrainAfter>,
}
```

### TakeAsIsRequest

[crates/music-server/src/prepare.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/prepare.rs)

```rust
#[derive(Debug, Default, Deserialize)]
struct TakeAsIsRequest {
    #[serde(default)]
    pub items: Option<Vec<String>>,
}
```

### ProcessRequest

[crates/music-server/src/processing.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/processing.rs)

```rust
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
struct ProcessRequest {
    #[serde(default)]
    pub denoise: Option<denoise::DenoiseSettings>,
    #[serde(default)]
    pub lifter: Option<lifter::LifterSettings>,
    #[serde(default)]
    pub naturalize: Option<naturalize::NaturalizeSettings>,
    /// VST3 plugins, run in order before mastering.
    #[serde(default)]
    pub vst: Option<Vec<crate::vst::VstSlot>>,
    #[serde(default)]
    pub master: Option<MasterSource>,
    #[serde(default)]
    pub normalize: Option<NormalizeSettings>,
}
```

### NormalizeSettings

[crates/music-server/src/processing.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/processing.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
struct NormalizeSettings {
    #[serde(default = "default_peak_clip")]
    pub peak_clip: u32,
}
```

### MasterSource

[crates/music-server/src/processing.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/processing.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
enum MasterSource {
    Song { song_id: String },
    Upload { upload_id: String },
}
```

### ProcessRun

[crates/music-server/src/processing.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/processing.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct ProcessRun {
    /// Tells a finishing worker whether its run is still the current one.
    pub id: String,
    pub song_id: String,
    pub stages: Vec<&'static str>,
    /// The stage working now, once started.
    pub stage: Option<&'static str>,
    pub done: bool,
    pub error: Option<String>,
    /// The preview's file name inside the processing folder, when ready.
    #[serde(skip)]
    pub preview: Option<String>,
    pub preview_ready: bool,
    pub request: ProcessRequest,
}
```

### Stage

[crates/music-server/src/progress.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/progress.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum Stage {
    /// The AR half writing the ABC score.
    Score,
    /// The AR half writing the semantic codes.
    Semantic,
    /// The NAR half solving the acoustic latents.
    Acoustic,
    /// The VAE turning latents into audio.
    Decode,
    /// SheetSage2 reading a recording into a score.
    Transcribe,
}
```

### Progress

[crates/music-server/src/progress.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/progress.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize)]
struct Progress {
    pub stage: Stage,
    /// Overall fraction of the job, 0 to 1.
    pub fraction: f64,
    /// The stage's own counter, e.g. `412/750`.
    pub detail: String,
}
```

### Detected

[crates/music-server/src/providers/cli_agent.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/cli_agent.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct Detected {
    pub id: &'static str,
    pub display_name: &'static str,
    pub path: String,
    pub version: String,
    pub default_model: Option<String>,
    /// Version-dependent abilities of this binary.
    pub features: Vec<String>,
}
```

### AgentInfo

[crates/music-server/src/providers/cli_agent.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/cli_agent.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct AgentInfo {
    pub id: &'static str,
    pub display_name: &'static str,
    pub installed: bool,
    pub path: Option<String>,
    pub version: Option<String>,
    pub default_model: Option<String>,
    pub features: Vec<String>,
    pub suggested_models: &'static [&'static str],
    pub install: &'static str,
    pub signin: &'static str,
    pub docs: Option<&'static str>,
}
```

### OpenRouterRequest

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize)]
struct OpenRouterRequest {
    pub method: HttpMethod,
    pub path: &'static str,
    pub body: Value,
}
```

### HttpMethod

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "UPPERCASE")]
enum HttpMethod {
    Post,
}
```

### ModelsResponse

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct ModelsResponse {
    #[serde(default)]
    pub data: Vec<RemoteModel>,
}
```

### ModelDefaults

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize, Serialize, PartialEq)]
#[serde(default)]
struct ModelDefaults {
    pub temperature: Option<f64>,
    pub top_p: Option<f64>,
    pub top_k: Option<i64>,
    pub frequency_penalty: Option<f64>,
    pub presence_penalty: Option<f64>,
    pub repetition_penalty: Option<f64>,
}
```

### RemoteModel

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Deserialize)]
struct RemoteModel {
    pub id: String,
    pub name: String,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub context_length: Option<u64>,
    #[serde(default)]
    pub supported_parameters: Vec<String>,
    #[serde(default)]
    pub pricing: Option<ModelPricing>,
    #[serde(default)]
    pub architecture: ModelArchitecture,
    #[serde(default)]
    pub default_parameters: ModelDefaults,
    /// What the model says about thinking, in its own terms. Not part of
    /// `default_parameters`: it is a field of its own, and ignoring it is how a
    /// request asks for an effort the model does not take.
    #[serde(default)]
    pub reasoning: Option<ReasoningSupport>,
}
```

### ReasoningSupport

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize, Serialize, PartialEq)]
#[serde(default)]
struct ReasoningSupport {
    /// What the model uses when nothing is asked for.
    pub default_effort: Option<String>,
    /// Whether it thinks unless told otherwise.
    pub default_enabled: Option<bool>,
    /// Whether it can be told otherwise at all.
    pub mandatory: bool,
    /// The only values it accepts.
    pub supported_efforts: Vec<String>,
}
```

### ModelPricing

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
struct ModelPricing {
    #[serde(default)]
    pub prompt: Option<String>,
    #[serde(default)]
    pub completion: Option<String>,
    #[serde(default)]
    pub image: Option<String>,
    #[serde(default)]
    pub request: Option<String>,
}
```

### ModelArchitecture

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize)]
struct ModelArchitecture {
    #[serde(default)]
    pub modality: Option<String>,
    #[serde(default)]
    pub input_modalities: Vec<String>,
    #[serde(default)]
    pub output_modalities: Vec<String>,
}
```

### CatalogModel

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Deserialize, Debug, Clone, Serialize)]
struct CatalogModel {
    /// What this model says its own sampling should be.
    #[serde(default)]
    pub defaults: ModelDefaults,
    pub id: String,
    pub name: String,
    pub description: Option<String>,
    pub context_length: Option<u64>,
    pub modality: Option<String>,
    pub input_modalities: Vec<String>,
    pub output_modalities: Vec<String>,
    pub supported_parameters: Vec<String>,
    pub pricing: Option<ModelPricing>,
    pub capabilities: Vec<Capability>,
    /// What this model will accept as thinking, in its own words.
    pub reasoning: Option<ReasoningSupport>,
}
```

### CapabilityCatalog

[crates/music-server/src/providers/openrouter.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/providers/openrouter.rs)

```rust
#[derive(Deserialize, Debug, Clone, Serialize)]
struct CapabilityCatalog {
    pub models: Vec<CatalogModel>,
}
```

### NetworkAccess

[crates/music-server/src/remote.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/remote.rs)

```rust
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
struct NetworkAccess {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub key: String,
}
```

### AccessChange

[crates/music-server/src/remote.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/remote.rs)

```rust
#[derive(Debug, Deserialize)]
struct AccessChange {
    #[serde(default)]
    enabled: Option<bool>,
    /// A new key replaces the old one; every browser signed in with it is out.
    #[serde(default)]
    new_key: bool,
}
```

### GpuSnapshot

[crates/music-server/src/resources.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/resources.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct GpuSnapshot {
    pub name: String,
    pub vram_used_mb: u64,
    pub vram_total_mb: u64,
    /// Percent of the sampling period during which the GPU was busy.
    pub utilization_percent: Option<u8>,
    pub temperature_c: Option<u8>,
    pub power_draw_w: Option<f32>,
    pub power_limit_w: Option<f32>,
}
```

### ProcessSnapshot

[crates/music-server/src/resources.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/resources.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct ProcessSnapshot {
    pub name: String,
    pub memory_mb: u64,
    pub cpu_percent: f32,
}
```

### ResourceSnapshot

[crates/music-server/src/resources.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/resources.rs)

```rust
#[derive(Debug, Clone, Serialize)]
struct ResourceSnapshot {
    pub cpu_percent: f32,
    pub ram_used_mb: u64,
    pub ram_total_mb: u64,
    pub gpus: Vec<GpuSnapshot>,
    /// The native engine process, when it is running on this machine.
    pub engine_process: Option<ProcessSnapshot>,
    /// Resident memory of the studio server itself.
    pub studio_process_mb: u64,
}
```

### Destination

[crates/music-server/src/saving.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/saving.rs)

```rust
#[derive(Debug, Deserialize)]
struct Destination {
    name: String,
}
```

### Served

[crates/music-server/src/saving.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/saving.rs)

```rust
#[derive(Debug, Deserialize)]
struct Served {
    url: String,
}
```

### Written

[crates/music-server/src/saving.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/saving.rs)

```rust
#[derive(Debug, Deserialize)]
struct Written {
    path: String,
}
```

### MidiRequest

[crates/music-server/src/score/api.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs)

```rust
#[derive(Deserialize)]
struct MidiRequest {
    abc: String,
}
```

### MarkRequest

[crates/music-server/src/score/api.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs)

```rust
#[derive(Deserialize)]
struct MarkRequest {
    abc: String,
    style: String,
    lyrics: String,
    cot: String,
    #[serde(default)]
    keep: bool,
}
```

### FromMidiRequest

[crates/music-server/src/score/api.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs)

```rust
#[derive(Deserialize)]
struct FromMidiRequest {
    /// The file, as base64.
    data: String,
    #[serde(default)]
    mode: Option<String>,
    #[serde(default)]
    vocal: Option<String>,
    #[serde(default)]
    instrument: Option<String>,
    /// Keep the sections the file names; without them the lyrics are laid along the tune when it is sung.
    #[serde(default)]
    sections: bool,
    #[serde(default)]
    grid: Option<u32>,
    #[serde(default)]
    vocal_octaves: i32,
    #[serde(default)]
    instrument_octaves: i32,
}
```

### ChordBedRequest

[crates/music-server/src/score/api.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs)

```rust
#[derive(Deserialize)]
struct ChordBedRequest {
    bpm: u32,
    key: String,
    meter: String,
    #[serde(default)]
    lyrics: String,
    #[serde(default)]
    seed: Option<u64>,
}
```

### VocalOctaveRequest

[crates/music-server/src/score/api.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/api.rs)

```rust
#[derive(serde::Deserialize)]
struct VocalOctaveRequest {
    abc: String,
    #[serde(default)]
    octaves: i32,
}
```

### Bed

[crates/music-server/src/score/chord_bed.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/chord_bed.rs)

```rust
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
struct Bed {
    pub abc: String,
    pub bars: usize,
    /// The progression of the verses, the choruses and a bridge, as chord symbols.
    pub progressions: Vec<Vec<String>>,
}
```

### Facts

[crates/music-server/src/score/import/mod.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/import/mod.rs)

```rust
#[derive(Clone, Debug, Serialize)]
struct Facts {
    pub bpm: i64,
    pub meter: String,
    pub bars: usize,
    pub seconds: f64,
    pub key: String,
    pub key_source: &'static str,
    pub grid: u32,
    pub voice: Option<usize>,
    pub instrument: Option<usize>,
    pub voice_shift: i32,
    pub instrument_shift: i32,
    pub karaoke: bool,
}
```

### Notice

[crates/music-server/src/score/import/mod.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/import/mod.rs)

```rust
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(tag = "code", rename_all = "snake_case")]
enum Notice {
    TempoChanges { low: f64, high: f64, bpm: f64 },
    PlayedIn,
    Moved { line: &'static str, track: usize, semitones: i32 },
    Struck { track: usize, count: usize },
    ChordsRead { track: usize, name: String },
    Unnamed { track: usize, bars: Vec<usize> },
    ChordsGuessed,
}
```

### Converted

[crates/music-server/src/score/import/mod.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/import/mod.rs)

```rust
#[derive(Clone, Debug, Serialize)]
struct Converted {
    pub abc: String,
    pub lyrics: String,
    pub parts: Vec<parts::Row>,
    pub facts: Facts,
    pub notices: Vec<Notice>,
}
```

### Row

[crates/music-server/src/score/import/parts.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/import/parts.rs)

```rust
#[derive(Clone, Debug, Serialize)]
struct Row {
    pub number: usize,
    pub name: String,
    pub family: &'static str,
    pub notes: usize,
    pub low: u8,
    pub high: u8,
    pub drums: bool,
    pub polyphony: usize,
    pub role: &'static str,
    pub why: &'static str,
}
```

### Transfer

[crates/music-server/src/score/instrumental.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/instrumental.rs)

```rust
#[derive(Debug, Serialize, PartialEq)]
struct Transfer {
    pub abc: String,
    /// Vocal notes now on Ins.
    pub moved: usize,
    /// Ins notes shortened or split around the voice.
    pub trimmed: usize,
    /// Ins notes the voice covered whole.
    pub dropped: usize,
}
```

### SheetBar

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
struct SheetBar {
    pub start: i64,
    pub length: i64,
    pub meter: String,
    pub key: String,
    pub section: String,
    pub editable: bool,
}
```

### SheetSection

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
struct SheetSection {
    pub name: String,
    pub bar: usize,
    pub bars: usize,
}
```

### SheetNote

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
struct SheetNote {
    pub start: i64,
    pub length: i64,
    pub pitch: i64,
}
```

### SheetChord

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
struct SheetChord {
    pub start: i64,
    pub name: String,
}
```

### SheetNotes

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[allow(non_snake_case)]
struct SheetNotes {
    pub Vocal: Vec<SheetNote>,
    pub Ins: Vec<SheetNote>,
}
```

### Sheet

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Serialize)]
struct Sheet {
    pub unit: i64,
    pub per_quarter: i64,
    pub bpm: u32,
    pub seconds: f64,
    pub total: i64,
    pub bars: Vec<SheetBar>,
    pub sections: Vec<SheetSection>,
    pub notes: SheetNotes,
    pub chords: Vec<SheetChord>,
    pub signatures: BTreeMap<String, i32>,
}
```

### Written

[crates/music-server/src/score/notation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/notation.rs)

```rust
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
struct Written {
    pub abc: String,
    pub bars: Vec<usize>,
}
```

### Crowded

[crates/music-server/src/score/phrasing.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/phrasing.rs)

```rust
#[derive(Clone, Copy, Debug, serde::Serialize)]
struct Crowded {
    pub ratio: f64,
    pub notes: usize,
    pub syllables: usize,
}
```

### BlockReport

[crates/music-server/src/score/section_match.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/section_match.rs)

```rust
#[derive(Clone, Debug, PartialEq, Serialize)]
struct BlockReport {
    /// 1-based, in the original lyrics.
    pub index: usize,
    pub tag: Option<String>,
    pub new_tag: Option<String>,
    /// 1-based score section, none when the block was left where it was.
    pub section: Option<usize>,
    pub first_line_seconds: Option<f64>,
    /// kept, renamed, merged, unsure or dropped.
    pub status: &'static str,
}
```

### Filled

[crates/music-server/src/score/section_match.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/section_match.rs)

```rust
#[derive(Clone, Debug, PartialEq, Serialize)]
struct Filled {
    pub section: usize,
    pub tag: String,
    pub copied_from: Option<usize>,
}
```

### Proposal

[crates/music-server/src/score/section_match.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/score/section_match.rs)

```rust
#[derive(Clone, Debug, PartialEq, Serialize)]
struct Proposal {
    pub lyrics: String,
    pub blocks: Vec<BlockReport>,
    pub filled: Vec<Filled>,
    pub unchanged: bool,
}
```

### SeparationConfig

[crates/music-server/src/separation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/separation.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(default)]
struct SeparationConfig {
    /// Graphics card or processor. "auto" takes the card when its runtime is
    /// installed, which is the difference between a minute and ten.
    pub runtime: crate::lyrics_sync::OnnxFlavour,
    /// Which stems to keep. Every one still costs the same run - the model
    /// returns all six - but writing only what the user wants keeps the
    /// library from filling with silence they never asked for.
    pub stems: Vec<String>,
    /// How much neighbouring segments share. More overlap means more passes
    /// and a smoother seam; a quarter is the model's own reference setting.
    pub overlap: f64,
    /// Which separator runs: HT-Demucs for six stems, Mel-Band RoFormer for the
    /// vocals and the rest. It also separates the vocals the vocals-only mode
    /// and training take.
    pub engine: Engine,
}
```

### Engine

[crates/music-server/src/separation.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/separation.rs)

```rust
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
enum Engine {
    #[default]
    Htdemucs,
    Roformer,
}
```

### Request

[crates/music-server/src/song_tokenizer.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/song_tokenizer.rs)

```rust
#[derive(Deserialize)]
struct Request {
    #[serde(default)] style: String,
    #[serde(default)] lyrics: String,
    #[serde(default = "full")] cot: String,
}
```

### Piece

[crates/music-server/src/song_tokenizer.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/song_tokenizer.rs)

```rust
#[derive(Deserialize)]
struct Piece { id: i32, start: usize, end: usize, bytes: Vec<u8> }
```

### Encoded

[crates/music-server/src/song_tokenizer.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/song_tokenizer.rs)

```rust
#[derive(Deserialize)]
struct Encoded { text: String, lyrics: String, lyrics_start_bytes: usize, lyrics_end_bytes: usize, pieces: Vec<Piece> }
```

### DatasetItem

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct DatasetItem {
    pub id: String,
    pub title: String,
    /// Who sings it, for finding its lyrics; from the file's tags or name.
    #[serde(default)]
    pub artist: String,
    /// Where the lyrics came from: a lyrics database's name, "recognised",
    /// or empty when they were written or came with the file.
    #[serde(default)]
    pub lyrics_source: String,
    /// The style sentence the model is prompted with.
    #[serde(default)]
    pub style: String,
    #[serde(default)]
    pub lyrics: String,
    #[serde(default)]
    pub instrumental: bool,
    /// The WAV file inside the dataset's `audio` folder.
    pub file: String,
    pub seconds: f64,
    /// Where it came from: a library song id or an imported file name.
    #[serde(default)]
    pub source: String,
    /// Where the lyrics stand in preparation; a stopped job picks up here.
    pub lyrics_state: LyricsState,
    /// Where the style stands in preparation.
    pub style_state: StyleState,
    /// What the listening model heard, kept until the style is written from it.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub heard: Option<HeardNote>,
}
```

### LyricsState

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
enum LyricsState {
    /// To be looked up or recognised.
    Wanted,
    /// Found or recognised; still to be laid out in sections.
    Found,
    /// Laid out, written by hand, or an instrumental.
    Done,
}
```

### StyleState

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
enum StyleState {
    /// To be listened to.
    Wanted,
    /// Heard; the style sentence is still to be written from it.
    Heard,
    /// Written, by the assistant or by hand.
    Done,
}
```

### HeardNote

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
struct HeardNote {
    pub genre: String,
    pub caption: String,
    pub bpm: u32,
}
```

### Dataset

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct Dataset {
    #[serde(default = "dataset_format")]
    pub format: String,
    pub id: String,
    pub name: String,
    /// A rare word the adapter learns to answer to.
    #[serde(default)]
    pub trigger: String,
    /// The user set the trigger, an empty one included; a word made from the
    /// name fills it only while this is false.
    #[serde(default)]
    pub trigger_chosen: bool,
    pub created_at: String,
    #[serde(default)]
    pub items: Vec<DatasetItem>,
}
```

### RunStatus

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum RunStatus {
    Running,
    Done,
    Failed,
    Cancelled,
    /// The studio closed while it ran.
    Interrupted,
}
```

### Run

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct Run {
    pub id: String,
    pub engine: String,
    pub dataset_id: String,
    pub dataset_name: String,
    pub name: String,
    pub trigger: String,
    pub recipe: Recipe,
    pub status: RunStatus,
    /// The stage working now, or the one that failed.
    pub stage: Option<String>,
    pub stages: Vec<String>,
    #[serde(default)]
    pub steps: Vec<TrainingStepRecord>,
    #[serde(default)]
    pub error: Option<String>,
    pub created_at: String,
    #[serde(default)]
    pub finished_at: Option<String>,
    /// Checkpoints already added to the adapter library, by step.
    #[serde(default)]
    pub installed: Vec<u32>,
    /// Each time the run was trained further, oldest first.
    #[serde(default)]
    pub continuations: Vec<Continuation>,
}
```

### Continuation

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct Continuation {
    pub from: u32,
    pub to: u32,
    pub at: String,
}
```

### TrainingStepRecord

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
struct TrainingStepRecord {
    pub step: u32,
    pub loss: f64,
    #[serde(default)]
    pub ar_kl: Option<f64>,
    #[serde(default)]
    pub step_ms: Option<f64>,
}
```

### TrainerSource

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Deserialize)]
struct TrainerSource {
    commit: String,
    shipped_as: String,
    release_tag: String,
    asset: String,
    /// Size of the released archive.
    bytes: u64,
    /// Size of the captioner's archive in the same release.
    caption_bytes: u64,
}
```

### ItemPatch

[crates/music-server/src/training.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/training.rs)

```rust
#[derive(Debug, Clone, Default, Deserialize)]
struct ItemPatch {
    pub title: Option<String>,
    pub artist: Option<String>,
    pub lyrics_source: Option<String>,
    pub style: Option<String>,
    pub lyrics: Option<String>,
    pub instrumental: Option<bool>,
    /// Set by the preparation, never by the page.
    #[serde(skip)]
    pub lyrics_state: Option<LyricsState>,
    #[serde(skip)]
    pub style_state: Option<StyleState>,
    #[serde(skip)]
    pub heard: Option<Option<HeardNote>>,
}
```

### VstPlugin

[crates/music-server/src/vst.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/vst.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct VstPlugin {
    pub name: String,
    #[serde(default)]
    pub vendor: String,
    pub path: String,
    #[serde(default)]
    pub uid: String,
}
```

### VstSlot

[crates/music-server/src/vst.rs](https://github.com/ictrektech/v-YuE2/blob/main/crates/music-server/src/vst.rs)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
struct VstSlot {
    pub path: String,
    pub name: String,
    /// The state file the plugin's window writes, when it was set up.
    #[serde(default)]
    pub state_id: Option<String>,
    #[serde(default = "enabled")]
    pub enabled: bool,
}
```

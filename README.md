# v-YuE2 for VOS

Free, local song creation with lyrics, editable ABC scores, covers and a private song library, adapted from [YuE2 Studio](https://github.com/timoncool/YuE2-Studio).

- Open from VOS: `/app/com.ictrek.v-yue2/`. VOS OAuth2 PKCE authenticates each account; songs and settings are isolated per account. The app has one flat VOS navigation entry; its displayed version comes from the installed package runtime.
- No model weights in the app package or service image. Confirm the model licenses, choose a model set, and request a download. Model administrators download selected files through V-Scheduler from [`vivibit/YuE2-Studio-GGUF`](https://modelscope.cn/models/vivibit/YuE2-Studio-GGUF).
- V-Scheduler owns shared downloads and snapshots; v-YuE2 mounts its model directory read-only, verifies the pinned SHA-256 and links the chosen files into the account workspace. Remove shared files through V-Scheduler. Models, songs and settings remain in persistent storage after restart.
- Chinese and English follow VOS language changes without reopening the app. Program updates use VOS App Store, not the upstream desktop updater. Upstream telemetry/news are disabled.
- Calibrated song generation, the library and browser score/MIDI editing are available. Local score-to-audio, replay variants and transcription need separate resource calibration before execution. Desktop executable/folder pickers, network sharing, CLI agents, VST hosts and LoRA training are not exposed as VOS browser operations. Optional upstream tools must not be presented as commercially licensed models.

## Providers and local compute

The Providers page selects models discovered from V-Scheduler for writing, cover generation and speech recognition. Only models declaring both the required capability and the supported synchronous API appear. Configure upstreams and provider credentials in V-Scheduler; the VOS app neither stores OpenRouter keys nor calls OpenRouter directly. Model requests use the current VOS user's identity, retaining private-model visibility and gateway authorization. Cloud models do not consume local compute permits; local gateway models follow V-Scheduler policy.

Native YuE2 jobs require a trusted V-Scheduler binding, a calibrated executor and a claimed resource permit before the native engine starts. Admission waits use V-Scheduler priorities; active jobs renew and verify their lease, receive stop requests, and return resources only after the native process group exits and model memory is unloaded. Missing approvals, unavailable scheduling, unsupported model combinations and out-of-range input are rejected rather than executed directly. Idle account services do not load the engine. Interrupted receipts stay in a private journal for stop/release reconciliation.

Measured native calibrations cover `amd` and NVIDIA Thor `thor-spark`, Q8 backbone + F32 VAE + companion v9, one job/batch, 8 threads, a fixed 8192-token context and strict unload. Bounds are supplied by the trusted calibration; the intended validation envelope is at most 120 seconds, 32 steps, 1024 prompt Unicode characters and 4096 lyric Unicode characters. LoRA, custom sampling, score-to-audio variants and uncalibrated local transcription are not covered by that executor. The Thor measurement approves 2 vCPU / 10 GiB unified memory, with no separate duplicate GPU-memory budget. Other profiles and model sets require their own calibration; model readiness alone never authorizes compute. The container also enforces 8 CPU / 32 GiB RAM limits.

## Model licenses

Application source: MIT; dependencies retain their individual licenses. YuE2 backbone/VAE weights: CC BY-NC 4.0 with the official individual-creator permission. Company commercial use requires separate authorization. SheetSage2, MERT and companion do not automatically receive that permission. Free app distribution and mirror hosting do not grant additional rights. See [original licenses](vos/licenses), [model manifest](vos/models.json), [third-party notices](THIRD_PARTY_NOTICES.md) and [upstream documentation](UPSTREAM_README.md).

## Build and release

Local machine is authoritative. Stage pinned engine source and Rust dependencies locally with `vos/scripts/prepare_sources.sh`, then rsync the repository and sources to an architecture-matched Linux build host. All build artifacts stay under `~/dev/build/v-YuE2`. Never run Git on build hosts.

```sh
bash docker/build_image.sh --sheet AMD_with_cuda --build-only
bash docker/build_image.sh --sheet AMD_with_cuda --publish-only
# After every required profile image is pushed and recorded in Feishu:
bash ictrek.app/scripts/update_version.sh patch
```

Profiles: AMD64 CUDA/CPU, ARM64 CUDA/CPU, Jetson/L4T and Thor/Spark. Availability is established by the release validation record, not by a build alone. `MODEL_HUB_SHARED_MODELS_PATH` must match V-Scheduler. User data is under `${VOS_APP_STORAGE_PATH}/data/users/<sha256-sub>/`; shared models are never removed by this app.

Runtime is Go + Rust + C++; no Python or Node.js runtime. Python is used only for build/release/model publication tooling on build hosts. Source model publication: `vos/scripts/publish_models.py --build-root ~/dev/build/v-YuE2/models --credentials <protected-file> --publish`.

VOS 的歌曲完成后会回收原生计算进程及权重。音频和其他无请求体的 GET/HEAD 读取若撞上该进程退出，会在确认旧进程退出后自动恢复一次；创作、导入等 POST 请求不会重放。账户目录、作品及原生停止证明不变。

仅修改 VOS Go 边界时，可使用 `docker/build_image.sh --sheet <原平台表> --gateway-only --gateway-base-image <已发布的同-profile镜像>`。基线镜像须从现有正式安装包/飞书记录核对，保留该镜像内的音乐服务、界面、原生引擎和系统依赖，只替换边界二进制，再按原流程发布镜像和更新飞书。缺少同 profile 基线或架构不匹配时拒绝继续；无 Go 的同架构机器须显式传入已构建的 `YUE_GATEWAY_BIN`。原完整构建方式保留。

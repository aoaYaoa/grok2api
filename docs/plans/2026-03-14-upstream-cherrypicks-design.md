# 上游精选合并（cherrypick）设计说明

**目标**
- 从 `xianyudaxian` 合并“多图参考”相关核心功能与必要修复。
- 从 `chenyme` 合并低冲突、高价值修复（避开 `_public` 目录重构与 API 目录迁移）。

**范围**
- 仅 cherry-pick 指定提交，不做全量合并。
- 全程在安全分支 `codex/merge-upstreams-20260314` 上完成。

**不做的事**
- 不引入 `_public` 目录重构与 `function` API 迁移。
- 不改现有路由结构与静态资源目录结构。

**合并策略**
- 按“依赖顺序”挑选提交，先落地核心 API 再落地 UI 修复。
- 单提交冲突就地解决，完成后立即跑最小验证。
- 合并完再恢复本地 stash（并发配置与测试）。

**拟合并提交**
- 来自 `xianyudaxian/main`：
  - `0dd0b40`（视频工作台参考图交互基础）
  - `df8c592`（图片编辑/视频工作台交互修复）
  - `76d2d62`（图片编辑/视频最终 payload 日志）
  - `bf86ff9`（视频多图请求修复）
  - `699bc08`（核心：视频 API 多图参考输入）
- 来自 `chenyme/main`（低冲突修复）：
  - `d322c46`（移除废弃模型 + 增强 payload 日志）
  - `f180c95`（CF cookies 配置增强）
  - `56bbd4b`（视频生成修复）

**测试与验证**
- Python（最小集）：
  - `python3 -m unittest tests.test_video_extension_payload -v`
  - `python3 -m unittest tests.test_video_extension_runtime_errors -v`
- JS（若 UI/JS 有冲突或改动）：
  - `node --test tests/nsfw_video_extend_entries.test.cjs`
  - `node --test tests/imagine_image_action_entries.test.cjs`

**回滚策略**
- 保留 `main` 不动。
- 如果任何提交冲突不可控，立即 `git cherry-pick --abort` 并回退到上一步。


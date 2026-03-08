# Video Extend Entry Design

## Goal
为 `video` 与 `nsfw` 工作台补齐显式的延长入口，减少用户必须先进入编辑区/猜测隐藏能力的操作成本。

## Confirmed UX
- `video` 模块保留现有工作区“延长视频”主按钮。
- `video` 模块新增卡片级快捷延长入口：历史/结果卡片、缓存视频列表都可直接点“延长”。
- `nsfw` 模块新增主按钮“延长当前视频”。
- `nsfw` 模块新增视频结果卡片级快捷延长入口。
- 所有入口统一复用现有 `/v1/public/video/start` 的 `is_video_extension` 能力，不新增后端路径。

## State And Data Flow
- `video`：点击快捷延长时，先把对应视频绑定为当前工作区视频，更新 `currentExtendPostId/originalFileAttachmentId`，再调用现有 `runExtendVideo()`。
- `nsfw`：新增“当前视频”状态；视频生成完成后可被选中，主按钮作用于当前选中视频，卡片按钮作用于对应卡片。
- `nsfw` 延长请求默认复用当前页面的画幅、分辨率、提示词，延长时长固定为 10 秒；起始时间先使用 0，避免再引入时间轴复杂度。
- `original_post_id/file_attachment_id` 优先使用视频生成来源的 `parentPostId`，缺失时回退到当前视频 postId。

## Error Handling
- 没有可用视频 URL 或无法提取 postId 时，直接 toast 明确报错。
- 延长进行中时，主按钮与卡片按钮进入禁用态，避免重复触发。
- 单卡片延长失败只影响该卡片状态，不阻塞其它已完成视频。

## Verification
- 增加静态回归测试，锁定 `nsfw` 主按钮、移动端代理按钮，以及 `video/nsfw` 的快捷延长选择器。
- 运行现有 Python 单测与新增 Node 静态测试。
- 对修改过的 Python 文件跑 `py_compile`，前端文件做最小 smoke 检查。

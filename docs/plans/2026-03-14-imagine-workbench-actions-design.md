# 图片编辑工作台历史动作与预览灯箱设计

日期: 2026-03-14

## 背景
图片编辑工作台的历史卡片缺少“继续/扩图”动作区与提示词增强回显，预览大图缺少可点击放大与下载入口，导致测试与体验不一致。

## 目标
- 补齐历史卡片动作区：提示词输入、灰色提示文本、继续/扩图按钮、增强提示词回显。
- 保持原功能不受影响，不对已有交互产生副作用。
- 补齐预览灯箱：点击当前画面或历史缩略图可打开预览并下载。
- 对齐 `tests/imagine_image_action_entries.test.cjs` 的期望。

## 非目标
- 不重构公共模块，不抽取共享组件。
- 不改变接口协议或服务端逻辑。

## 方案与取舍
采用“仅在工作台内补齐 DOM 与 JS 逻辑”的方案（低风险，改动最小）。不抽公共模块以避免影响瀑布流与已有页面。

## 组件与结构
- **预览灯箱 DOM**：新增 `#previewLightbox`、`#previewLightboxImg`、`#previewLightboxDownload` 与关闭按钮。
- **历史动作区 DOM**：在历史卡片内添加提示词输入框、灰色提示文本、继续/扩图按钮，以及提示词增强按钮区域。

## 数据流与交互
- 历史提示词输入：仅用于当前动作，不做自动回填；增强结果回显到输入框，并写入本地存储以满足测试期望。
- 继续/扩图动作：优先使用本地预览 URL，再使用历史来源 URL，最后回落 public URL；当参考图数量 >= 2 时进入合并模式并抑制 parentPostId 链路。
- 预览灯箱：点击当前画面或历史缩略图打开；设置图片 src 与下载链接；点击遮罩或关闭按钮退出。

## 错误处理与边界
- 缺少 URL 时不打开灯箱；提示 toast。
- 缺少 prompt 时允许继续提交（保持原有容错）。

## 测试与验证
- 通过 `node --test tests/imagine_image_action_entries.test.cjs`。
- 关注现有工作台功能不回归（上传/拖拽/编辑/历史渲染）。

## 受影响文件
- `app/static/public/pages/imagine_workbench.html`
- `app/static/public/js/imagine_workbench.js`
- `app/static/public/css/imagine_workbench.css`（预计无需改动，仅确认已有样式生效）

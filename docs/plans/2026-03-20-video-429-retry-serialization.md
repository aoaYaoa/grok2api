# Video 429 Retry Serialization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让同一 `parent_post_id` 的视频请求不再因并发撞上同一绑定 token 而整体 429 失败，并在上游返回 429 时自动切换下一个可用 token 重试。

**Architecture:** 保持现有 `/v1/public/video/start` 与 `/v1/public/video/sse` 接口不变，只在服务层补两层保护。第一层是在 `VideoService` 里让上游 429 继续以 `UpstreamException` 形式冒泡到 `completions()` 外层，以便复用现有 `mark_rate_limited()` 与 token 切换逻辑。第二层是在 parent-post 视频路径上对同一 `parent_post_id` 加轻量异步锁，避免同一张图并发 4 路同时打到同一绑定 token。

**Tech Stack:** FastAPI, asyncio, unittest, Docker Compose

---

### Task 1: Document the failing 429 retry behavior with tests

**Files:**
- Modify: `tests/test_video_auth_fallback.py`
- Test: `tests/test_video_auth_fallback.py`

**Step 1: Write the failing test**

新增两个测试：
- `test_parent_post_429_uses_next_token`
- `test_same_parent_post_requests_are_serialized`

第一个测试约束：
- 首个 token 命中 `generate_from_parent_post()` 返回 `UpstreamException(..., status_code=429)`
- `VideoService.completions()` 必须调用 `token_mgr.mark_rate_limited(first_token)`
- 然后切到下一个 token 再次执行并最终成功

第二个测试约束：
- 并发两次针对同一 `parent_post_id` 调用 parent-post 视频路径
- 第二个调用必须等第一个调用释放同图锁后再进入真正的上游请求

**Step 2: Run test to verify it fails**

Run: `python3 -m unittest tests.test_video_auth_fallback.VideoAuthFallbackTests.test_parent_post_429_uses_next_token`
Expected: FAIL，因为当前 parent-post 路径把 429 包成 `AppException`，外层不会切 token

Run: `python3 -m unittest tests.test_video_auth_fallback.VideoAuthFallbackTests.test_same_parent_post_requests_are_serialized`
Expected: FAIL，因为当前没有同图串行锁

**Step 3: Write minimal implementation**

不在这一步实现，只确认测试能准确卡住两个行为。

**Step 4: Run test to verify it fails**

Run: 同 Step 2
Expected: 两个测试都红，且失败原因与预期一致

**Step 5: Commit**

```bash
git add tests/test_video_auth_fallback.py
git commit -m "test: cover video 429 retry and parent serialization"
```

### Task 2: Let parent-post 429 bubble to the outer retry loop

**Files:**
- Modify: `app/services/grok/services/video.py`
- Test: `tests/test_video_auth_fallback.py`

**Step 1: Write the failing test**

使用 Task 1 的 `test_parent_post_429_uses_next_token`。

**Step 2: Run test to verify it fails**

Run: `python3 -m unittest tests.test_video_auth_fallback.VideoAuthFallbackTests.test_parent_post_429_uses_next_token`
Expected: FAIL，外层没有切 token

**Step 3: Write minimal implementation**

在 `generate_from_parent_post()` 的异常处理里：
- 如果异常本身是 `UpstreamException`，直接 `raise`
- 避免把 429、401 这类上游状态过早包成 `AppException`
- 保留其他异常走 `_classify_video_error()` 归一化

**Step 4: Run test to verify it passes**

Run: `python3 -m unittest tests.test_video_auth_fallback.VideoAuthFallbackTests.test_parent_post_429_uses_next_token`
Expected: PASS

**Step 5: Commit**

```bash
git add app/services/grok/services/video.py tests/test_video_auth_fallback.py
git commit -m "fix: retry parent post video on upstream 429"
```

### Task 3: Serialize concurrent requests for the same parent post

**Files:**
- Modify: `app/services/grok/services/video.py`
- Test: `tests/test_video_auth_fallback.py`

**Step 1: Write the failing test**

使用 Task 1 的 `test_same_parent_post_requests_are_serialized`。

**Step 2: Run test to verify it fails**

Run: `python3 -m unittest tests.test_video_auth_fallback.VideoAuthFallbackTests.test_same_parent_post_requests_are_serialized`
Expected: FAIL，因为当前两个并发调用会同时进入上游请求

**Step 3: Write minimal implementation**

在 `app/services/grok/services/video.py`：
- 新增基于 `parent_post_id` 的进程内 `asyncio.Lock` 映射
- 只包住 parent-post 视频请求上游段，不扩大到整个 `completions()` 生命周期
- 非 parent-post、multi-reference、video extension 路径不受影响

**Step 4: Run test to verify it passes**

Run: `python3 -m unittest tests.test_video_auth_fallback.VideoAuthFallbackTests.test_same_parent_post_requests_are_serialized`
Expected: PASS

**Step 5: Commit**

```bash
git add app/services/grok/services/video.py tests/test_video_auth_fallback.py
git commit -m "fix: serialize concurrent parent post video requests"
```

### Task 4: Verify the full fix and redeploy locally

**Files:**
- Modify: `docker-compose.yml` (none expected)
- Test: `tests/test_video_auth_fallback.py`

**Step 1: Run targeted regression tests**

Run: `python3 -m unittest tests.test_video_auth_fallback`
Expected: PASS

Run: `python3 -m unittest tests.test_media_post_import tests.test_imagine_parent_post_route tests.test_video_auth_fallback`
Expected: PASS

**Step 2: Rebuild local Docker**

Run: `docker compose up -d --build grok2api`
Expected: container recreated and started successfully

**Step 3: Verify service health**

Run: `curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18000/v1/public/verify`
Expected: `200`

**Step 4: Manual regression check**

在 `NSFW 工作台` 选择同一张图并发发视频：
- 不应再立刻出现 4 路一起 `429`
- 若首个 token 被限流，应看到后续 token 接管

**Step 5: Commit**

```bash
git add app/services/grok/services/video.py tests/test_video_auth_fallback.py docs/plans/2026-03-20-video-429-retry-serialization.md
git commit -m "fix: harden parent post video retry under 429"
```

# Videos Compatibility Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `/v1/videos` compatibility endpoint so the shared pure-HTML client can call local `grok2api` without understanding internal `chat.completion` video responses.

**Architecture:** Keep the existing video generation pipeline unchanged and add a thin adapter in `app/api/v1/video_api.py`. The adapter will accept the HTML client's request shape, map it to `VideoService.completions(...)`, normalize the returned HTML/markdown/url content into a direct video URL, and return an image-style JSON payload with `data[0].url`.

**Tech Stack:** FastAPI, Pydantic, existing `VideoService`, unittest with `AsyncMock`

---

### Task 1: Lock compatibility behavior with tests

**Files:**
- Create: `tests/test_videos_compat_api.py`
- Modify: `app/api/v1/video_api.py`

**Step 1: Write the failing tests**

- Add a test for `POST /v1/videos` request mapping:
  - `size=1792x1024` becomes `aspect_ratio=3:2`
  - `seconds=18` becomes `video_length=18`
  - `quality=high` becomes `resolution=720p`
  - `image_reference` becomes a user `image_url` message and uses `single_image_mode=reference`
- Add a test proving HTML video content is normalized into `{"data":[{"url":"..."}]}`
- Add a test proving markdown `[video](...)` content is normalized into the same response shape

**Step 2: Run the focused test file and verify it fails**

Run: `python3 -m unittest tests.test_videos_compat_api -v`

Expected: FAIL because `/v1/videos` compatibility code does not exist yet.

### Task 2: Implement the compatibility endpoint

**Files:**
- Modify: `app/api/v1/video_api.py`

**Step 1: Add request model and helpers**

- Define a request model for the HTML client's payload:
  - `model`, `prompt`, `size`, `seconds`, `quality`, `image_reference`, `stream`
- Add helper functions to:
  - map `size` to aspect ratio
  - map `quality` to `480p/720p`
  - extract a direct video URL from HTML, markdown, or plain URL content

**Step 2: Add `POST /videos`**

- Call `VideoService.completions(...)` with `stream=False`
- Convert the response into:

```json
{
  "created": 0,
  "data": [
    { "url": "http://..." }
  ]
}
```

- Return useful HTTP errors when the result cannot be normalized into a playable URL

**Step 3: Keep the rest of the video API untouched**

- Do not change `/v1/chat/completions`
- Do not change `/v1/public/video/*`
- Do not change `/v1/video/extend`

### Task 3: Verify

**Files:**
- Test: `tests/test_videos_compat_api.py`

**Step 1: Run focused tests**

Run: `python3 -m unittest tests.test_videos_compat_api -v`

Expected: PASS

**Step 2: Run one adjacent video test file as regression coverage**

Run: `python3 -m unittest tests.test_video_extension_runtime_errors -v`

Expected: PASS

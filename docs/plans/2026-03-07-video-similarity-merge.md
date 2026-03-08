# Video Similarity Merge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Group visually similar cached videos together, preserve chronological order inside each group, and render a higher-quality 60fps merged MP4.

**Architecture:** Extract three representative frames per source video with ffmpeg in Docker, compute lightweight image features in Python, and build similarity groups with chronological stabilization. Then normalize each video sequentially to a common 960x1280/60fps/high-quality format and concat the normalized outputs to avoid ffmpeg filter-graph OOM.

**Tech Stack:** Docker ffmpeg/ffprobe, shell scripts, Python 3 with PIL and numpy.

---

### Task 1: Build Feature Inputs

**Files:**
- Create: `data/tmp/video/.similarity_extract_<ts>.sh`
- Create: `data/tmp/video/.similarity_frames_<ts>/`
- Input: `data/tmp/video/.merge_order_*.txt`

**Step 1:** Extract 3 representative frames per source video.
**Step 2:** Save them into a timestamped temp directory.
**Step 3:** Verify frame count matches 3 x video count.

### Task 2: Compute Similarity Order

**Files:**
- Create: `data/tmp/video/.similarity_order_<ts>.txt`
- Create: `data/tmp/video/.similarity_groups_<ts>.json`

**Step 1:** Load extracted frames in Python.
**Step 2:** Build normalized grayscale feature vectors.
**Step 3:** Cluster videos by visual similarity.
**Step 4:** Sort items within each group by original chronological order.
**Step 5:** Save final ordered list.

### Task 3: Render High-Quality 60fps Segments

**Files:**
- Create: `data/tmp/video/.merge_hq_run_<ts>.sh`
- Create: `data/tmp/video/.merge_hq_norm_<ts>/`

**Step 1:** Iterate through the similarity-ordered list.
**Step 2:** Normalize each video to `960x1280`, `60fps`, `libx264`, `crf 18`, `preset medium`.
**Step 3:** Use motion interpolation with `minterpolate`.
**Step 4:** Save concat list during rendering.

### Task 4: Concat And Verify

**Files:**
- Create: `data/tmp/video/merged_similarity_hq_<ts>.mp4`

**Step 1:** Concat normalized segments with stream copy.
**Step 2:** Probe final output for duration, resolution, frame rate, and file size.
**Step 3:** Report final path and metrics.

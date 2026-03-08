# Video Similarity Merge Design

- Goal: Group visually similar cached videos together, keep chronological order within each group, and output a higher-quality 60fps merged video.
- Inputs: 133 cached source videos in data/tmp/video.
- Similarity method: Extract 3 representative frames per video, compute lightweight perceptual hashes in Python, sort by nearest-neighbor similarity, then stabilize within groups by original mtime.
- Render method: Sequential normalize + concat pipeline to avoid ffmpeg filter graph OOM; use minterpolate for 60fps and lower CRF for better quality.
- Output: portrait 960x1280 H.264 MP4, 60fps, improved quality over prior build.
- Risks: similarity is visual, not semantic; interpolation increases render time and file size.

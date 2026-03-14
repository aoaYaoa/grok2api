"""
Local cache utilities.
"""

import json
import re
from typing import Any, Dict

from app.core.storage import DATA_DIR

IMAGE_EXTS = {".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}
VIDEO_EXTS = {".mp4", ".mov", ".m4v", ".webm", ".avi", ".mkv"}


class CacheService:
    """Local cache service."""

    def __init__(self):
        base_dir = DATA_DIR / "tmp"
        self.image_dir = base_dir / "image"
        self.video_dir = base_dir / "video"
        self.image_dir.mkdir(parents=True, exist_ok=True)
        self.video_dir.mkdir(parents=True, exist_ok=True)

    def _cache_dir(self, media_type: str):
        return self.image_dir if media_type == "image" else self.video_dir

    def _allowed_exts(self, media_type: str):
        return IMAGE_EXTS if media_type == "image" else VIDEO_EXTS

    def _media_meta_dir(self):
        return DATA_DIR / "tmp" / "media-meta"

    def _load_video_metadata(self) -> dict[str, dict[str, Any]]:
        meta_dir = self._media_meta_dir()
        if not meta_dir.exists():
            return {}

        metadata_by_post_id: dict[str, dict[str, Any]] = {}
        for file_path in meta_dir.glob("*.json"):
            try:
                payload = json.loads(file_path.read_text(encoding="utf-8"))
            except Exception:
                continue
            if not isinstance(payload, dict):
                continue
            if str(payload.get("media_type") or "").strip().lower() != "video":
                continue
            post_id = str(payload.get("post_id") or "").strip()
            if not post_id:
                continue
            metadata_by_post_id[post_id] = payload
        return metadata_by_post_id

    @staticmethod
    def _extract_post_id_from_name(name: str) -> str:
        text = str(name or "").strip()
        if not text:
            return ""
        match = re.search(
            r"generated-([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})-",
            text,
        )
        if match:
            return match.group(1)
        all_matches = re.findall(
            r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}",
            text,
        )
        return all_matches[-1] if all_matches else ""

    def get_stats(self, media_type: str = "image") -> Dict[str, Any]:
        cache_dir = self._cache_dir(media_type)
        if not cache_dir.exists():
            return {"count": 0, "size_mb": 0.0}

        allowed = self._allowed_exts(media_type)
        files = [
            f for f in cache_dir.glob("*") if f.is_file() and f.suffix.lower() in allowed
        ]
        total_size = sum(f.stat().st_size for f in files)
        return {"count": len(files), "size_mb": round(total_size / 1024 / 1024, 2)}

    def list_files(
        self, media_type: str = "image", page: int = 1, page_size: int = 1000
    ) -> Dict[str, Any]:
        cache_dir = self._cache_dir(media_type)
        if not cache_dir.exists():
            return {"total": 0, "page": page, "page_size": page_size, "items": []}

        allowed = self._allowed_exts(media_type)
        files = [
            f for f in cache_dir.glob("*") if f.is_file() and f.suffix.lower() in allowed
        ]

        items = []
        for f in files:
            try:
                stat = f.stat()
                items.append(
                    {
                        "name": f.name,
                        "size_bytes": stat.st_size,
                        "mtime_ms": int(stat.st_mtime * 1000),
                    }
                )
            except Exception:
                continue

        items.sort(key=lambda x: x["mtime_ms"], reverse=True)

        total = len(items)
        start = max(0, (page - 1) * page_size)
        paged = items[start : start + page_size]

        metadata_by_post_id = self._load_video_metadata() if media_type == "video" else {}

        for item in paged:
            item["view_url"] = f"/v1/files/{media_type}/{item['name']}"
            if media_type != "video":
                continue
            post_id = self._extract_post_id_from_name(item["name"])
            if not post_id:
                continue
            metadata = metadata_by_post_id.get(post_id)
            if not metadata:
                continue
            item["post_id"] = str(metadata.get("post_id") or "").strip()
            item["share_link"] = str(metadata.get("share_link") or "").strip()
            item["original_post_id"] = str(metadata.get("original_post_id") or "").strip()
            item["media_url"] = str(metadata.get("media_url") or "").strip()
            item["thumbnail_url"] = str(metadata.get("thumbnail_url") or "").strip()
            item["local_url"] = str(metadata.get("local_url") or "").strip()

        return {"total": total, "page": page, "page_size": page_size, "items": paged}

    def delete_file(self, media_type: str, name: str) -> Dict[str, Any]:
        cache_dir = self._cache_dir(media_type)
        file_path = cache_dir / name.replace("/", "-")

        if file_path.exists():
            try:
                file_path.unlink()
                return {"deleted": True}
            except Exception:
                pass
        return {"deleted": False}

    def clear(self, media_type: str = "image") -> Dict[str, Any]:
        cache_dir = self._cache_dir(media_type)
        if not cache_dir.exists():
            return {"count": 0, "size_mb": 0.0}

        files = list(cache_dir.glob("*"))
        total_size = sum(f.stat().st_size for f in files if f.is_file())
        count = 0

        for f in files:
            if f.is_file():
                try:
                    f.unlink()
                    count += 1
                except Exception:
                    pass

        return {"count": count, "size_mb": round(total_size / 1024 / 1024, 2)}


__all__ = ["CacheService"]

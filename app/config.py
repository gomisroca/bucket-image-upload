"""
Config - Same environment variables as used by the Go version, so the two versions are interchangeable from the outside.
"""

from __future__ import annotations

import os
from dataclasses import dataclass

@dataclass(frozen=True)
class Settings:
    port: int
    max_upload_bytes: int
    api_key: str

    storage_backend: str  # 'local' or 's3'
    upload_dir: str

    s3_bucket: str
    s3_region: str
    s3_endpoint: str
    s3_access_key_id: str
    s3_secret_access_key: str
    s3_public_base_url: str
    s3_presign_ttl_seconds: int

    ratelimiter_url: str
    ratelimiter_api_key: str
    ratelimiter_fail_open: bool

 
def _bool_env(key: str, default: bool) -> bool:
    v = os.getenv(key)
    if v is None:
        return default
    return v.strip().lower() in ("1", "true", "yes", "on")


def load_settings() -> Settings:
    return Settings(
        port=int(os.getenv("PORT", "8080")),
        max_upload_bytes=int(os.getenv("MAX_UPLOAD_BYTES", str(10 * 1024 * 1024))),
        api_key=os.getenv("API_KEY", ""),
        storage_backend=os.getenv("STORAGE_BACKEND", "local"),
        upload_dir=os.getenv("UPLOAD_DIR", "./uploads"),
        s3_bucket=os.getenv("S3_BUCKET", ""),
        s3_region=os.getenv("S3_REGION", ""),
        s3_endpoint=os.getenv("S3_ENDPOINT", ""),
        s3_access_key_id=os.getenv("S3_ACCESS_KEY_ID", ""),
        s3_secret_access_key=os.getenv("S3_SECRET_ACCESS_KEY", ""),
        s3_public_base_url=os.getenv("S3_PUBLIC_BASE_URL", ""),
        s3_presign_ttl_seconds=int(os.getenv("S3_PRESIGN_TTL_SECONDS", "3600")),
        ratelimiter_url=os.getenv("RATELIMITER_URL", ""),
        ratelimiter_api_key=os.getenv("RATELIMITER_API_KEY", ""),
        ratelimiter_fail_open=_bool_env("RATELIMITER_FAIL_OPEN", True),
    )

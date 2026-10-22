#!/usr/bin/env python3
"""Cloudflare R2 Benchmark Artifact Store.

Manages large benchmark evaluation datasets, pre-computed embedding caches,
and evaluation results in Cloudflare R2, keeping bulk binary and JSON artifacts
out of local Git and the application PostgreSQL database.

Usage:
    python r2_store.py push <local_path> <r2_key>
    python r2_store.py pull <r2_key> <local_path>
    python r2_store.py list [prefix]
    python r2_store.py sync-run <run_dir>
"""
import argparse
import hashlib
import json
import os
import pathlib
import sys
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent


def get_r2_config():
    acc = os.environ.get("CLOUDFLARE_ACCOUNT_ID")
    key = os.environ.get("R2_ACCESS_KEY_ID")
    secret = os.environ.get("R2_SECRET_ACCESS_KEY")
    bucket = os.environ.get("R2_BUCKET_NAME", "memro-benchmarks")

    env_file = ROOT / ".env"
    if env_file.exists() and (not acc or not key or not secret):
        for line in env_file.read_text().splitlines():
            line = line.strip()
            if line.startswith("CLOUDFLARE_ACCOUNT_ID="):
                acc = line.split("=", 1)[1].strip().strip('"').strip("'")
            elif line.startswith("R2_ACCESS_KEY_ID="):
                key = line.split("=", 1)[1].strip().strip('"').strip("'")
            elif line.startswith("R2_SECRET_ACCESS_KEY="):
                secret = line.split("=", 1)[1].strip().strip('"').strip("'")
            elif line.startswith("R2_BUCKET_NAME="):
                bucket = line.split("=", 1)[1].strip().strip('"').strip("'")

    endpoint = f"https://{acc}.r2.cloudflarestorage.com" if acc else ""
    return acc, key, secret, bucket, endpoint


def get_s3_client():
    try:
        import boto3
        from botocore.config import Config
    except ImportError:
        return None

    acc, key, secret, bucket, endpoint = get_r2_config()
    if not (key and secret and endpoint):
        return None

    return boto3.client(
        "s3",
        endpoint_url=endpoint,
        aws_access_key_id=key,
        aws_secret_access_key=secret,
        config=Config(signature_version="s3v4"),
        region_name="auto",
    )


def push_file(local_path: pathlib.Path, r2_key: str):
    local_path = pathlib.Path(local_path)
    if not local_path.exists():
        raise FileNotFoundError(f"Local file not found: {local_path}")

    s3 = get_s3_client()
    _, _, _, bucket, _ = get_r2_config()

    if s3:
        print(f"Uploading {local_path} ({local_path.stat().st_size:,} bytes) to R2 bucket '{bucket}': {r2_key} ...")
        s3.upload_file(str(local_path), bucket, r2_key)
        print("Upload successful.")
    else:
        print(f"[Simulated/Dry-Run] Would upload {local_path} to r2://{bucket}/{r2_key}")
        print("Note: boto3 or valid R2 credentials not configured. Set R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY.")


def pull_file(r2_key: str, local_path: pathlib.Path):
    local_path = pathlib.Path(local_path)
    s3 = get_s3_client()
    _, _, _, bucket, _ = get_r2_config()

    if s3:
        local_path.parent.mkdir(parents=True, exist_ok=True)
        print(f"Downloading r2://{bucket}/{r2_key} to {local_path} ...")
        s3.download_file(bucket, r2_key, str(local_path))
        print("Download successful.")
    else:
        print(f"[Simulated/Dry-Run] Would download r2://{bucket}/{r2_key} to {local_path}")


def list_objects(prefix: str = ""):
    s3 = get_s3_client()
    _, _, _, bucket, _ = get_r2_config()

    if s3:
        resp = s3.list_objects_v2(Bucket=bucket, Prefix=prefix)
        items = resp.get("Contents", [])
        print(f"Objects in r2://{bucket}/{prefix} ({len(items)} items):")
        for item in items:
            print(f"- {item['Key']} ({item['Size']:,} bytes, {item['LastModified']})")
    else:
        print(f"[Simulated/Dry-Run] Listing r2://{bucket}/{prefix}")


def sync_run(run_dir: pathlib.Path):
    run_dir = pathlib.Path(run_dir)
    if not run_dir.exists():
        raise FileNotFoundError(f"Run dir not found: {run_dir}")

    for f in run_dir.glob("*"):
        if f.is_file() and not f.name.endswith(".sqlite"):
            r2_key = f"runs/{run_dir.name}/{f.name}"
            push_file(f, r2_key)


def main():
    parser = argparse.ArgumentParser(description="Cloudflare R2 Benchmark Artifact Store")
    sub = parser.add_subparsers(dest="command", required=True)

    push_p = sub.add_parser("push")
    push_p.add_argument("local_path")
    push_p.add_argument("r2_key")

    pull_p = sub.add_parser("pull")
    pull_p.add_argument("r2_key")
    pull_p.add_argument("local_path")

    list_p = sub.add_parser("list")
    list_p.add_argument("prefix", nargs="?", default="")

    sync_p = sub.add_parser("sync-run")
    sync_p.add_argument("run_dir")

    args = parser.parse_args()
    if args.command == "push":
        push_file(args.local_path, args.r2_key)
    elif args.command == "pull":
        pull_file(args.r2_key, args.local_path)
    elif args.command == "list":
        list_objects(args.prefix)
    elif args.command == "sync-run":
        sync_run(args.run_dir)


if __name__ == "__main__":
    main()

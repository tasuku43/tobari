#!/usr/bin/env python3
"""Print the exact repository files needed by the Linux helper build."""

import json
import os
import subprocess


ROOT = os.path.realpath(os.path.join(os.path.dirname(__file__), ".."))
MODULE = "github.com/tasuku43/tobari"


def objects(data: str):
    decoder = json.JSONDecoder()
    offset = 0
    while offset < len(data):
        while offset < len(data) and data[offset].isspace():
            offset += 1
        if offset == len(data):
            return
        value, offset = decoder.raw_decode(data, offset)
        yield value


files = {"go.mod", "go.sum"}
for architecture in ("amd64", "arm64"):
    environment = dict(os.environ, GOOS="linux", GOARCH=architecture, CGO_ENABLED="0")
    result = subprocess.run(
        ["go", "list", "-deps", "-json", "-tags=tobari_exposure_helper", "./cmd/tobari-expose"],
        cwd=ROOT,
        env=environment,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    for package in objects(result.stdout):
        module = package.get("Module") or {}
        if module.get("Path") != MODULE:
            continue
        directory = package["Dir"]
        for field in ("GoFiles", "CgoFiles", "SFiles", "EmbedFiles"):
            for name in package.get(field, []):
                path = os.path.realpath(os.path.join(directory, name))
                if os.path.commonpath((ROOT, path)) != ROOT or not os.path.isfile(path):
                    raise SystemExit(f"unsafe helper source path: {path}")
                relative = os.path.relpath(path, ROOT)
                if relative.startswith("internal/infra/runtimeassets/_helper-source/"):
                    raise SystemExit("helper source closure became recursive")
                files.add(relative)

snapshot_files = []
for name in files:
    if name == "go.mod":
        name = "tobari-go.mod"
    elif name == "go.sum":
        name = "tobari-go.sum"
    snapshot_files.append(name)

for name in sorted(snapshot_files):
    print(name)

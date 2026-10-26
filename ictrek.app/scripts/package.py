#!/usr/bin/env python3
from __future__ import annotations

import argparse
import io
import json
import os
import re
import shutil
import tarfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
DIST = ROOT / "dist"
SPREADSHEET = os.getenv("FEISHU_SPREADSHEET_TOKEN", "Htotsn3oahO1zxt73YMcaB1zn8e")
REPOSITORY = "swr.cn-southwest-2.myhuaweicloud.com/ictrek/v-yue2"
PROFILES = {
    "VYUE2_AMD_IMAGE": ["AMD_with_mxn100"],
    "VYUE2_AMD_CUDA_IMAGE": ["AMD_with_cuda"],
    "VYUE2_ARM_IMAGE": ["ARM_without_cuda", "SOPHON_bm1688"],
    "VYUE2_ARM_CUDA_IMAGE": ["ARM_with_cuda"],
    "VYUE2_L4T_IMAGE": ["l4t"],
    "VYUE2_THOR_IMAGE": ["thor_spark"],
}
DEPENDENCIES = {
    "com.ictrek.v-scheduler": ("ictrektech/V-Scheduler", "0.0.24"),
}


def appstore_version(app_id: str, base: str) -> str:
    """私有 GitHub 仓库改从已授权隧道读取实时发布记录，不扩大 CI 令牌权限。"""
    parsed = urllib.parse.urlparse(base)
    if parsed.scheme != "http" or parsed.hostname not in {"127.0.0.1", "localhost"} or parsed.path not in {"", "/"}:
        raise RuntimeError("App Store dependency verification requires the approved loopback tunnel")
    with urllib.request.urlopen(f"{base.rstrip('/')}/api/appstore/apps/{app_id}", timeout=30) as response:
        record = json.load(response)
    detail = record.get("data") or {}
    version = detail.get("current_version", "")
    versions = [v for v in detail.get("versions", []) if re.fullmatch(r"\d+\.\d+\.\d+", v)]
    archive = detail.get("app_tar_gz") or {}
    expected = f"appstore/apps/{app_id}/{version}/app.tar.gz"
    if (record.get("code") != 0 or detail.get("id") != app_id or not versions
            or version != max(versions, key=lambda v: tuple(map(int, v.split("."))))
            or archive.get("size", 0) <= 0 or archive.get("download_path") != expected):
        raise RuntimeError(f"{app_id}: no verified latest published App Store package")
    # 实际文件必须可访问，不能把仅有索引的记录当成已发布包。
    probe = urllib.request.Request(f"{base.rstrip('/')}/store/{expected}",
                                   headers={"Range": "bytes=0-1", "Accept-Encoding": "identity"})
    with urllib.request.urlopen(probe, timeout=30) as response:
        prefix = response.read(2)
        valid = prefix == b"\x1f\x8b"
        if response.status == 206:
            valid = valid and response.headers.get("Content-Range") == f"bytes 0-1/{archive['size']}"
        elif response.status == 200:
            # 部署端可能忽略 Range 或省略 HEAD 长度；流式计数，不信任 SPA/错误页的 200。
            count = len(prefix)
            while count <= archive["size"]:
                chunk = response.read(min(1024 * 1024, archive["size"] + 1 - count))
                if not chunk:
                    break
                count += len(chunk)
            valid = valid and count == archive["size"]
        else:
            valid = False
        if not valid:
            raise RuntimeError(f"{app_id}: published App Store archive is unavailable or changed")
    return version


def resolve_dependencies(manifest: dict) -> None:
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "v-YuE2 VOS packaging"}
    token = os.getenv("GH_TOKEN")
    if token:
        headers["Authorization"] = "Bearer " + token
    for dependency in manifest.get("dependencies", {}).get("apps", []):
        repository, minimum = DEPENDENCIES[dependency["id"]]
        url = f"https://api.github.com/repos/{repository}/releases/latest"
        source = "GitHub Release"
        try:
            with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=30) as response:
                release = json.load(response)
        except urllib.error.HTTPError as exc:
            base = os.getenv("VYUE2_APPSTORE_API_URL", "")
            if exc.code not in {401, 403, 404} or not base:
                raise RuntimeError(f"{repository}: cannot verify published dependency; App Store tunnel required for private repositories") from exc
            version = appstore_version(dependency["id"], base)
            source = "VOS App Store"
        else:
            match = re.fullmatch(r"v?(\d+\.\d+\.\d+)", release.get("tag_name", ""))
            if (not match or not release.get("published_at") or release.get("draft")
                    or release.get("prerelease") or not any(
                        asset.get("name", "").endswith("_pull.tar") for asset in release.get("assets", []))):
                raise RuntimeError(f"{repository}: no verified published VOS package")
            version = match.group(1)
        # 已发布版本不能替代本应用实际需要的协议下限；不足时明确阻止打包。
        if tuple(map(int, version.split("."))) < tuple(map(int, minimum.split("."))):
            raise RuntimeError(f"{repository}: published {version} lacks required contract >= {minimum}")
        dependency["version"] = ">=" + version
        print(f"Dependency resolved: {dependency['id']} source={source} published={version} package=>={version}")


def request(method: str, url: str, token: str = "", body: dict | None = None) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    with urllib.request.urlopen(urllib.request.Request(url, data=data, headers=headers, method=method), timeout=30) as response:
        value = json.load(response)
    if value.get("code") != 0:
        raise RuntimeError(value)
    return value


def cell_text(value) -> str:
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, list):
        return "".join(cell_text(part) for part in value).strip()
    if isinstance(value, dict):
        return str(value.get("text") or value.get("link") or "").strip()
    return "" if value is None else str(value).strip()


def column_name(index: int) -> str:
    output = ""
    while index:
        index, rem = divmod(index - 1, 26)
        output = chr(65 + rem) + output
    return output


class Feishu:
    def __init__(self) -> None:
        configured = os.getenv("FEISHU_CONFIG_FILE")
        paths = [Path(configured)] if configured else [Path.home() / ".feishu.components.json", Path.home() / ".feishu.json"]
        path = next((candidate for candidate in paths if candidate.is_file()), None)
        if path is None:
            raise RuntimeError("Feishu credential file not found")
        config = json.loads(path.read_text())
        login = request(
            "POST",
            "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
            body={"app_id": config["feishu_app_id"], "app_secret": config["feishu_app_secret"]},
        )
        self.token = login["tenant_access_token"]
        self.base = "https://open.feishu.cn/open-apis/sheets"
        response = request("GET", f"{self.base}/v3/spreadsheets/{SPREADSHEET}/sheets/query", self.token)
        self.sheets = {row["title"]: row["sheet_id"] for row in response["data"]["sheets"]}

    def latest_image(self, title: str) -> str:
        sheet_id = self.sheets[title]
        response = request("GET", f"{self.base}/v2/spreadsheets/{SPREADSHEET}/values/{sheet_id}!A1:ZZ2", self.token)
        rows = response.get("data", {}).get("valueRange", {}).get("values", [])
        header = rows[0] if rows else []
        repos = rows[1] if len(rows) > 1 else []
        column = next((index for index, value in enumerate(header, 1) if cell_text(value) == "v-yue2"), None)
        if column is None:
            raise RuntimeError(f"v-yue2 column missing from {title}")
        repository = cell_text(repos[column - 1]) if len(repos) >= column else REPOSITORY
        response = request(
            "GET",
            f"{self.base}/v2/spreadsheets/{SPREADSHEET}/values/{sheet_id}!{column_name(column)}4:{column_name(column)}2000",
            self.token,
        )
        values = response.get("data", {}).get("valueRange", {}).get("values", [])
        tag = next((cell_text(row[0]) for row in values if row and cell_text(row[0])), "")
        if not tag:
            raise RuntimeError(f"v-yue2 tag missing from {title}")
        return f"{repository or REPOSITORY}:{tag}"


def render(version: str, images: dict[str, str], stage: Path) -> None:
    stage.mkdir(parents=True)
    for name in ("manifest.yml", "configs.yml", "routers.yml", "README.zh-CN.md", "README.en.md"):
        text = (SRC / name).read_text(encoding="utf-8").replace("__APP_VERSION__", version)
        if name == "manifest.yml":
            manifest = yaml.safe_load(text)
            resolve_dependencies(manifest)
            text = yaml.safe_dump(manifest, allow_unicode=True, sort_keys=False)
        (stage / name).write_text(text, encoding="utf-8")
    shutil.copy2(SRC / "icon.png", stage / "icon.png")
    env = "".join(f"{key}={value}\n" for key, value in images.items())
    (stage / ".env").write_text(env, encoding="utf-8")
    compose = (SRC / "docker-compose.yml").read_text(encoding="utf-8").replace("__APP_VERSION__", version)
    compose = re.sub(
        r"\$\{([A-Z0-9_]+)(?::-[^}]*)?\}",
        lambda match: images.get(match.group(1), match.group(0)),
        compose,
    )
    (stage / "docker-compose.yml").write_text(compose, encoding="utf-8")
    if re.search(r"\$\{[^}]*_IMAGE[^}]*\}", compose):
        raise RuntimeError("unrendered image variable remains")


def package(version: str, stage: Path) -> Path:
    DIST.mkdir(exist_ok=True)
    app_tar = DIST / "app.tar.gz"
    members = [".env", "manifest.yml", "docker-compose.yml", "configs.yml", "routers.yml", "icon.png", "README.zh-CN.md", "README.en.md"]
    with tarfile.open(app_tar, "w:gz") as archive:
        for name in members:
            archive.add(stage / name, arcname=name)
    output = DIST / f"v_yue2_{version}_pull.tar"
    payload = app_tar.read_bytes()
    info = tarfile.TarInfo("app.tar.gz")
    info.size = len(payload)
    with tarfile.open(output, "w") as archive:
        archive.addfile(info, io.BytesIO(payload))
    with tarfile.open(output) as archive:
        if archive.getnames() != ["app.tar.gz"]:
            raise RuntimeError("invalid outer package")
    for name in members:
        if name == "icon.png":
            continue
        text = (stage / name).read_text()
        if re.search(r"__[A-Z0-9_]+__", text):
            raise RuntimeError(f"unrendered placeholder: {name}")
    compose = yaml.safe_load((stage / "docker-compose.yml").read_text())
    if not compose.get("networks", {}).get("vos_default", {}).get("external"):
        raise RuntimeError("missing external vos_default network")
    for service in compose["services"].values():
        if not re.match(r"^[\w.-]+\.[\w.-]+(?::\d+)?/[^\s]+$", service.get("image", "")):
            raise RuntimeError("invalid service image")
        if not service.get("networks", {}).get("vos_default", {}).get("aliases"):
            raise RuntimeError("missing vos_default service alias")
    manifest = yaml.safe_load((stage / "manifest.yml").read_text())
    profiles = {row["name"] for row in manifest["profiles"]}
    used = {profile for service in compose["services"].values() for profile in service["profiles"]}
    if profiles != used or manifest["id"] != "com.ictrek.v-yue2":
        raise RuntimeError("manifest/compose profile mismatch")
    routers = yaml.safe_load((stage / "routers.yml").read_text())["routers"]
    if len(routers) != 1:
        raise RuntimeError("expected a single flat VOS entry")
    page = routers[0]
    if (page["id"] != "v-yue2" or page["kind"] != "page"
            or page["iframe-src"] != f"/app/com.ictrek.v-yue2/?v={version}"
            or not page.get("entry-point") or not page.get("embed")):
        raise RuntimeError("invalid VOS navigation contract")
    return output


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", default=os.getenv("PACKAGE_VERSION", ""))
    args = parser.parse_args()
    version = args.version or (ROOT / "VERSION").read_text().strip()
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise SystemExit(f"invalid version: {version}")
    feishu = Feishu()
    images = {}
    for variable, sheets in PROFILES.items():
        errors = []
        for sheet in sheets:
            try:
                images[variable] = feishu.latest_image(sheet)
                break
            except Exception as exc:
                errors.append(str(exc))
        if variable not in images:
            raise RuntimeError("; ".join(errors))
    stage = DIST / "staging"
    if stage.exists():
        shutil.rmtree(stage)
    render(version, images, stage)
    output = package(version, stage)
    print(output)


if __name__ == "__main__":
    main()

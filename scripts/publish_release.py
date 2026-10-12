import json
import os
import subprocess
import sys
import urllib.request
import urllib.error

def get_github_token():
    p = subprocess.Popen(['git', 'credential', 'fill'], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
    out, _ = p.communicate('protocol=https\nhost=github.com\n\n')
    token = ''
    for line in out.splitlines():
        if line.startswith('password='):
            token = line.split('=', 1)[1]
            break
    if not token:
        raise RuntimeError("Failed to retrieve GitHub credential from git credential manager.")
    return token

def create_opener():
    # Use local proxy if available, fallback to direct
    try:
        import socket
        s = socket.socket()
        s.settimeout(1)
        s.connect(('127.0.0.1', 20808))
        s.close()
        proxy = urllib.request.ProxyHandler({'http': 'http://127.0.0.1:20808', 'https': 'http://127.0.0.1:20808'})
        return urllib.request.build_opener(proxy)
    except Exception:
        return urllib.request.build_opener()

def main():
    repo = "santian666/santian-singbox"
    version_file = os.path.join(os.path.dirname(__file__), "..", "data", "build-version.json")
    with open(version_file, "r", encoding="utf-8-sig") as f:
        ver_data = json.load(f)
    ver = ver_data.get("version", "").strip()
    if not ver:
        raise ValueError("Invalid version in build-version.json")
    tag = f"v{ver}" if not ver.startswith("v") else ver

    token = get_github_token()
    opener = create_opener()

    release_payload = {
        "tag_name": tag,
        "target_commitish": "main",
        "name": f"SanTian-Singbox {tag}",
        "body": (
            f"## SanTian-Singbox {tag}\n\n"
            "- 界面运行设置全面显示【客户端版本 · 内核版本】，版本状态与跳变一目了然\n"
            "- 全新原子级纯 Go 进程内热替换引擎（重命名 .old -> 原生直拷新文件 -> Win32 原生平滑拉起 -> 旧进程优雅退出）\n"
            "- 彻底废除 .bat / cmd 脚本，100% 杜绝 Windows Internet 安全设置拦截与杀软启发式误报\n"
            "- 支持【全部升级】一键联检软件客户端与内核更新\n"
            "- 启动时自动清理历史旧文件残余，用户节点配置 100% 完整保留"
        ),
        "draft": False,
        "prerelease": False
    }

    print(f"Creating release {tag} for {repo}...")
    req = urllib.request.Request(
        f"https://api.github.com/repos/{repo}/releases",
        data=json.dumps(release_payload).encode("utf-8"),
        headers={
            "Authorization": f"token {token}",
            "User-Agent": "SanTian-Singbox-Uploader",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json"
        },
        method="POST"
    )

    rel = None
    try:
        with opener.open(req) as resp:
            rel = json.loads(resp.read().decode("utf-8"))
            print(f"Release created successfully with ID: {rel['id']}")
    except urllib.error.HTTPError as e:
        if e.code == 422:
            print(f"Release {tag} already exists, fetching existing release...")
            req_get = urllib.request.Request(
                f"https://api.github.com/repos/{repo}/releases/tags/{tag}",
                headers={
                    "Authorization": f"token {token}",
                    "User-Agent": "SanTian-Singbox-Uploader",
                    "Accept": "application/vnd.github+json"
                }
            )
            with opener.open(req_get) as resp_get:
                rel = json.loads(resp_get.read().decode("utf-8"))
                print(f"Fetched existing release ID: {rel['id']}")
        else:
            body = e.read().decode("utf-8", errors="replace")
            print(f"HTTP Error {e.code}: {body}")
            sys.exit(1)

    upload_url = rel["upload_url"].split("{")[0]
    assets = rel.get("assets", [])

    # Upload files from dist
    dist_dir = os.path.join(os.path.dirname(__file__), "..", "dist")
    files_to_upload = [
        "SanTian-Singbox.zip",
        "SanTian-Singbox-windows-amd64.zip"
    ]

    for fname in files_to_upload:
        fpath = os.path.join(dist_dir, fname)
        if not os.path.exists(fpath):
            print(f"Warning: File {fpath} does not exist, skipping.")
            continue

        # Check if already uploaded and delete old asset
        for asset in assets:
            if asset.get("name") == fname:
                del_url = asset.get("url")
                print(f"Deleting existing asset {fname} (ID {asset.get('id')})...")
                del_req = urllib.request.Request(
                    del_url,
                    headers={
                        "Authorization": f"token {token}",
                        "User-Agent": "SanTian-Singbox-Uploader",
                        "Accept": "application/vnd.github+json"
                    },
                    method="DELETE"
                )
                with opener.open(del_req) as del_resp:
                    del_resp.read()

        size = os.path.getsize(fpath)
        print(f"Uploading {fname} ({size / (1024*1024):.2f} MB)...")
        with open(fpath, "rb") as af:
            file_data = af.read()

        up_req = urllib.request.Request(
            f"{upload_url}?name={fname}",
            data=file_data,
            headers={
                "Authorization": f"token {token}",
                "User-Agent": "SanTian-Singbox-Uploader",
                "Accept": "application/vnd.github+json",
                "Content-Type": "application/zip",
                "Content-Length": str(size)
            },
            method="POST"
        )
        with opener.open(up_req) as up_resp:
            res = json.loads(up_resp.read().decode("utf-8"))
            print(f"Uploaded {fname} -> {res.get('browser_download_url')}")

    print(f"\nAll assets successfully published to GitHub Release {tag}!")

if __name__ == "__main__":
    main()

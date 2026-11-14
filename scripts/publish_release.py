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
            "- 直连 DNS 跟随当前网络：不再固定使用国内 DNS，程序自动读取当前物理网卡的 DNS；读取不到时从 223.5.5.5 / 119.29.29.29 / 1.1.1.1 / 8.8.8.8 / 9.9.9.9 中测速选择。海外用户直连访问更快，TUN 模式下也不会产生 DNS 回环\n"
            "- 换网络自动恢复：切换 Wi-Fi / 手机热点或休眠唤醒后，如果原直连 DNS 已不可用，会自动切换并重启内核；网络稳定时不会重启，每小时最多自动切换 3 次\n"
            "- Kill Switch 修复：开启前保存原始防火墙策略并在关闭时恢复；程序崩溃后再次启动会自动解除残留阻断；TUN 模式下由 strict_route 防泄露，不再与防火墙阻断冲突；不再放行所有 UDP 53\n"
            "- 批量测试重做：结果逐个实时显示，延迟和出口 IP 一次测完；停止立即生效并关闭临时内核；配置无效的节点自动跳过，不再拖慢整批；测试过程中滚动表格不再跳回顶部\n"
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

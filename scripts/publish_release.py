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
            "- 真实 IP 防泄露与断网急停 (Kill Switch)：基于 Windows 高级防火墙规则实现底层零穿透防护，智能熔断激活、内核异常或退出时默认阻断非代理直连出站，彻底堵死真实 IP 泄露窗口\n"
            "- DNS 解析回环根治：将 default_domain_resolver 从 local 升级为 china 国内权威直连 DNS，彻底切断 Windows Dnscache 在 TUN 模式下反复分配 FakeIP 的递归死循环\n"
            "- WebRTC STUN 泄露阻断：TUN 路由规则对 UDP 3478, 3479, 5349, 19302, 19305 等 STUN 端口强制拦截，杜绝指纹浏览器通过 WebRTC 探针暴露真实出口\n"
            "- 分应用代理防漏与 TUN 解耦共存：添加应用时自动正则通配其目录及子目录下的所有内核进程（如 chrome.exe、msedgewebview2.exe），并支持 TUN 底层接管与指定应用强制代理分流无缝共存\n"
            "- 故障转移锁区防风控：限制自动切换只能在同国家/同地区节点中优选，坚决禁止跨国漂移导致指纹环境或 AI 账号封号；ProxyMap 映射节点禁止自动切换打断\n"
            "- TUN 容错优化与驱动卸载平滑化：彻底剔除 pnputil /remove-device 物理注销设备驱动，TUN 健康检查引入平滑重试缓冲；内核终止采用先温和通知后兜底强杀\n"
            "- 规则模式精度提升：移除 jd、qq 等过宽关键词对海外域名（如 jdk.java.net）的误杀，Windows NCSI 联网探针与时间同步直连国内 DNS，消除系统误报断网\n"
            "- 开机自启动增强：注册表启动命令自动携带 --restart-proxy 参数，确保开机自启无缝恢复代理"
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

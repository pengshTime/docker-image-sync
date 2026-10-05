#!/usr/bin/env python3
"""本地管理 images.txt 并触发/查看 GitHub Actions 同步。

用法:
    python scripts/images.py pull              # 远端列表 -> 本地
    python scripts/images.py push              # 本地 -> 远端并触发同步，等待结果
    python scripts/images.py push --no-run     # 只提交列表，不触发同步
    python scripts/images.py status            # 最近一次运行的结果
    python scripts/images.py watch [run_id]    # 等待某次运行结束并打印摘要

凭据复用 git 已登录的 GitHub 账号（git credential），也可用 --token 或环境变量
GITHUB_TOKEN / GH_TOKEN 覆盖。只依赖标准库。
"""

from __future__ import annotations

import argparse
import base64
import io
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

API = "https://api.github.com"
WORKFLOW = "sync.yaml"
DEFAULT_LIST = "images.txt"
POLL_INTERVAL = 10
MAX_WAIT = 3600


def out(*args):
    print(*args, flush=True)


def die(message: str) -> "None":
    sys.stderr.write(f"错误: {message}\n")
    raise SystemExit(1)


# ---------------------------------------------------------------- 仓库与凭据

def run_git(*args: str) -> str:
    result = subprocess.run(
        ["git", *args], capture_output=True, text=True,
        env={**os.environ, "GIT_TERMINAL_PROMPT": "0"},
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or f"git {' '.join(args)} 失败")
    return result.stdout.strip()


def detect_repo() -> tuple[str, str]:
    """从 origin 远程地址解析 owner/name，省去手写仓库名。"""
    try:
        url = run_git("remote", "get-url", "origin")
    except (RuntimeError, FileNotFoundError) as exc:
        die(f"无法读取 git origin，请在仓库内运行或设置环境变量 GIT_REPO=owner/name: {exc}")
    match = re.search(r"[/:]([^/:]+)/([^/:]+?)(?:\.git)?/?$", url)
    if not match:
        die(f"无法从 origin 地址解析仓库: {url}")
    return match.group(1), match.group(2)


def detect_branch() -> str:
    override = os.environ.get("GIT_BRANCH")
    if override:
        return override
    try:
        return run_git("rev-parse", "--abbrev-ref", "HEAD")
    except (RuntimeError, FileNotFoundError):
        return "main"


def get_token(explicit: str | None) -> str:
    for candidate in (explicit, os.environ.get("GITHUB_TOKEN"), os.environ.get("GH_TOKEN")):
        if candidate:
            return candidate
    # 复用 git 已存的 GitHub 凭据，避免另配 PAT
    result = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True,
        env={**os.environ, "GIT_TERMINAL_PROMPT": "0"},
    )
    if result.returncode == 0:
        for line in result.stdout.splitlines():
            if line.startswith("password="):
                return line[len("password="):]
    die("未找到 GitHub 凭据。请先用 git 登录该账号，或传入 --token / 设置 GITHUB_TOKEN。")


# ---------------------------------------------------------------- API

def request(method: str, path: str, token: str, body=None, accept="application/vnd.github+json"):
    url = path if path.startswith("http") else f"{API}{path}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Accept", accept)
    req.add_header("User-Agent", "docker-image-sync-local")
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=60) as response:
            payload = response.read()
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", "replace")
        try:
            detail = json.loads(detail).get("message", detail)
        except json.JSONDecodeError:
            pass
        if exc.code == 403 and "workflow" in detail.lower():
            detail += "（凭据缺少 workflow 权限，无法触发 Actions；可在 https://github.com/settings/tokens 换一个带 workflow scope 的 PAT）"
        if exc.code == 404 and "/actions/workflows/" in url:
            detail += f"（{WORKFLOW} 可能还没推送到目标分支）"
        die(f"{method} {url} -> HTTP {exc.code}: {detail}")
    except urllib.error.URLError as exc:
        die(f"{method} {url} 请求失败: {exc.reason}")
    if not payload:
        return None
    return json.loads(payload)


def fetch_list(owner: str, repo: str, branch: str, path: str, token: str):
    response = request(
        "GET",
        f"/repos/{owner}/{repo}/contents/{urllib.parse.quote(path)}?ref={urllib.parse.quote(branch)}",
        token,
    )
    if isinstance(response, dict) and "content" in response:
        return base64.b64decode(response["content"]).decode("utf-8"), response["sha"]
    return None, None


def commit_list(owner, repo, branch, path, content, token, message) -> str | None:
    """提交列表文件，返回 commit sha；内容未变化时返回 None。"""
    remote, sha = fetch_list(owner, repo, branch, path, token)
    if remote == content:
        out("列表内容与远端一致，无需提交。")
        return None
    response = request(
        "PUT",
        f"/repos/{owner}/{repo}/contents/{urllib.parse.quote(path)}",
        token,
        body={
            "message": message,
            "content": base64.b64encode(content.encode("utf-8")).decode(),
            "sha": sha,
            "branch": branch,
        },
    )
    return response["commit"]["sha"]


def dispatch(owner, repo, branch, path, token) -> None:
    request(
        "POST",
        f"/repos/{owner}/{repo}/actions/workflows/{WORKFLOW}/dispatches",
        token,
        body={"ref": branch, "inputs": {"image_list_file": path}},
    )
    out(f"已触发 {WORKFLOW}（ref={branch}）。")


def latest_runs(owner, repo, token, limit=10):
    response = request(
        "GET",
        f"/repos/{owner}/{repo}/actions/workflows/{WORKFLOW}/runs?per_page={limit}",
        token,
    )
    return response.get("workflow_runs", []) if response else []


def find_run(owner, repo, token, sha) -> dict | None:
    """找到由指定 commit 触发的运行；找不到则退回最近一次运行。"""
    deadline = time.time() + 60
    newest = None
    while time.time() < deadline:
        runs = latest_runs(owner, repo, token)
        if runs:
            newest = runs[0]
            for run in runs:
                if run["head_sha"] == sha:
                    return run
            if newest.get("event") == "workflow_dispatch" and newest.get("status") != "completed":
                return newest
        if not sha:
            return newest
        time.sleep(2)
    return newest


def results_from_log(text: str) -> list[str]:
    """从 Actions 日志里取出每个镜像的同步结果行。"""
    return [
        match.group(0)
        for line in text.splitlines()
        for match in [re.search(r"\[(SUCCESS|SKIP|FAIL)\].*", line)]
        if match
    ]


def print_run(owner, repo, run: dict, token: str, show_log: bool) -> None:
    status = run.get("status")
    conclusion = run.get("conclusion")
    out(f"Run #{run['run_number']}  {run.get('name')}")
    out(f"  状态: {status} / {conclusion or '-'}")
    out(f"  开始: {run.get('created_at')}  链接: {run.get('html_url')}")
    if show_log and conclusion:
        lines = results_from_log(fetch_job_log(owner, repo, run["id"], token))
        if lines:
            out("  同步明细:")
            for line in lines:
                out(f"    {line}")
        if conclusion != "success":
            out(f"  失败原因见日志: {run.get('html_url')}")


def fetch_job_log(owner, repo, run_id, token) -> str:
    """下载 run 日志（zip 里的每个 job 一个 .txt），用于还原同步明细。"""
    url = f"{API}/repos/{owner}/{repo}/actions/runs/{run_id}/logs"
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("User-Agent", "docker-image-sync-local")
    try:
        with urllib.request.urlopen(req, timeout=120) as response:
            blob = response.read()
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            return ""  # 日志被旧的 run 清理步骤删掉了
        die(f"下载日志失败: HTTP {exc.code}")
    with zipfile.ZipFile(io.BytesIO(blob)) as archive:
        for name in sorted(archive.namelist()):
            if name.endswith(".txt"):
                return archive.read(name).decode("utf-8", "replace")
    return ""


def wait_for_run(owner, repo, run, token, timeout=MAX_WAIT) -> dict:
    run_id = run["id"]
    out(f"等待 Run #{run['run_number']} 结束（每 {POLL_INTERVAL}s 轮询，超时 {timeout}s）...")
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        run = request("GET", f"/repos/{owner}/{repo}/actions/runs/{run_id}", token)
        if run["status"] != last:
            out(f"  [{time.strftime('%H:%M:%S')}] {run['status']}")
            last = run["status"]
        if run["status"] == "completed":
            return run
        time.sleep(POLL_INTERVAL)
    die(f"等待超时（{timeout}s），请稍后运行: python scripts/images.py status")


# ---------------------------------------------------------------- 本地文件

def local_path(path: str) -> str:
    if os.path.isabs(path) or os.path.exists(path):
        return path
    try:
        root = run_git("rev-parse", "--show-toplevel")
    except (RuntimeError, FileNotFoundError):
        return path
    candidate = os.path.join(root, path)
    return candidate if os.path.exists(candidate) else path


def parse_sections(content: str) -> dict[str, list[str]]:
    """按 [provider] 分组解析列表，与 Go 端 image.LoadFromFile 的口径一致。"""
    sections: dict[str, list[str]] = {}
    current = "default"
    for raw in content.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("[") and line.endswith("]"):
            current = line[1:-1].strip().lower()
            sections.setdefault(current, [])
            continue
        sections.setdefault(current, []).append(line)
    return sections


def read_local(path: str) -> str:
    try:
        with open(path, encoding="utf-8") as handle:
            return handle.read()
    except FileNotFoundError:
        die(f"本地找不到 {path}，先执行: python scripts/images.py pull")


# ---------------------------------------------------------------- 命令

def cmd_pull(args, owner, repo, branch, token):
    content, sha = fetch_list(owner, repo, branch, args.list, token)
    if content is None:
        die(f"远端 {branch} 分支上没有 {args.list}")
    with open(args.local_list, "w", encoding="utf-8", newline="\n") as handle:
        handle.write(content)
    out(f"已拉取 {owner}/{repo}:{args.list} -> {args.local_list}（{len(content.splitlines())} 行, sha={sha[:8]}）")
    for section, images in parse_sections(content).items():
        out(f"  [{section}] {len(images)} 个: {', '.join(images) or '无'}")


def cmd_push(args, owner, repo, branch, token):
    content = read_local(args.local_list)
    sha = commit_list(owner, repo, branch, args.list, content, token, args.message or f"Update {args.list}")
    if sha is None:
        if not args.no_run:
            run = latest_runs(owner, repo, token)[:1]
            if run:
                print_run(owner, repo, run[0], token, show_log=False)
                out("  （内容与远端相同，未产生新提交，上面是最近一次运行）")
        return
    out(f"已提交 {owner}/{repo}:{args.list} @ {sha[:8]}")
    if args.no_run:
        out("按 --no-run 跳过触发同步。")
        return
    dispatch(owner, repo, branch, args.list, token)
    run = find_run(owner, repo, token, sha)
    if not run:
        die("未能定位触发的运行，请到 Actions 页面查看。")
    finished = wait_for_run(owner, repo, run, token)
    print_run(owner, repo, finished, token, show_log=True)
    raise SystemExit(0 if finished["conclusion"] == "success" else 1)


def cmd_status(args, owner, repo, branch, token):
    if args.run_id:
        print_run(owner, repo, request("GET", f"/repos/{owner}/{repo}/actions/runs/{args.run_id}", token),
                  token, show_log=True)
        return
    runs = latest_runs(owner, repo, token, limit=args.limit)
    if not runs:
        out("没有运行记录（历史运行会被 workflow 的清理步骤删除）。")
        return
    for run in runs:
        print_run(owner, repo, run, token, show_log=False)
    out(f"查看明细: python scripts/images.py status {runs[0]['id']}")


def cmd_watch(args, owner, repo, branch, token):
    run = {"id": int(args.run_id)} if args.run_id else find_run(owner, repo, token, None)
    if not run:
        die("没有可等待的运行。")
    run = request("GET", f"/repos/{owner}/{repo}/actions/runs/{run['id']}", token)
    if run["status"] != "completed":
        run = wait_for_run(owner, repo, run, token)
    print_run(owner, repo, run, token, show_log=True)
    raise SystemExit(0 if run["conclusion"] == "success" else 1)


def main(argv=None):
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")

    parser = argparse.ArgumentParser(description="管理 docker-image-sync 的镜像列表并触发同步")
    parser.add_argument("command", nargs="?", choices=["pull", "push", "status", "watch"], default="pull")
    parser.add_argument("--list", default=DEFAULT_LIST, help=f"列表文件名，默认 {DEFAULT_LIST}")
    parser.add_argument("--repo", help="owner/name，默认从 git origin 解析")
    parser.add_argument("--ref", help="分支，默认当前分支")
    parser.add_argument("--token", help="GitHub token，默认复用 git 凭据或环境变量")
    parser.add_argument("-m", "--message", help="提交说明，默认 Update images.txt")
    parser.add_argument("--no-run", action="store_true", help="push 时只提交不触发同步")
    parser.add_argument("--limit", type=int, default=5, help="status 显示的运行条数")
    parser.add_argument("run_id", nargs="?", help="status/watch 指定的 run id")
    args = parser.parse_args(argv)

    args.local_list = local_path(args.list)
    owner, repo = (args.repo.split("/", 1) if args.repo else detect_repo())
    branch = args.ref or detect_branch()
    token = get_token(args.token)

    handlers = {"pull": cmd_pull, "push": cmd_push, "status": cmd_status, "watch": cmd_watch}
    handlers[args.command](args, owner, repo, branch, token)


if __name__ == "__main__":
    main()

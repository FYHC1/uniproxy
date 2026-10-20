"""Exercise only owned hidden test consoles, never the user's console."""
import ctypes
from ctypes import wintypes
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

if sys.platform != "win32":
    raise SystemExit("This test requires Windows; no commands were started")
if len(sys.argv) != 3:
    raise SystemExit("Usage: python scripts/test-windows-console.py <uniproxy.exe> <mihomo.exe>")

kernel = ctypes.WinDLL("kernel32", use_last_error=True)
user = ctypes.WinDLL("user32", use_last_error=True)
kernel.FreeConsole.argtypes = []
kernel.FreeConsole.restype = wintypes.BOOL
kernel.AttachConsole.argtypes = [wintypes.DWORD]
kernel.AttachConsole.restype = wintypes.BOOL
kernel.GetConsoleWindow.argtypes = []
kernel.GetConsoleWindow.restype = wintypes.HWND
kernel.GetConsoleProcessList.argtypes = [ctypes.POINTER(wintypes.DWORD), wintypes.DWORD]
kernel.GetConsoleProcessList.restype = wintypes.DWORD
user.PostMessageW.argtypes = [wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM]
user.PostMessageW.restype = wintypes.BOOL

def close_owned_console(child):
    # FreeConsole detaches this disposable Python test process only.
    kernel.FreeConsole()
    if not kernel.AttachConsole(child.pid):
        raise ctypes.WinError(ctypes.get_last_error())
    try:
        processes = (wintypes.DWORD * 64)()
        count = kernel.GetConsoleProcessList(processes, 64)
        if count < 2 or count > 64 or child.pid not in processes[:count] or os.getpid() not in processes[:count]:
            raise RuntimeError("Not the owned test console; refusing close")
        hwnd = kernel.GetConsoleWindow()
        if not hwnd:
            raise RuntimeError("Owned test console has no window")
    finally:
        kernel.FreeConsole()
    if not user.PostMessageW(hwnd, 0x0010, 0, 0):
        raise ctypes.WinError(ctypes.get_last_error())

app = str(Path(sys.argv[1]).resolve(strict=True))
core = str(Path(sys.argv[2]).resolve(strict=True))
root = Path(tempfile.mkdtemp(prefix="uniproxy-console-test-"))
env = {key: value for key, value in os.environ.items() if not key.upper().startswith("UNIPROXY_")}
env.update({"UNIPROXY_CORE_PATH": core, "UNIPROXY_DATA_DIR": str(root / "data"), "LOCALAPPDATA": str(root / "cache")})
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
env["UNIPROXY_PORT"] = str(port)
startup = subprocess.STARTUPINFO()
startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
startup.wShowWindow = 0
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
results = []
for round_number in range(2):
    child = subprocess.Popen([app], cwd=root, env=env, creationflags=subprocess.CREATE_NEW_CONSOLE, startupinfo=startup)
    try:
        deadline = time.monotonic() + 30
        while True:
            if child.poll() is not None:
                raise RuntimeError("Owned test app exited before ready")
            try:
                with opener.open(f"http://127.0.0.1:{port}/api/bootstrap", timeout=1) as response:
                    boot = json.load(response)
                if boot["runtime"]["running"] and not boot["runtime"]["dirty"]:
                    break
            except (OSError, ValueError):
                pass
            if time.monotonic() >= deadline:
                raise RuntimeError("Owned test app not ready")
            time.sleep(0.05)
        close_owned_console(child)
        code = child.wait(timeout=10)
        if code != 0:
            raise RuntimeError(f"Owned console close was not graceful: {code}")
        results.append({"case": "actual-console-WM_CLOSE", "round": round_number + 1, "exit": code, "running_before_close": True})
    finally:
        if child.poll() is None:
            child.kill()
            child.wait(timeout=10)

# An invalid no-argument Explorer-style launch must retain its error console.
invalid = dict(env)
invalid["UNIPROXY_HOST"] = "invalid-host-for-test"
child = subprocess.Popen([app], cwd=root, env=invalid, creationflags=subprocess.CREATE_NEW_CONSOLE, startupinfo=startup)
try:
    log_dir = root / "cache" / "UniProxy" / "logs"
    deadline = time.monotonic() + 10
    while not log_dir.exists() or not list(log_dir.glob("startup-*.log")):
        if child.poll() is not None:
            raise RuntimeError("Failed Explorer-style launch flashed and exited")
        if time.monotonic() >= deadline:
            raise RuntimeError("Failure report not saved")
        time.sleep(0.05)
    time.sleep(0.2)
    if child.poll() is not None:
        raise RuntimeError("Failure window did not remain open")
    close_owned_console(child)
    code = child.wait(timeout=10)
    if code != 1:
        raise RuntimeError(f"Failure console did not preserve error status: {code}")
    results.append({"case": "standalone-error-console-retained", "exit_after_close": code, "report_saved": True})
finally:
    if child.poll() is None:
        child.kill()
        child.wait(timeout=10)
print(json.dumps({"root": str(root), "results": results}, indent=2))

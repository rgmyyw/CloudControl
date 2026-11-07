# tools — Android 13 兼容组件

CloudControl 原生依赖的 minicap/minitouch 二进制在 Android 10+（尤其 MIUI 的 SELinux 策略）上无法工作。
本目录提供两个 Go 编写的兼容替代（shim），运行在手机上，由 atx-agent 按原协议调用，前端零改动。

## 组件

| 目录 | 作用 | 部署位置 |
|---|---|---|
| `minicap-shim/` | 投屏：uia2 截图 → minicap 帧协议（`@minicap` 套接字 + `-i`/`-s` 模式） | 手机 `/data/local/tmp/minicap` |
| `minitouch-shim/` | 触控：minitouch 命令 → uia2 `injectInputEvent` 流式注入（`@minitouch` 套接字） | 手机 `/data/local/tmp/minitouch` |
| `device-setup.sh` | 手机重启后一键恢复 atx-agent 与 u2 服务 | Mac 上执行 |

前置依赖（推送一次，重启不丢）：手机上的 `atx-agent 0.5.4` 与 `u2.jar`
（uiautomator2 3.x wheel 内置，以 `app_process` 方式常驻 9008 端口）。

## 编译（Mac 上，需 Go ≥1.21）

```bash
cd tools/minicap-shim
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o minicap-shim-arm64 .

cd ../minitouch-shim
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o minitouch-shim-arm64 .
```

编译产物不入库（已在 .gitignore），推送到手机：

```bash
adb push minicap-shim/minicap-shim-arm64  /data/local/tmp/minicap
adb push minitouch-shim/minitouch-shim-arm64 /data/local/tmp/minitouch
adb shell chmod 755 /data/local/tmp/minicap /data/local/tmp/minitouch
```

## 运行参数

- `MINICAP_SHIM_SCALE`（默认 0.5）、`MINICAP_SHIM_QUALITY`（默认 60）：投屏画质与流畅度取舍
- 静止画面自动跳帧；触控为逐事件流式注入（~15ms/事件），网页操作可跟手

## 恢复

手机重启后：`bash tools/device-setup.sh [serial] [server-ip:port]`

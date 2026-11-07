#!/system/bin/sh
# CloudControl 手机端服务一键恢复（手机重启后执行）
# 用法: bash tools/device-setup.sh   （在 Mac 上执行，需已连接 adb）
# 依赖（一次性安装，重启不丢）:
#   /data/local/tmp/minicap      -> minicap-shim（tools/minicap-shim 交叉编译）
#   /data/local/tmp/minitouch    -> minitouch-shim（tools/minitouch-shim 交叉编译）
#   /data/local/tmp/u2.jar       -> uiautomator2 3.x wheel 内置
#   /data/local/tmp/atx-agent    -> openatx atx-agent 0.5.4
#   com.github.uiautomator(+test) apk（可缺，u2.jar 路线不依赖）

SERIAL="${1:-9bddb250}"
SERVER="${2:-$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1):8000}"

adb -s "$SERIAL" shell "nohup /data/local/tmp/atx-agent server -d -t $SERVER >/data/local/tmp/atx-agent.log 2>&1 &"
adb -s "$SERIAL" shell "nohup env CLASSPATH=/data/local/tmp/u2.jar app_process / com.wetest.uia2.Main -p 9008 >/data/local/tmp/u2.log 2>&1 &"
sleep 3
echo "atx-agent: $(adb -s "$SERIAL" shell 'cat /data/local/tmp/atx-agent.log' | tail -1)"
curl -s --max-time 5 -X POST "http://127.0.0.1:$(adb -s "$SERIAL" shell 'cat /proc/net/tcp6 >/dev/null && echo 19008')/jsonrpc/0" >/dev/null 2>&1 || true
adb -s "$SERIAL" forward tcp:19008 tcp:9008
curl -s --max-time 5 -X POST "http://127.0.0.1:19008/jsonrpc/0" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"deviceInfo","params":[]}' | head -c 120
echo
echo "OK: minicap/minitouch shim 由 atx-agent 在首次投屏/触控时自动拉起"

// minitouch-shim: atx-agent 0.5.4 兼容的 minitouch 替代实现。
//
// 背景：Android 13 (MIUI) 的 SELinux 拒绝 shell 用户读写 /dev/input/*，
// 原生 minitouch 无法工作；MIUI 同时静默丢弃 `adb shell input` 注入。
// 可用的注入通道是 uia2 服务（com.wetest.uia2.Main，127.0.0.1:9008）的
// JSON-RPC injectInputEvent（UiAutomation 通道），逐事件延迟 ~15ms，可实时跟手。
//
// 兼容 atx-agent 的调用方式：
//   - 服务 `/data/local/tmp/minitouch`（无参数）启动后监听抽象 unix 套接字 @minitouch
//   - banner 三行（atx-agent 逐行 Scanf 解析）：
//     "^ 1"                    协议版本
//     "^ 10 1080 2400 255"     maxContacts maxX maxY maxPressure
//     "$ <pid>"                进程号
//   - 命令行（atx-agent 从 WebSocket JSON 翻译后写入）：
//     "d <i> <x> <y> <pressure>" / "m ..." / "u <i>" / "c" / "w <ms>"
//
// 事件映射（uia2 ACTION_DOWN=0 / ACTION_UP=1 / ACTION_MOVE=2）：
//
//	d -> injectInputEvent(0, x, y, 0)
//	m -> injectInputEvent(2, x, y, 0)   // 实时逐点注入，跟手
//	u -> injectInputEvent(1, 上次坐标, 0) // atx-agent 的 u 只带 index，坐标取最近一次 d/m
//	c -> 忽略（提交标记，uia2 注入即生效）
//	w -> sleep
//
// 长按与拖拽不需要特殊处理：按下到抬起的时长由前端事件流自然决定。
// MOVE 事件做合并（coalescing）：mousemove 风暴时只注入最新坐标，防止积压。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	rpcURL     = "http://127.0.0.1:9008/jsonrpc/0"
	socketName = "@minitouch"
)

const (
	actionDown = 0
	actionUp   = 1
	actionMove = 2
)

type rpcResponse struct {
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var rpcClient = &http.Client{Timeout: 10 * time.Second}

func inject(action, x, y int) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "injectInputEvent",
		"params": []interface{}{action, x, y, 0},
	})
	resp, err := rpcClient.Post(rpcURL, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var out rpcResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	if out.Error != nil {
		return fmt.Errorf("inject %d: %s", action, out.Error.Message)
	}
	return nil
}

func queryDisplay() (int, int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "deviceInfo", "params": []interface{}{},
	})
	resp, err := rpcClient.Post(rpcURL, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return 1080, 2400
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Result struct {
			DisplayWidth  int `json:"displayWidth"`
			DisplayHeight int `json:"displayHeight"`
		} `json:"result"`
	}
	if json.Unmarshal(body, &out) != nil {
		return 1080, 2400
	}
	return out.Result.DisplayWidth, out.Result.DisplayHeight
}

type pt struct{ x, y int }

type touchEvent struct {
	op    string // d / m / u / w
	index int
	x, y  int
	ms    int
}

// touchSession: 单个 @minitouch 连接的事件处理。
// MOVE 合并：latest 保存每指最新坐标，队列里只放一个 "待注入" 标记；
// DOWN/UP 必须按序处理，故不合并。
type touchSession struct {
	mu     sync.Mutex
	latest map[int]pt
	events chan touchEvent
}

func newTouchSession() *touchSession {
	return &touchSession{
		latest: map[int]pt{},
		events: make(chan touchEvent, 64),
	}
}

func (s *touchSession) run() {
	active := map[int]bool{} // worker 侧的按下手指标记
	for ev := range s.events {
		switch ev.op {
		case "d":
			active[ev.index] = true
			inject(actionDown, ev.x, ev.y)
		case "m":
			if !active[ev.index] {
				continue
			}
			s.mu.Lock()
			p := s.latest[ev.index]
			s.mu.Unlock()
			inject(actionMove, p.x, p.y)
		case "u":
			if !active[ev.index] {
				continue
			}
			delete(active, ev.index)
			s.mu.Lock()
			p, ok := s.latest[ev.index]
			delete(s.latest, ev.index)
			s.mu.Unlock()
			if ok {
				inject(actionUp, p.x, p.y)
			}
		case "w":
			time.Sleep(time.Duration(ev.ms) * time.Millisecond)
		}
	}
}

func (s *touchSession) handle(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	case "d", "m":
		if len(fields) < 4 {
			return
		}
		idx, _ := strconv.Atoi(fields[1])
		x, _ := strconv.Atoi(fields[2])
		y, _ := strconv.Atoi(fields[3])
		s.mu.Lock()
		s.latest[idx] = pt{x, y}
		s.mu.Unlock()
		if fields[0] == "d" {
			s.events <- touchEvent{op: "d", index: idx, x: x, y: y}
			return
		}
		// m：队列里已有待注入标记则跳过，worker 会取最新坐标
		select {
		case s.events <- touchEvent{op: "m", index: idx}:
		default:
		}
	case "u":
		if len(fields) < 2 {
			return
		}
		idx, _ := strconv.Atoi(fields[1])
		s.events <- touchEvent{op: "u", index: idx}
	case "w":
		if len(fields) < 2 {
			return
		}
		ms, _ := strconv.Atoi(fields[1])
		s.events <- touchEvent{op: "w", ms: ms}
	}
	// "c" 提交标记：uia2 注入即生效，无需处理
}

func handleConn(conn net.Conn, maxX, maxY int) {
	defer conn.Close()
	fmt.Fprintf(conn, "^ 1\n")
	fmt.Fprintf(conn, "^ 10 %d %d 255\n", maxX, maxY)
	fmt.Fprintf(conn, "$ %d\n", os.Getpid())

	s := newTouchSession()
	go s.run()
	defer close(s.events)

	rd := bufio.NewReader(conn)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		s.handle(strings.TrimSpace(line))
	}
}

func main() {
	maxX, maxY := queryDisplay()
	ln, err := net.Listen("unix", socketName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "minitouch-shim: listen:", err)
		os.Exit(1)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "minitouch-shim: accept:", err)
			os.Exit(1)
		}
		go handleConn(conn, maxX, maxY)
	}
}

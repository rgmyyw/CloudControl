// minicap-shim: atx-agent 0.5.4 兼容的 minicap 替代实现（Android 10+ 无预编译 minicap 时的方案）。
//
// 兼容 atx-agent 对 /data/local/tmp/minicap 的三种调用：
//  1. 流模式  `minicap -S -P WxH@VWxVH/ROT`：监听抽象 unix 套接字 @minicap，
//     按 stf minicap 协议输出 24 字节 banner + uint32(LE) 长度前缀的 JPEG 帧
//  2. `minicap -i`：stdout 输出 JSON 设备信息（MinicapInfo）
//  3. `minicap -P WxH@VWxVH/ROT -s`：输出单帧 JPEG 到 stdout 后退出
//
// 帧来源为本机 uia2 服务（com.wetest.uia2.Main，127.0.0.1:9008）的
// JSON-RPC takeScreenshot，无原生 minicap 依赖。
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
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
	rpcURL      = "http://127.0.0.1:9008/jsonrpc/0"
	socketName  = "@minicap"
	frameMinGap = 30 * time.Millisecond // 截图本身 ~70ms，此间隔仅防紧密空转
)

type minicapInfo struct {
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Rotation int     `json:"rotation"`
	Density  float32 `json:"density"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type deviceInfo struct {
	DisplayRotation int `json:"displayRotation"`
	DisplayWidth    int `json:"displayWidth"`
	DisplayHeight   int `json:"displayHeight"`
}

var rpcClient = &http.Client{Timeout: 5 * time.Second}

func rpcCall(method string, params []interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	resp, err := rpcClient.Post(rpcURL, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out rpcResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc %s: %s", method, out.Error.Message)
	}
	return out.Result, nil
}

// takeScreenshot 返回当前屏幕 JPEG 原始字节；scale<1 时由 uia2 服务端缩放
func takeScreenshot(scale float64, quality int) ([]byte, error) {
	result, err := rpcCall("takeScreenshot", []interface{}{scale, quality})
	if err != nil {
		return nil, err
	}
	var b64 string
	if err := json.Unmarshal(result, &b64); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(b64)
}

func queryInfo() (minicapInfo, error) {
	result, err := rpcCall("deviceInfo", []interface{}{})
	if err != nil {
		return minicapInfo{}, err
	}
	var d deviceInfo
	if err := json.Unmarshal(result, &d); err != nil {
		return minicapInfo{}, err
	}
	return minicapInfo{
		Width:    d.DisplayWidth,
		Height:   d.DisplayHeight,
		Rotation: d.DisplayRotation,
		Density:  440,
	}, nil
}

// parsePArg 解析 -P WxH@VWxVH/ROT（-P 总在参数中，缺失时回退 deviceInfo）
func parsePArg(args []string) (w, h, vw, vh, rot int, ok bool) {
	for i, a := range args {
		if a == "-P" && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%dx%d@%dx%d/%d", &w, &h, &vw, &vh, &rot)
			return w, h, vw, vh, rot, true
		}
	}
	return 0, 0, 0, 0, 0, false
}

func bannerBytes(info minicapInfo) []byte {
	b := make([]byte, 24)
	b[0] = 1  // version
	b[1] = 24 // header size
	binary.LittleEndian.PutUint32(b[2:6], uint32(os.Getpid()))
	binary.LittleEndian.PutUint32(b[6:10], uint32(info.Width))
	binary.LittleEndian.PutUint32(b[10:14], uint32(info.Height))
	binary.LittleEndian.PutUint32(b[14:18], uint32(info.Width))
	binary.LittleEndian.PutUint32(b[18:22], uint32(info.Height))
	b[22] = byte(info.Rotation)
	b[23] = 0 // quirks
	return b
}

// hub: 单一截图生产者向所有已连接的 atx-agent 连接扇出帧
type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newHub() *hub { return &hub{subs: make(map[chan []byte]struct{})} }

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 2)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) broadcast(jpeg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- jpeg:
		default: // 订阅者消费慢则丢帧，保持实时性
		}
	}
}

func producerLoop(h *hub) {
	// 环境变量可调：MINICAP_SHIM_SCALE（0~1，默认 0.5 兼顾流畅与带宽）、
	// MINICAP_SHIM_QUALITY（JPEG 质量，默认 60）。画质换流畅度时降低 scale。
	scale := 0.5
	quality := 60
	if v, err := strconv.ParseFloat(os.Getenv("MINICAP_SHIM_SCALE"), 32); err == nil && v > 0 && v <= 1 {
		scale = v
	}
	if v, err := strconv.Atoi(os.Getenv("MINICAP_SHIM_QUALITY")); err == nil && v > 0 && v <= 100 {
		quality = v
	}

	var last []byte
	for {
		jpeg, err := takeScreenshot(scale, quality)
		if err == nil && len(jpeg) > 4 {
			// 静止画面跳过广播：省带宽，也让变化帧能立刻发出
			if !bytes.Equal(jpeg, last) {
				last = jpeg
				h.broadcast(jpeg)
			}
		} else if err != nil {
			time.Sleep(time.Second) // uia2 未就绪时退避
		}
		time.Sleep(frameMinGap)
	}
}

func handleConn(conn net.Conn, info minicapInfo, h *hub) {
	defer conn.Close()
	// atx-agent 只读不写；对端断开时读端先感知，用于回收连接
	go func() {
		io.Copy(io.Discard, conn)
		conn.Close()
	}()

	if _, err := conn.Write(bannerBytes(info)); err != nil {
		return
	}
	ch := h.subscribe()
	defer h.unsubscribe(ch)
	for jpeg := range ch {
		head := make([]byte, 4)
		binary.LittleEndian.PutUint32(head, uint32(len(jpeg)))
		if _, err := conn.Write(head); err != nil {
			return
		}
		if _, err := conn.Write(jpeg); err != nil {
			return
		}
	}
}

func streamMode(info minicapInfo, h *hub) error {
	ln, err := net.Listen("unix", socketName)
	if err != nil {
		return fmt.Errorf("listen %s: %v", socketName, err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(conn, info, h)
	}
}

func main() {
	args := os.Args[1:]
	hasFlag := func(f string) bool {
		for _, a := range args {
			if a == f {
				return true
			}
		}
		return false
	}

	info, err := queryInfo()
	if err != nil {
		fmt.Fprintln(os.Stderr, "minicap-shim: query deviceInfo:", err)
		os.Exit(1)
	}

	switch {
	case hasFlag("-i"):
		out, _ := json.Marshal(info)
		fmt.Println(string(out))
	case hasFlag("-s"):
		jpeg, err := takeScreenshot(1.0, 80)
		if err != nil {
			fmt.Fprintln(os.Stderr, "minicap-shim: screenshot:", err)
			os.Exit(1)
		}
		os.Stdout.Write(jpeg)
	default:
		if _, _, _, _, rot, ok := parsePArg(args); ok {
			info.Rotation = rot
		}
		h := newHub()
		go producerLoop(h)
		if err := streamMode(info, h); err != nil {
			fmt.Fprintln(os.Stderr, "minicap-shim:", err)
			os.Exit(1)
		}
	}
}

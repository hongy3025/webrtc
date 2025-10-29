// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
// 许可声明：该文件遵循 MIT 开源许可。

//go:build !js
// +build !js

// 构建约束：仅在非 WASM/非浏览器环境下构建（标准 Go 运行环境）。

// simple-datachannel is a simple datachannel demo that auto connects.
// 示例说明：一个最简的 DataChannel 演示，浏览器与 Go 服务端建立连接并互发文本消息。
package main

import (
	// 标准库：JSON 编解码、日志输出、HTTP 服务器
	"encoding/json"
	"fmt"
	"net/http"

	// Pion WebRTC：PeerConnection、DataChannel、ICE 相关类型
	"github.com/pion/webrtc/v4"
)

func main() {
	var pc *webrtc.PeerConnection
	// pc 保存当前会话的 PeerConnection 指针（由 /offer 初始化）。

	setupOfferHandler(&pc)     // 注册 /offer 路由：接收浏览器发来的 SDP Offer 并返回 Answer
	setupCandidateHandler(&pc) // 注册 /candidate 路由：接收浏览器发来的 ICE 候选（Trickle ICE）
	setupStaticHandler()       // 注册静态文件路由：返回 demo.html 页面

	fmt.Println("🚀 Signaling server started on http://localhost:8080")
	//nolint:gosec
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}

func setupOfferHandler(pc **webrtc.PeerConnection) {
	http.HandleFunc("/offer", func(responseWriter http.ResponseWriter, r *http.Request) {
		// 解析浏览器端发送的 SDP Offer（JSON）
		var offer webrtc.SessionDescription
		if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
			http.Error(responseWriter, err.Error(), http.StatusBadRequest)

			return
		}

		// 创建 PeerConnection，并配置常用兼容项
		var err error
		*pc, err = webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}}, // 使用 Google STUN 获取公网可达地址
			},
			BundlePolicy:  webrtc.BundlePolicyBalanced, // 使用平衡的 Bundle 策略（常见浏览器兼容）
			RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire, // 强制 RTCP 与 RTP 复用（减少端口占用）
		})
		if err != nil {
			http.Error(responseWriter, err.Error(), http.StatusInternalServerError)

			return
		}

		setupICECandidateHandler(*pc) // 注册 ICE 候选回调：调试输出
		setupDataChannelHandler(*pc)  // 注册 DataChannel 回调：收发文本消息

		// 处理 Offer：设置远端描述、创建与设置本端 Answer，并在 ICE 完成后返回给浏览器
		if err := processOffer(*pc, offer, responseWriter); err != nil {
			http.Error(responseWriter, err.Error(), http.StatusInternalServerError)

			return
		}
	})
}

func setupICECandidateHandler(pc *webrtc.PeerConnection) {
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		// Trickle ICE：当本端发现新的候选时触发；此处简单打印调试信息
		if c != nil {
			fmt.Printf("🌐 New ICE candidate: %s\n", c.Address)
		}
	})
}

func setupDataChannelHandler(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		// 浏览器创建 DataChannel 后，服务端将收到此回调并获取通道对象 d
		fmt.Printf("✅ DataChannel created (Server): %s, ID: %d, Negotiated: %v\n", d.Label(), d.ID(), d.Negotiated())
		d.OnOpen(func() {
			// 当 DataChannel 建立完成，发送欢迎文本
			fmt.Println("✅ DataChannel opened (Server)")
			if sendErr := d.SendText("Hello from Go server 👋"); sendErr != nil {
				fmt.Printf("Failed to send text: %v\n", sendErr)
			}
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			// 当收到浏览器发来的消息，打印其内容
			fmt.Printf("📩 Received: %s\n", string(msg.Data))
		})
	})
}

func processOffer(
	pc *webrtc.PeerConnection,
	offer webrtc.SessionDescription,
	responseWriter http.ResponseWriter,
) error {
	// 设置远端描述（浏览器的 SDP Offer），驱动本端生成对应的应答参数
	if err := pc.SetRemoteDescription(offer); err != nil {
		return err
	}

	// 生成本端 SDP Answer（协调编解码能力、媒体/数据通道等）
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// 设置本地描述为刚生成的 Answer，启动底层网络与 ICE 收集
	if err := pc.SetLocalDescription(answer); err != nil {
		return err
	}

	// 等待 ICE 候选收集完成（非 Trickle 模式），确保一次性返回完整的 Answer
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	finalAnswer := pc.LocalDescription()
	if finalAnswer == nil {
		//nolint:err113
		return fmt.Errorf("local description is nil after ICE gathering")
	}

	// 返回 JSON 格式的 Answer 给浏览器（包含 ICE 候选），完成握手
	responseWriter.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(responseWriter).Encode(*finalAnswer); err != nil {
		fmt.Printf("Failed to encode answer: %v\n", err)
	}

	return nil
}

func setupCandidateHandler(pc **webrtc.PeerConnection) {
	http.HandleFunc("/candidate", func(responseWriter http.ResponseWriter, r *http.Request) {
		// 解析浏览器端在 ICE 收集中上报的候选（Trickle ICE）
		var candidate webrtc.ICECandidateInit
		if err := json.NewDecoder(r.Body).Decode(&candidate); err != nil {
			http.Error(responseWriter, err.Error(), http.StatusBadRequest)

			return
		}
		fmt.Printf("📡 Received ICE candidate: %s\n", candidate.Candidate)
		// 将候选增量添加到当前 PeerConnection（需已存在 pc）
		if *pc != nil {
			if err := (*pc).AddICECandidate(candidate); err != nil {
				fmt.Println("Failed to add candidate:", err)
			}
		}
	})
}

func setupStaticHandler() {
	// demo.html：根路径返回前端页面（内含创建 Offer、发送候选、创建 DataChannel 的逻辑）
	http.HandleFunc("/", func(responseWriter http.ResponseWriter, r *http.Request) {
		http.ServeFile(responseWriter, r, "./demo.html")
	})
}

// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
// 许可声明：MIT；该示例演示“单端口承载多个 PeerConnection 的 ICE”能力。

//go:build !js
// +build !js

// ice-single-port demonstrates Pion WebRTC's ability to serve many PeerConnections on a single port.
// 示例说明：通过 SettingEngine + UDPMux，在同一个 UDP 端口上复用所有 WebRTC 流量（STUN/DTLS/SRTP/SCTP）。
package main

import (
    // 标准库：HTTP、JSON 编解码、日志与定时器
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    // Pion 依赖：ICE（多路复用 UDPMux）、WebRTC 主包
    "github.com/pion/ice/v4"
    "github.com/pion/webrtc/v4"
)

var api *webrtc.API //nolint
// 共享 API：绑定了全局 SettingEngine，使多个 PeerConnection 共享同一 UDP 端口。

// Everything below is the Pion WebRTC API! Thanks for using it ❤️.
func doSignaling(res http.ResponseWriter, req *http.Request) {
    peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})
    if err != nil {
        panic(err)
    }
    // 为每个到来的浏览器请求创建一个新的 PeerConnection，但底层端口相同。

    // Set the handler for ICE connection state
    // This will notify you when the peer has connected/disconnected
    peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
        fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
    })
    // 监听 ICE 状态变化：观察连接建立/断开等状态。

    // Send the current time via a DataChannel to the remote peer every 3 seconds
    peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
        d.OnOpen(func() {
            for range time.Tick(time.Second * 3) {
                if err = d.SendText(time.Now().String()); err != nil {
                    panic(err)
                }
            }
        })
    })
    // 当浏览器创建 DataChannel 后：每 3 秒发一次当前时间，验证连接与复用效果。

    var offer webrtc.SessionDescription
    if err = json.NewDecoder(req.Body).Decode(&offer); err != nil {
        panic(err)
    }
    // 解析浏览器提交的 SDP Offer。

    if err = peerConnection.SetRemoteDescription(offer); err != nil {
        panic(err)
    }
    // 设置远端描述，启动底层 ICE/DTLS 等流程（共享端口）。

    // Create channel that is blocked until ICE Gathering is complete
    gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
    // 返回一个通道：当本端 ICE 候选收集完成时被关闭（本示例禁用 Trickle）。

    answer, err := peerConnection.CreateAnswer(nil)
    if err != nil {
        panic(err)
    } else if err = peerConnection.SetLocalDescription(answer); err != nil {
        panic(err)
    }
    // 生成并设置 Answer，开始收集候选并建立传输。

    // Block until ICE Gathering is complete, disabling trickle ICE
    // we do this because we only can exchange one signaling message
    // in a production application you should exchange ICE Candidates via OnICECandidate
    <-gatherComplete
    // 等待候选收集完成后再返回，本示例做一次性交换。

    response, err := json.Marshal(*peerConnection.LocalDescription())
    if err != nil {
        panic(err)
    }
    // 将本端的 Answer（带候选）编码为 JSON 响应给浏览器。

    res.Header().Set("Content-Type", "application/json")
    if _, err := res.Write(response); err != nil {
        panic(err)
    }
}

func main() {
    // Create a SettingEngine, this allows non-standard WebRTC behavior
    settingEngine := webrtc.SettingEngine{}
    // SettingEngine：允许自定义非默认行为，例如共享端口、多路复用等。

    // Configure our SettingEngine to use our UDPMux. By default a PeerConnection has
    // no global state. The API+SettingEngine allows the user to share state between them.
    // In this case we are sharing our listening port across many.
    // Listen on UDP Port 8443, will be used for all WebRTC traffic
    mux, err := ice.NewMultiUDPMuxFromPort(8443)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Listening for WebRTC traffic at %d\n", 8443)
    settingEngine.SetICEUDPMux(mux)
    // 创建一个 UDPMux：监听 8443 端口，并将所有 WebRTC 流量复用在此端口。
    // 通过 SetICEUDPMux 注入到 SettingEngine，使后续 PeerConnection 共享该端口。

    // Create a new API using our SettingEngine
    api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
    // 基于 SettingEngine 构建 API 对象：用于创建共享端口的 PeerConnection。

    http.Handle("/", http.FileServer(http.Dir(".")))
    http.HandleFunc("/doSignaling", doSignaling)
    // 根路径提供前端页面；/doSignaling 用于一次性信令交换（禁用 Trickle）。

    fmt.Println("Open http://localhost:8080 to access this demo")
    // nolint: gosec
    panic(http.ListenAndServe(":8080", nil))
}

// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
// 许可声明：该示例遵循 MIT 开源许可。

// ice-restart demonstrates Pion WebRTC's ICE Restart abilities.
// 示例说明：展示如何在现有连接上进行 ICE Restart，重新收集候选并恢复连接。
package main

import (
    // 标准库：HTTP 服务与 JSON 编解码、日志输出、定时器
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    // Pion WebRTC 主包
    "github.com/pion/webrtc/v4"
)

var peerConnection *webrtc.PeerConnection //nolint
// 全局 PeerConnection：首次请求时创建并复用，用于演示同一会话上的 ICE Restart。

// nolint: cyclop
func doSignaling(res http.ResponseWriter, req *http.Request) {
    var err error

    if peerConnection == nil {
        if peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{}); err != nil {
            panic(err)
        }
        // 创建 PeerConnection，配置为空（浏览器端提供 STUN），用于承载 DataChannel。

        // 设置 ICE 连接状态变化回调：打印当前状态（checking/connected/disconnected/failed 等）
        peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
            fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
        })

        // 当远端创建 DataChannel 后：每 3 秒通过该通道发送当前时间字符串
        peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
            d.OnOpen(func() {
                for range time.Tick(time.Second * 3) {
                    if err = d.SendText(time.Now().String()); err != nil {
                        panic(err)
                    }
                }
            })
        })
    }

    var offer webrtc.SessionDescription
    if err = json.NewDecoder(req.Body).Decode(&offer); err != nil {
        panic(err)
    }
    // 解码浏览器端提交的 SDP Offer（可能包含 `{iceRestart: true}` 触发重启）。

    if err = peerConnection.SetRemoteDescription(offer); err != nil {
        panic(err)
    }
    // 设置远端描述：若为 ICE Restart 的 Offer，将触发重新收集本端候选。

    // Create channel that is blocked until ICE Gathering is complete
    gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
    // 返回一个通道：当本端 ICE 候选收集完成时关闭，用于非 Trickle 的一次性交换。

    answer, err := peerConnection.CreateAnswer(nil)
    if err != nil {
        panic(err)
    } else if err = peerConnection.SetLocalDescription(answer); err != nil {
        panic(err)
    }
    // 生成并设置本端 Answer，启动底层网络收发与候选收集。

    // Block until ICE Gathering is complete, disabling trickle ICE
    // we do this because we only can exchange one signaling message
    // in a production application you should exchange ICE Candidates via OnICECandidate
    <-gatherComplete
    // 阻塞直到 ICE 候选收集完成；示例只交换一次信令，不使用 Trickle。

    response, err := json.Marshal(*peerConnection.LocalDescription())
    if err != nil {
        panic(err)
    }
    // 将本端 LocalDescription（包含 ICE 候选的 Answer）编码为 JSON 以响应浏览器。

    res.Header().Set("Content-Type", "application/json")
    if _, err := res.Write(response); err != nil {
        panic(err)
    }
}

func main() {
    http.Handle("/", http.FileServer(http.Dir(".")))   // 根路径提供 index.html 静态页面
    http.HandleFunc("/doSignaling", doSignaling)         // 信令接口：接收 Offer，返回 Answer

    fmt.Println("Open http://localhost:8080 to access this demo")
    // nolint: gosec
    panic(http.ListenAndServe(":8080", nil))
}

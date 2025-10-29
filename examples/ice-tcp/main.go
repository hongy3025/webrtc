// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

// ice-tcp demonstrates Pion WebRTC's ICE TCP abilities.
//
// 中文说明：
// 这个示例演示如何只使用 TCP 作为 ICE 传输层（禁用 UDP），
// 并通过 ICETCPMux 在单一 TCP 监听端口上承载所有 WebRTC 连接。
// 前端通过一次性信令（不使用 Trickle ICE）完成 Offer/Answer，
// 连接成功后由服务端通过 DataChannel 每 3 秒推送一次时间字符串。
package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    // 中文：引入标准库 net 以创建 TCP 监听（作为 WebRTC 的 ICE-TCP 监听基础）
    "net"
    "net/http"
    "time"

    "github.com/pion/webrtc/v4"
)

var api *webrtc.API //nolint

func doSignaling(res http.ResponseWriter, req *http.Request) { //nolint:cyclop
    // 中文：使用带有 SettingEngine 的全局 api 创建 PeerConnection
    peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})
    if err != nil {
        panic(err)
    }

    // Set the handler for ICE connection state
    // This will notify you when the peer has connected/disconnected
    // 中文：注册 ICE 连接状态变化回调，用于观察 TCP 下的连接生命周期
    peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
        fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
    })

    // Send the current time via a DataChannel to the remote peer every 3 seconds
    // 中文：当远端创建的 DataChannel 到达服务端时，打开后每 3 秒发送一次当前时间
    peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
        d.OnOpen(func() {
            for range time.Tick(time.Second * 3) {
                if err = d.SendText(time.Now().String()); err != nil {
                    // 中文：如果连接已关闭，SendText 可能返回 io.ErrClosedPipe，直接结束循环即可
                    if errors.Is(err, io.ErrClosedPipe) {
                        return
                    }
                    panic(err)
                }
            }
        })
    })

    // 中文：读取浏览器发送的 SDP Offer（一次性信令，不做候选增量）
    var offer webrtc.SessionDescription
    if err = json.NewDecoder(req.Body).Decode(&offer); err != nil {
        panic(err)
    }

    // 中文：设置远端描述（浏览器的本地描述），触发 ICE/DTLS/SCTP 初始化
    if err = peerConnection.SetRemoteDescription(offer); err != nil {
        panic(err)
    }

    // Create channel that is blocked until ICE Gathering is complete
    // 中文：创建一个阻塞通道，直到本地 ICE Candidate 收集完成（用于一次性返回 Answer）
    gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

    // 中文：创建 Answer 并设置为本地描述，随后等待候选收集全部结束
    answer, err := peerConnection.CreateAnswer(nil)
    if err != nil {
        panic(err)
    } else if err = peerConnection.SetLocalDescription(answer); err != nil {
        panic(err)
    }

    // Block until ICE Gathering is complete, disabling trickle ICE
    // we do this because we only can exchange one signaling message
    // in a production application you should exchange ICE Candidates via OnICECandidate
    // 中文：阻塞直到 ICE 收集完成，这里禁用 Trickle ICE（仅进行一次 HTTP 交互）
    <-gatherComplete

    // 中文：将本地 SDP Answer 序列化为 JSON 并返回给浏览器
    response, err := json.Marshal(*peerConnection.LocalDescription())
    if err != nil {
        panic(err)
    }

    res.Header().Set("Content-Type", "application/json")
    if _, err := res.Write(response); err != nil {
        panic(err)
    }
}

//nolint:cyclop
func main() {
    // 中文：创建 SettingEngine，用于强制开启仅 TCP 的网络类型并挂载 ICETCPMux
    settingEngine := webrtc.SettingEngine{}

    // Enable support only for TCP ICE candidates.
    // 中文：仅启用 TCP4/TCP6 两种网络类型；这样将只生成 TCP 类型的 ICE 候选
    settingEngine.SetNetworkTypes([]webrtc.NetworkType{
        webrtc.NetworkTypeTCP4,
        webrtc.NetworkTypeTCP6,
    })

    // 中文：在 0.0.0.0:8443 上创建一个 TCP 监听，承载 ICE-TCP（含 DTLS/SRTP/SCTP）传输
    tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{
        IP:   net.IP{0, 0, 0, 0},
        Port: 8443,
    })
    if err != nil {
        panic(err)
    }

    // 中文：打印服务端的 TCP 监听地址，供调试观察
    fmt.Printf("Listening for ICE TCP at %s\n", tcpListener.Addr())

    // 中文：创建 ICETCPMux，将同一 TCP 监听复用到多个 PeerConnection 上；第三个参数 8 为连接后队列长度
    tcpMux := webrtc.NewICETCPMux(nil, tcpListener, 8)
    // 中文：将 ICETCPMux 安装到 SettingEngine，使所有后续 PeerConnection 复用该 TCP Mux
    settingEngine.SetICETCPMux(tcpMux)

    // 中文：用配置过的 SettingEngine 创建全局 API，对应后续所有 PeerConnection 的构造
    api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

    // 中文：提供静态文件服务（当前目录）与简易信令接口 /doSignaling
    http.Handle("/", http.FileServer(http.Dir(".")))
    http.HandleFunc("/doSignaling", doSignaling)

    // 中文：提示访问地址并启动 HTTP 服务（示例仅演示用，未做 TLS/鉴权/错误恢复）
    fmt.Println("Open http://localhost:8080 to access this demo")
    // nolint: gosec
    panic(http.ListenAndServe(":8080", nil))
}

// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// pion-to-pion is an example of two pion instances communicating directly!
//
// 中文说明：
// 该示例演示两个独立的 Pion 进程（Offer 端与 Answer 端）直接通过 WebRTC 通信。
// 两端通过简单的 HTTP 接口交换 SDP 与 ICE 候选，实现快速打洞与建立连接。
// 本文件为 Offer 端，负责：
// 1) 启动 HTTP 服务用于接收 Answer 端发送的候选与 SDP 应答；
// 2) 创建 PeerConnection 与 DataChannel，定期发送随机字符串；
// 3) 生成本地 Offer 并通过 HTTP 发送给 Answer 端；
// 4) 在收集到新的 ICE 候选时，若已有远端描述则立即上报到 Answer 端，否则暂存。
package main

import (
    "bytes"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/pion/randutil"
    "github.com/pion/webrtc/v4"
)

func signalCandidate(addr string, candidate *webrtc.ICECandidate) error {
    // 中文：将 ICE 候选序列化为纯字符串（Candidate 字段），POST 到对端 /candidate
    payload := []byte(candidate.ToJSON().Candidate)
    resp, err := http.Post( //nolint:noctx
        fmt.Sprintf("http://%s/candidate", addr),
        "application/json; charset=utf-8",
        bytes.NewReader(payload),
    )
    if err != nil {
        return err
    }

    return resp.Body.Close()
}

//nolint:gocognit, cyclop
func main() {
    // 中文：命令行参数，用于配置本端与对端的 HTTP 地址
    offerAddr := flag.String("offer-address", ":50000", "Address that the Offer HTTP server is hosted on.")
    answerAddr := flag.String("answer-address", "127.0.0.1:60000", "Address that the Answer HTTP server is hosted on.")
    flag.Parse()

    // 中文：用于在远端描述未设置时暂存本端产生的候选，待设置后批量发送
    var candidatesMux sync.Mutex
    pendingCandidates := make([]*webrtc.ICECandidate, 0)

    // Everything below is the Pion WebRTC API! Thanks for using it ❤️.

    // Prepare the configuration
    // 中文：配置 STUN 服务器，帮助双方收集服务器反射候选（打洞）
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {
                URLs: []string{"stun:stun.l.google.com:19302"},
            },
        },
    }

    // Create a new RTCPeerConnection
    // 中文：创建 PeerConnection，后续用于建立 DTLS/SCTP 与 DataChannel
    peerConnection, err := webrtc.NewPeerConnection(config)
    if err != nil {
        panic(err)
    }
    defer func() {
        if cErr := peerConnection.Close(); cErr != nil {
            fmt.Printf("cannot close peerConnection: %v\n", cErr)
        }
    }()

    // When an ICE candidate is available send to the other Pion instance
    // the other Pion instance will add this candidate by calling AddICECandidate
    // 中文：当本端收集到新的候选时，如果远端描述尚未设置则暂存，
    // 一旦远端描述已设置则立即通过 HTTP 发送到对端 /candidate。
    peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
        if candidate == nil {
            return
        }

        candidatesMux.Lock()
        defer candidatesMux.Unlock()

        desc := peerConnection.RemoteDescription()
        if desc == nil {
            pendingCandidates = append(pendingCandidates, candidate)
        } else if onICECandidateErr := signalCandidate(*answerAddr, candidate); onICECandidateErr != nil {
            panic(onICECandidateErr)
        }
    })

    // A HTTP handler that allows the other Pion instance to send us ICE candidates
    // This allows us to add ICE candidates faster, we don't have to wait for STUN or TURN
    // candidates which may be slower
    // 中文：提供 /candidate 接口，接收 Answer 端上报的候选并立即调用 AddICECandidate
    http.HandleFunc("/candidate", func(res http.ResponseWriter, req *http.Request) { //nolint: revive
        candidate, candidateErr := io.ReadAll(req.Body)
        if candidateErr != nil {
            panic(candidateErr)
        }
        if candidateErr := peerConnection.AddICECandidate(
            webrtc.ICECandidateInit{Candidate: string(candidate)},
        ); candidateErr != nil {
            panic(candidateErr)
        }
    })

    // A HTTP handler that processes a SessionDescription given to us from the other Pion process
    // 中文：提供 /sdp 接口，接收对端的 SDP（通常为 Answer），设置远端描述后，把暂存候选批量上报
    http.HandleFunc("/sdp", func(res http.ResponseWriter, req *http.Request) { //nolint: revive
        sdp := webrtc.SessionDescription{}
        if sdpErr := json.NewDecoder(req.Body).Decode(&sdp); sdpErr != nil {
            panic(sdpErr)
        }

        if sdpErr := peerConnection.SetRemoteDescription(sdp); sdpErr != nil {
            panic(sdpErr)
        }

        candidatesMux.Lock()
        defer candidatesMux.Unlock()

        for _, c := range pendingCandidates {
            if onICECandidateErr := signalCandidate(*answerAddr, c); onICECandidateErr != nil {
                panic(onICECandidateErr)
            }
        }
    })
    // Start HTTP server that accepts requests from the answer process
    // nolint: gosec
    // 中文：启动 Offer 端的 HTTP 服务器，供 Answer 端回传候选与 SDP
    go func() { panic(http.ListenAndServe(*offerAddr, nil)) }()

    // Create a datachannel with label 'data'
    // 中文：创建 DataChannel（本端），用于与 Answer 端互发文本消息
    dataChannel, err := peerConnection.CreateDataChannel("data", nil)
    if err != nil {
        panic(err)
    }

    // Set the handler for Peer connection state
    // This will notify you when the peer has connected/disconnected
    // 中文：监听 PeerConnection 状态变化，失败或关闭时退出进程
    peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
        fmt.Printf("Peer Connection State has changed: %s\n", state.String())

        if state == webrtc.PeerConnectionStateFailed {
            // Wait until PeerConnection has had no network activity for 30 seconds or another failure.
            // It may be reconnected using an ICE Restart.
            // Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
            // Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
            // 中文：进入失败状态后直接退出示例
            fmt.Println("Peer Connection has gone to failed exiting")
            os.Exit(0)
        }

        if state == webrtc.PeerConnectionStateClosed {
            // PeerConnection was explicitly closed. This usually happens from a DTLS CloseNotify
            // 中文：显式关闭时退出示例
            fmt.Println("Peer Connection has gone to closed exiting")
            os.Exit(0)
        }
    })

    // Register channel opening handling
    // 中文：DataChannel 打开后，每 5 秒生成一次随机字符串并通过 DataChannel 发送
    dataChannel.OnOpen(func() {
        fmt.Printf(
            "Data channel '%s'-'%d' open. Random messages will now be sent to any connected DataChannels every 5 seconds\n",
            dataChannel.Label(), dataChannel.ID(),
        )

        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        for range ticker.C {
            message, sendTextErr := randutil.GenerateCryptoRandomString(
                15, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
            )
            if sendTextErr != nil {
                panic(sendTextErr)
            }

            // Send the message as text
            fmt.Printf("Sending '%s'\n", message)
            if sendTextErr = dataChannel.SendText(message); sendTextErr != nil {
                panic(sendTextErr)
            }
        }
    })

    // Register text message handling
    // 中文：打印收到的对端消息（文本）
    dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
        fmt.Printf("Message from DataChannel '%s': '%s'\n", dataChannel.Label(), string(msg.Data))
    })

    // Create an offer to send to the other process
    // 中文：创建 SDP Offer（包含本端的媒体/传输参数）
    offer, err := peerConnection.CreateOffer(nil)
    if err != nil {
        panic(err)
    }

    // Sets the LocalDescription, and starts our UDP listeners
    // Note: this will start the gathering of ICE candidates
    // 中文：设置本地描述，开始候选收集（含 STUN）
    if err = peerConnection.SetLocalDescription(offer); err != nil {
        panic(err)
    }

    // Send our offer to the HTTP server listening in the other process
    // 中文：序列化并将 Offer 通过 HTTP 发送到 Answer 端 /sdp
    payload, err := json.Marshal(offer)
    if err != nil {
        panic(err)
    }
    resp, err := http.Post( //nolint:noctx
        fmt.Sprintf("http://%s/sdp", *answerAddr),
        "application/json; charset=utf-8",
        bytes.NewReader(payload),
    )
    if err != nil {
        panic(err)
    } else if err := resp.Body.Close(); err != nil {
        panic(err)
    }

    // Block forever
    // 中文：阻塞防止进程退出，保持连接与 HTTP 服务运行
    select {}
}

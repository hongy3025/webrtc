// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// pion-to-pion is an example of two pion instances communicating directly!
//
// 中文说明：
// 该示例是 Pion-to-Pion 的 Answer 端，与 Offer 端通过简单 HTTP 接口交换 SDP 与候选。
// Answer 端职责：
// 1) 启动 HTTP 服务并提供 /sdp 与 /candidate；
// 2) 处理来自 Offer 端的 SDP（Offer），生成本地 Answer 并回传；
// 3) 收到新候选时在设置远端描述前暂存，设置后批量上报；
// 4) 监听 PeerConnection 状态与 DataChannel 行为，定期发送随机消息。
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
    // 中文：将候选序列化为纯字符串并 POST 到对端 /candidate
    payload := []byte(candidate.ToJSON().Candidate)
    resp, err := http.Post(fmt.Sprintf("http://%s/candidate", addr), // nolint:noctx
        "application/json; charset=utf-8", bytes.NewReader(payload))
    if err != nil {
        return err
    }

    return resp.Body.Close()
}

// nolint:gocognit, cyclop
func main() {
    // 中文：命令行参数，配置 Offer 端与本端的 HTTP 地址
    offerAddr := flag.String("offer-address", "localhost:50000", "Address that the Offer HTTP server is hosted on.")
    answerAddr := flag.String("answer-address", ":60000", "Address that the Answer HTTP server is hosted on.")
    flag.Parse()

    // 中文：暂存本端收集到的候选，待远端描述设置后批量上报
    var candidatesMux sync.Mutex
    pendingCandidates := make([]*webrtc.ICECandidate, 0)
    // Everything below is the Pion WebRTC API! Thanks for using it ❤️.

    // Prepare the configuration
    // 中文：配置 STUN 服务器，用于收集服务器反射候选
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {
                URLs: []string{"stun:stun.l.google.com:19302"},
            },
        },
    }

    // Create a new RTCPeerConnection
    // 中文：创建 PeerConnection，承担传输与 DataChannel 功能
    peerConnection, err := webrtc.NewPeerConnection(config)
    if err != nil {
        panic(err)
    }
    defer func() {
        if err := peerConnection.Close(); err != nil {
            fmt.Printf("cannot close peerConnection: %v\n", err)
        }
    }()

    // When an ICE candidate is available send to the other Pion instance
    // the other Pion instance will add this candidate by calling AddICECandidate
    // 中文：本端产生候选时，若远端描述未设置则暂存；已设置则立即上报至 Offer 端
    peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
        if candidate == nil {
            return
        }

        candidatesMux.Lock()
        defer candidatesMux.Unlock()

        desc := peerConnection.RemoteDescription()
        if desc == nil {
            pendingCandidates = append(pendingCandidates, candidate)
        } else if onICECandidateErr := signalCandidate(*offerAddr, candidate); onICECandidateErr != nil {
            panic(onICECandidateErr)
        }
    })

    // A HTTP handler that allows the other Pion instance to send us ICE candidates
    // This allows us to add ICE candidates faster, we don't have to wait for STUN or TURN
    // candidates which may be slower
    // 中文：提供 /candidate 接口，接收对端候选并调用 AddICECandidate 加速建立连接
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
    // 中文：提供 /sdp 接口，接收 Offer 端的 SDP（Offer），生成本端 Answer 并回传，然后设置本地描述
    http.HandleFunc("/sdp", func(res http.ResponseWriter, req *http.Request) { // nolint: revive
        sdp := webrtc.SessionDescription{}
        if err := json.NewDecoder(req.Body).Decode(&sdp); err != nil {
            panic(err)
        }

        if err := peerConnection.SetRemoteDescription(sdp); err != nil {
            panic(err)
        }

        // Create an answer to send to the other process
        // 中文：生成本端 Answer，描述传输与媒体参数
        answer, err := peerConnection.CreateAnswer(nil)
        if err != nil {
            panic(err)
        }

        // Send our answer to the HTTP server listening in the other process
        // 中文：将 Answer 通过 HTTP 回传到 Offer 端 /sdp，完成 SDP 交换
        payload, err := json.Marshal(answer)
        if err != nil {
            panic(err)
        }
        resp, err := http.Post( //nolint:noctx
            fmt.Sprintf("http://%s/sdp", *offerAddr),
            "application/json; charset=utf-8",
            bytes.NewReader(payload),
        ) // nolint:noctx
        if err != nil {
            panic(err)
        } else if closeErr := resp.Body.Close(); closeErr != nil {
            panic(closeErr)
        }

        // Sets the LocalDescription, and starts our UDP listeners
        // 中文：设置本地描述，开始 ICE 候选收集
        err = peerConnection.SetLocalDescription(answer)
        if err != nil {
            panic(err)
        }

        // 中文：批量发送暂存的候选到 Offer 端
        candidatesMux.Lock()
        for _, c := range pendingCandidates {
            onICECandidateErr := signalCandidate(*offerAddr, c)
            if onICECandidateErr != nil {
                panic(onICECandidateErr)
            }
        }
        candidatesMux.Unlock()
    })

    // Set the handler for Peer connection state
    // This will notify you when the peer has connected/disconnected
    // 中文：监听 PC 状态变化；失败或关闭时退出示例
    peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
        fmt.Printf("Peer Connection State has changed: %s\n", state.String())

        if state == webrtc.PeerConnectionStateFailed {
            // Wait until PeerConnection has had no network activity for 30 seconds or another failure.
            // It may be reconnected using an ICE Restart.
            // Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
            // Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
            fmt.Println("Peer Connection has gone to failed exiting")
            os.Exit(0)
        }

        if state == webrtc.PeerConnectionStateClosed {
            // PeerConnection was explicitly closed. This usually happens from a DTLS CloseNotify
            fmt.Println("Peer Connection has gone to closed exiting")
            os.Exit(0)
        }
    })

    // Register data channel creation handling
    // 中文：当对端创建了 DataChannel，本端将收到回调。为其绑定打开与消息事件。
    peerConnection.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
        fmt.Printf("New DataChannel %s %d\n", dataChannel.Label(), dataChannel.ID())

        // Register channel opening handling
        // 中文：DataChannel 打开后每 5 秒发送随机字符串到对端
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
        // 中文：打印收到的对端文本消息
        dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
            fmt.Printf("Message from DataChannel '%s': '%s'\n", dataChannel.Label(), string(msg.Data))
        })
    })

    // Start HTTP server that accepts requests from the offer process to exchange SDP and Candidates
    // nolint: gosec
    // 中文：启动 Answer 端 HTTP 服务器，接收 Offer 端的 SDP 与候选请求
    panic(http.ListenAndServe(*answerAddr, nil))
}

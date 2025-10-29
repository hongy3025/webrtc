// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
// 许可声明：该示例遵循 MIT 开源许可。

// data-channels is a Pion WebRTC application that shows how you can send/recv DataChannel messages from a web browser
// 示例说明：展示如何通过浏览器与服务端使用 WebRTC DataChannel 发送/接收文本消息。
package main

import (
    // 标准库：输入输出、编解码与字符串处理、计时器
    "bufio"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "strings"
    "time"

    // Pion 工具与 WebRTC API
    "github.com/pion/randutil"
    "github.com/pion/webrtc/v4"
)

// nolint:cyclop
func main() {
    // Everything below is the Pion WebRTC API! Thanks for using it ❤️.
    // 以下为使用 Pion WebRTC 的主要流程。

    // Prepare the configuration
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {
                URLs: []string{"stun:stun.l.google.com:19302"},
            },
        },
    }
    // PeerConnection 配置：使用 Google STUN 获取外网可达地址。

    // Create a new RTCPeerConnection
    peerConnection, err := webrtc.NewPeerConnection(config)
    if err != nil {
        panic(err)
    }
    defer func() {
        if cErr := peerConnection.Close(); cErr != nil {
            fmt.Printf("cannot close peerConnection: %v\n", cErr)
        }
    }()
    // 创建 PeerConnection，并在退出时安全关闭释放资源。

    // Set the handler for Peer connection state
    // This will notify you when the peer has connected/disconnected
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
    // 连接状态回调：打印状态；在失败/关闭时直接退出示例程序。

    // Register data channel creation handling
    peerConnection.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
        fmt.Printf("New DataChannel %s %d\n", dataChannel.Label(), dataChannel.ID())
        // 当远端（浏览器）创建 DataChannel 后，服务端会收到该回调并获取通道对象。

        // Register channel opening handling
        dataChannel.OnOpen(func() {
            fmt.Printf(
                "Data channel '%s'-'%d' open. Random messages will now be sent to any connected DataChannels every 5 seconds\n",
                dataChannel.Label(), dataChannel.ID(),
            )

            ticker := time.NewTicker(5 * time.Second)
            defer ticker.Stop()
            for range ticker.C {
                message, sendErr := randutil.GenerateCryptoRandomString(15, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
                if sendErr != nil {
                    panic(sendErr)
                }

                // Send the message as text
                fmt.Printf("Sending '%s'\n", message)
                if sendErr = dataChannel.SendText(message); sendErr != nil {
                    panic(sendErr)
                }
            }
        })
        // 当 DataChannel 打开后：每 5 秒生成 15 位随机字符串并通过文本消息发送给对端。

        // Register text message handling
        dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
            fmt.Printf("Message from DataChannel '%s': '%s'\n", dataChannel.Label(), string(msg.Data))
        })
        // 收到对端发送的文本消息时：打印消息内容。
    })

    // Wait for the offer to be pasted
    offer := webrtc.SessionDescription{}
    decode(readUntilNewline(), &offer)
    // 从标准输入读取 base64 编码的 Offer，解码成 SessionDescription。

    // Set the remote SessionDescription
    err = peerConnection.SetRemoteDescription(offer)
    if err != nil {
        panic(err)
    }
    // 设置远端描述（浏览器的 Offer），进入应答流程。

    // Create an answer
    answer, err := peerConnection.CreateAnswer(nil)
    if err != nil {
        panic(err)
    }
    // 生成本端 SDP Answer。

    // Create channel that is blocked until ICE Gathering is complete
    gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
    // 返回一个通道：当 ICE 候选收集完成时关闭，用于非 Trickle 的一次性交换。

    // Sets the LocalDescription, and starts our UDP listeners
    err = peerConnection.SetLocalDescription(answer)
    if err != nil {
        panic(err)
    }
    // 设置本地描述并启动底层网络发送接收。

    // Block until ICE Gathering is complete, disabling trickle ICE
    // we do this because we only can exchange one signaling message
    // in a production application you should exchange ICE Candidates via OnICECandidate
    <-gatherComplete
    // 阻塞直到 ICE 候选收集完成；示例中只交换一次信令，禁用 Trickle。

    // Output the answer in base64 so we can paste it in browser
    fmt.Println(encode(peerConnection.LocalDescription()))
    // 输出 base64 编码的 Answer，供浏览器粘贴设置。

    // Block forever
    select {}
    // 阻塞保持进程运行（直到外部中断）。
}

// Read from stdin until we get a newline.
func readUntilNewline() (in string) {
    var err error

    r := bufio.NewReader(os.Stdin)
    for {
        in, err = r.ReadString('\n')
        if err != nil && !errors.Is(err, io.EOF) {
            panic(err)
        }

        if in = strings.TrimSpace(in); len(in) > 0 {
            break
        }
    }

    fmt.Println("")

    return
}
// 从 stdin 读取直到换行；去除空白并返回有效字符串。

// JSON encode + base64 a SessionDescription.
func encode(obj *webrtc.SessionDescription) string {
    b, err := json.Marshal(obj)
    if err != nil {
        panic(err)
    }

    return base64.StdEncoding.EncodeToString(b)
}
// 将 SessionDescription 序列化为 JSON 并进行 base64 编码，便于复制粘贴传输。

// Decode a base64 and unmarshal JSON into a SessionDescription.
func decode(in string, obj *webrtc.SessionDescription) {
    b, err := base64.StdEncoding.DecodeString(in)
    if err != nil {
        panic(err)
    }

    if err = json.Unmarshal(b, obj); err != nil {
        panic(err)
    }
}
// 从 base64 字符串解码并反序列化为 SessionDescription 对象。

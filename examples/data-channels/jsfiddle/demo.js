/* eslint-env browser */

// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
// 许可声明：该示例遵循 MIT 开源许可。

const pc = new RTCPeerConnection({
  iceServers: [
    {
      urls: 'stun:stun.l.google.com:19302'
    }
  ]
})
// 创建浏览器端 RTCPeerConnection，配置 Google STUN 以进行 NAT 穿透。
const log = msg => {
  document.getElementById('logs').innerHTML += msg + '<br>'
}
// 简单日志输出：将文本追加到页面的 logs 容器。

const sendChannel = pc.createDataChannel('foo')
sendChannel.onclose = () => console.log('sendChannel has closed')
sendChannel.onopen = () => console.log('sendChannel has opened')
sendChannel.onmessage = e => log(`Message from DataChannel '${sendChannel.label}' payload '${e.data}'`)
// 创建名为 'foo' 的 DataChannel，并为关闭、打开、收到消息分别设置回调。

pc.oniceconnectionstatechange = e => log(`🧊 ICE ConnectionState: ${pc.iceConnectionState}`)
pc.onicecandidate = event => {
  if (event.candidate === null) {
    document.getElementById('localSessionDescription').value = btoa(JSON.stringify(pc.localDescription))
  } else {
    log(`🧊 ICE candidate: ${JSON.stringify(event.candidate, null, 2)}`)
  }
}
// ICE 状态变化时记录当前状态；当候选收集完成（candidate 为 null）时输出本地 SDP（含候选）的 base64 文本。

pc.onnegotiationneeded = e =>
  pc.createOffer().then(d => pc.setLocalDescription(d)).catch(e => log(`Failed to create offer: ${e}`))
// 当需要协商时（首次或参数变化触发），创建 Offer 并设置为本地描述。

window.sendMessage = () => {
  const message = document.getElementById('message').value
  if (message === '') {
    return alert('Message must not be empty')
  }

  sendChannel.send(message)
}
// 点击按钮后，从输入框读取非空文本，通过 DataChannel 发送到服务端。

window.startSession = () => {
  const sd = document.getElementById('remoteSessionDescription').value
  if (sd === '') {
    return alert('Session Description must not be empty')
  }

  try {
    pc.setRemoteDescription(JSON.parse(atob(sd)))
  } catch (e) {
    alert(e)
  }
}
// 将服务端返回的 Answer（base64 封装 JSON）解码并设置到远端描述，完成握手。

window.copySDP = () => {
  const browserSDP = document.getElementById('localSessionDescription')

  browserSDP.focus()
  browserSDP.select()

  try {
    const successful = document.execCommand('copy')
    const msg = successful ? 'successful' : 'unsuccessful'
    log('Copying SDP was ' + msg)
  } catch (err) {
    log('Unable to copy SDP ' + err)
  }
}
// 复制本地 SDP 到剪贴板，方便粘贴到服务端输入。

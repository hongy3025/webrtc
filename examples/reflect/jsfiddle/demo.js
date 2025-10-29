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
// 创建浏览器端的 RTCPeerConnection，并使用 Google STUN 服务器进行 NAT 穿透。
// 此处仅配置 STUN，不含 TURN，适用于公网直连或对称 NAT 之外的场景。
const log = msg => {
  document.getElementById('logs').innerHTML += msg + '<br>'
}
// 简单日志函数：将文本追加到页面上的 logs 容器中。

navigator.mediaDevices.getUserMedia({ video: true, audio: true })
  .then(stream => {
    stream.getTracks().forEach(track => pc.addTrack(track, stream))
    pc.createOffer().then(d => pc.setLocalDescription(d)).catch((err) => log('[1]' + err))
  }).catch((err) => log('[2]' + err))
// 采集本地媒体（视频+音频），并将每条轨添加到 RTCPeerConnection。
// 创建本地 SDP Offer 并设置为本地描述，启动连接流程；错误通过 log 输出。

pc.oniceconnectionstatechange = e => log('[3]' + pc.iceConnectionState)
// ICE 连接状态变化时记录当前状态（如 checking、connected、disconnected 等）。
pc.onicecandidate = event => {
  if (event.candidate === null) {
    document.getElementById('localSessionDescription').value = btoa(JSON.stringify(pc.localDescription))
  }
}
// 当 ICE 候选收集完成（event.candidate 为 null）时，将本地描述（含候选）序列化并 base64 输出到文本框。
pc.ontrack = function (event) {
  const el = document.createElement(event.track.kind)
  el.srcObject = event.streams[0]
  el.autoplay = true
  el.controls = true

  document.getElementById('remoteVideos').appendChild(el)
}
// 收到远端媒体轨后：根据轨类型（audio 或 video）创建相应元素，设置流对象并添加到页面显示。

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
// 启动会话：从页面读取服务端返回的 Answer（base64 封装的 JSON），解码并设置为远端描述。
// 成功后，若网络可达并编解码兼容，将能收到服务端回送的媒体并显示。

window.copySDP = () => {
  const browserSDP = document.getElementById('localSessionDescription')

  browserSDP.focus()
  browserSDP.select()

  try {
    const successful = document.execCommand('copy')
    const msg = successful ? 'successful' : 'unsuccessful'
    log('[4] Copying SDP was ' + msg)
  } catch (err) {
    log('[5] Unable to copy SDP ' + err)
  }
}
// 将页面中的本地 SDP 文本复制到剪贴板，便于粘贴到服务器终端。

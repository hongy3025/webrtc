/* eslint-env browser */
// 浏览器环境：该示例运行于前端，配合服务端进行简单信令交换。

// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
// 许可声明：MIT；该页面演示发布/订阅广播的浏览器侧逻辑。

const log = msg => {
  // 追加日志到页面
  document.getElementById('logs').innerHTML += msg + '<br>'
}

// 创建一个会话：根据 isPublisher 区分“发布者”或“订阅者”逻辑
window.createSession = isPublisher => {
  const pc = new RTCPeerConnection({
    iceServers: [
      {
        urls: 'stun:stun.l.google.com:19302'
      }
    ]
  })
  // 监听 ICE 连接状态变化并打印（checking/connected/disconnected/failed 等）
  pc.oniceconnectionstatechange = e => log(pc.iceConnectionState)
  pc.onicecandidate = event => {
    if (event.candidate === null) {
      // 当候选收集结束：将本地 SDP（Offer/Answer）写入文本框（base64 JSON）
      document.getElementById('localSessionDescription').value = btoa(JSON.stringify(pc.localDescription))
    }
  }

  if (isPublisher) {
    navigator.mediaDevices.getUserMedia({ video: true, audio: false })
      .then(stream => {
        // 发布者：采集本地摄像头，不采集音频；添加到 PeerConnection 并本地预览
        stream.getTracks().forEach(track => pc.addTrack(track, stream))
        document.getElementById('video1').srcObject = stream
        pc.createOffer()
          // 生成 Offer 并设置本地描述；候选收集完成后在上面的 onicecandidate 写入文本框
          .then(d => pc.setLocalDescription(d))
          .catch(log)
      }).catch(log)
  } else {
    // 订阅者：声明希望接收一个视频轨
    pc.addTransceiver('video')
    pc.createOffer()
      // 生成订阅者的 Offer 并设置本地描述
      .then(d => pc.setLocalDescription(d))
      .catch(log)

    pc.ontrack = function (event) {
      // 收到服务端转发的媒体流：绑定到 video 播放
      const el = document.getElementById('video1')
      el.srcObject = event.streams[0]
      el.autoplay = true
      el.controls = true
    }
  }

  // 将对端（服务端输出）SDP 设置到本地 PeerConnection，完成协商
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

  // 复制本地 SDP 到剪贴板，便于粘贴到服务端或对端
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

  // 隐藏“创建会话”按钮，改为显示信令交换区域
  const btns = document.getElementsByClassName('createSessionButton')
  for (let i = 0; i < btns.length; i++) {
    btns[i].style = 'display: none'
  }

  document.getElementById('signalingContainer').style = 'display: block'
}

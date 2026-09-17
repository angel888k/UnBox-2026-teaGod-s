<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Hls, { type ErrorData, type Events } from 'hls.js'
import mpegts from 'mpegts.js'
import { useHlsTracks, type TrackState } from '../useHlsTracks'
import TrackMenu from './TrackMenu.vue'

export interface PlaybackPlan {
  ID: string
  Backend: 'web' | 'mpv'
  URL: string
  Kind: string
  CanFallback: boolean
}

/** 标准播放信号：组件只上报，不决定切集或换源。 */
export type PlaybackState = 'playing' | 'buffering' | 'ready' | 'error' | 'ended'

const props = defineProps<{
  plan: PlaybackPlan | null
  seekTo?: number
  emptyText?: string
  /** 为真时播放错误只上报，不再自行降级到 mpv，由点播会话协调器决定换源。 */
  suppressFallback?: boolean
}>()
const emit = defineEmits<{
  fallback: [id: string, position: number]
  progress: [time: number, duration: number]
  playback: [state: PlaybackState, message?: string]
}>()
const video = ref<HTMLVideoElement | null>(null)
const trackState = ref<TrackState | null>(null)
const menuOpen = ref(false)
// 画面旋转角度（度），90/270 时按容器比例缩放，避免旋转后画面被裁掉。
const rotation = ref(0)
// 自绘播放控件状态：原生 controls 会跟着画面一起旋转，且全屏时无法与工具按钮共存。
const playing = ref(false)
const currentTime = ref(0)
const duration = ref(0)
const muted = ref(false)
const volume = ref(1)
const isFullscreen = ref(false)
const controlsVisible = ref(false)
const playbackRate = ref(1)
const pipActive = ref(false)
const pipSupported = ref(false)
const HIDE_CONTROLS_DELAY = 3000
// 单击画面切换播放/暂停，但要等一小会儿确认不是双击（双击是切全屏）。
const CLICK_TOGGLE_DELAY = 250
const RATE_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2]
let hideTimer: ReturnType<typeof setTimeout> | null = null
let clickTimer: ReturnType<typeof setTimeout> | null = null
let hls: Hls | null = null
let flv: ReturnType<typeof mpegts.createPlayer> | null = null
let fallbackSent = false
let errorReported = false
let networkRestarts = 0
let mediaRecoveries = 0
let attachGeneration = 0
// autoplayPending：本次挂载的流就绪后是否要自动开始播放。
let autoplayPending = false

// 传输抖动（签名过期、CDN 限流）与「后端确实解不了」必须区别对待：前者原地重试，
// 后者直接换 mpv。预算内的重试只影响这一条播放，不重置后端选择。
const MAX_NETWORK_RESTARTS = 3
const MAX_MEDIA_RECOVERIES = 2
const isHls = computed(() => props.plan?.Backend === 'web' && props.plan?.Kind === 'hls' && Hls.isSupported())

function cleanup() {
  attachGeneration++
  trackState.value?.detach(); trackState.value = null
  menuOpen.value = false
  autoplayPending = false
  playing.value = false
  currentTime.value = 0
  duration.value = 0
  pipActive.value = false
  clearHideTimer()
  if (clickTimer !== null) {
    clearTimeout(clickTimer)
    clickTimer = null
  }
  hls?.destroy(); hls = null
  flv?.destroy(); flv = null
  if (video.value) { video.value.pause(); video.value.removeAttribute('src'); video.value.load() }
}

// cycleRotation 依次切换 0° → 90° → 180° → 270° → 0°。
function cycleRotation() {
  rotation.value = (rotation.value + 90) % 360
  applyRotation()
}

// applyRotation 把画面按当前角度旋转。旋转 90/270 时内容宽高互换，需要按容器
// 与视频的实际比例做一次等比缩放，否则宽画面转竖后会溢出、被 overflow 裁掉。
function applyRotation() {
  const element = video.value
  const box = element?.parentElement
  if (!element || !box) return
  const degrees = rotation.value
  let scale = 1
  if (degrees % 180 !== 0) {
    const boxWidth = box.clientWidth
    const boxHeight = box.clientHeight
    const videoWidth = element.videoWidth || boxWidth
    const videoHeight = element.videoHeight || boxHeight
    if (boxWidth > 0 && boxHeight > 0 && videoWidth > 0 && videoHeight > 0) {
      // object-fit: contain 下内容在元素内的显示尺寸
      let contentWidth = boxWidth
      let contentHeight = (boxWidth * videoHeight) / videoWidth
      if (contentHeight > boxHeight) {
        contentHeight = boxHeight
        contentWidth = (boxHeight * videoWidth) / videoHeight
      }
      // 旋转后内容宽高互换，等比缩放到容器内
      scale = Math.min(boxWidth / contentHeight, boxHeight / contentWidth)
    }
  }
  element.style.transform = degrees === 0 ? '' : `rotate(${degrees}deg) scale(${scale})`
}

// requestAutoplay 在流就绪后自动起播：切集/换源后画面应当接着播，
// 而不是停在那里等用户再点一次原生播放按钮。被浏览器拦截时保持暂停。
function requestAutoplay() {
  const element = video.value
  if (!element || !autoplayPending) return
  autoplayPending = false
  try {
    const result = element.play()
    if (result && typeof result.catch === 'function') result.catch(() => {})
  } catch { /* 自动播放被拦截：保留原生控件供手动播放 */ }
}

// onFullscreenChange 让整个播放容器进全屏。原生全屏按钮只把 <video> 全屏，
// 那样右上角的轨道/旋转控件会跟着消失；检测到 video 独占全屏就换成容器全屏。
function onFullscreenChange() {
  const element = video.value
  const box = element?.parentElement
  if (element && box && document.fullscreenElement === element) {
    const request = box.requestFullscreen?.bind(box)
    if (request) {
      const result = request()
      if (result && typeof result.catch === 'function') result.catch(() => {})
    }
  }
  isFullscreen.value = !!box && fullscreenElement() === box
  applyRotation()
}

// fullscreenElement/requestFullscreen/exitFullscreen 兼容 WebKitGTK 的 webkit 前缀。
function fullscreenElement(): Element | null {
  const doc = document as Document & { webkitFullscreenElement?: Element | null }
  return doc.fullscreenElement ?? doc.webkitFullscreenElement ?? null
}

function requestFullscreen(el: HTMLElement): void {
  const target = el as HTMLElement & { webkitRequestFullscreen?: () => Promise<void> | void }
  const request = el.requestFullscreen?.bind(el) ?? target.webkitRequestFullscreen?.bind(el)
  if (!request) return
  const result = request() as Promise<void> | void
  if (result && typeof (result as Promise<void>).catch === 'function') (result as Promise<void>).catch(() => {})
}

function exitFullscreen(): void {
  const doc = document as Document & { webkitExitFullscreen?: () => void }
  const exit = document.exitFullscreen?.bind(document) ?? doc.webkitExitFullscreen?.bind(doc)
  exit?.()
}

function clearHideTimer(): void {
  if (hideTimer !== null) {
    clearTimeout(hideTimer)
    hideTimer = null
  }
}

// showControls 在鼠标靠近时显示控件；播放中静置一段时间后自动隐藏。
function showControls(): void {
  controlsVisible.value = true
  scheduleHideControls()
}

function scheduleHideControls(): void {
  clearHideTimer()
  // 暂停中或轨道菜单打开时保持可见，避免用户找不到控件。
  if (!playing.value || menuOpen.value) return
  hideTimer = setTimeout(() => { controlsVisible.value = false }, HIDE_CONTROLS_DELAY)
}

function onPlay(): void {
  playing.value = true
  emit('playback', 'playing')
  scheduleHideControls()
}

function onPause(): void {
  playing.value = false
  controlsVisible.value = true
  clearHideTimer()
}

function onVolumeChange(): void {
  const element = video.value
  if (!element) return
  muted.value = element.muted
  volume.value = element.muted ? 0 : element.volume
}

function togglePlay(): void {
  const element = video.value
  if (!element) return
  if (element.paused) {
    try {
      const result = element.play()
      if (result && typeof result.catch === 'function') result.catch(() => {})
    } catch { /* 播放被拦截时保持暂停 */ }
  } else {
    element.pause()
  }
}

function toggleMute(): void {
  const element = video.value
  if (element) element.muted = !element.muted
}

function onRateChange(event: Event): void {
  const element = video.value
  if (!element) return
  const next = Number((event.target as HTMLSelectElement).value)
  element.playbackRate = next
  playbackRate.value = next
}

function onRateUpdate(): void {
  const element = video.value
  if (element) playbackRate.value = element.playbackRate
}

// togglePictureInPicture 优先用标准 API，Safari/WebKitGTK 回退到 webkit 模式切换。
async function togglePictureInPicture(): Promise<void> {
  const element = video.value as (HTMLVideoElement & {
    webkitSetPresentationMode?: (mode: string) => void
  }) | null
  if (!element) return
  try {
    if (document.pictureInPictureElement === element && document.exitPictureInPicture) {
      await document.exitPictureInPicture()
      return
    }
    const request = element.requestPictureInPicture?.bind(element)
    if (request) await request()
    else element.webkitSetPresentationMode?.('picture-in-picture')
  } catch { /* 画中画不可用时静默忽略 */ }
}

function onEnterPictureInPicture(): void { pipActive.value = true }
function onLeavePictureInPicture(): void { pipActive.value = false }

// detectPictureInPicture 判断当前 WebView 是否支持画中画：Chromium 用标准 API，
// WebKit（Safari / WebKitGTK）走 webkitSetPresentationMode。
function detectPictureInPicture(): void {
  if (typeof HTMLVideoElement === 'undefined') return
  const doc = document as Document & { pictureInPictureEnabled?: boolean }
  const webkit = 'webkitSetPresentationMode' in HTMLVideoElement.prototype
  const standard = doc.pictureInPictureEnabled === true && 'requestPictureInPicture' in HTMLVideoElement.prototype
  pipSupported.value = webkit || standard
}

// onVideoClick 单击画面切换播放/暂停；若是双击（切全屏）则取消这次单击动作。
function onVideoClick(): void {
  showControls()
  if (clickTimer !== null) return
  clickTimer = setTimeout(() => {
    clickTimer = null
    togglePlay()
  }, CLICK_TOGGLE_DELAY)
}

function onVideoDblClick(): void {
  if (clickTimer !== null) {
    clearTimeout(clickTimer)
    clickTimer = null
  }
  toggleFullscreen()
}

function onSeekInput(event: Event): void {
  const element = video.value
  if (!element) return
  const next = Number((event.target as HTMLInputElement).value)
  element.currentTime = next
  currentTime.value = next
}

function onVolumeInput(event: Event): void {
  const element = video.value
  if (!element) return
  const next = Number((event.target as HTMLInputElement).value)
  element.volume = next
  element.muted = next === 0
}

function toggleFullscreen(): void {
  const box = video.value?.parentElement
  if (!box) return
  if (fullscreenElement()) exitFullscreen()
  else requestFullscreen(box)
}

// onKeydown 补回原生控件提供的常用快捷键（自绘控件后浏览器不再代劳）。
function onKeydown(event: KeyboardEvent): void {
  const element = video.value
  if (!element) return
  switch (event.key) {
    case ' ':
    case 'k':
      event.preventDefault()
      togglePlay()
      break
    case 'ArrowLeft':
      event.preventDefault()
      element.currentTime = Math.max(0, element.currentTime - 5)
      break
    case 'ArrowRight':
      event.preventDefault()
      element.currentTime = Math.min(element.duration || 0, element.currentTime + 5)
      break
    case 'f':
      event.preventDefault()
      toggleFullscreen()
      break
  }
}

// formatTime 输出 mm:ss（超过一小时给 h:mm:ss）。
function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '00:00'
  const total = Math.floor(seconds)
  const pad = (n: number) => String(n).padStart(2, '0')
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(secs)}` : `${pad(minutes)}:${pad(secs)}`
}

function requestFallback() {
  if (!fallbackSent && props.plan?.CanFallback && props.plan.Backend === 'web') {
    fallbackSent = true
    // 带上已看位置：mpv 据此续播，否则降级等于从头重播。
    emit('fallback', props.plan.ID, video.value?.currentTime || 0)
  }
}

// reportError 把播放失败上报给上层。suppressFallback 为真时不再自行降级，
// 换源交给 App 的点播会话协调器；同一次播放只上报一次错误信号。
function reportError(message?: string) {
  if (!errorReported) {
    errorReported = true
    emit('playback', 'error', message)
  }
  if (!props.suppressFallback) requestFallback()
}

// 原生 <video> 的 error 事件在 hls.js/mpegts 路径下可能只是内部恢复过程中的
// 中间态，这两条路径由各自的错误回调按重试预算处理，避免误判成致命错误。
function onVideoError() {
  if (hls || flv) return
  reportError()
}

// hls.js 的 fatal 只表示它自己的重试策略用尽，不等于后端能力不足。
// 1.7.1 里 attachMediaError 是 details 而非 type，挂载失败归入 MEDIA_ERROR。
function onHlsError(_event: Events, data: ErrorData) {
  if (!data.fatal) return
  console.warn('[hls] fatal', data.type, data.details)
  if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
    mediaRecoveries++
    if (mediaRecoveries <= MAX_MEDIA_RECOVERIES && hls) hls.recoverMediaError()
    else reportError(String(data.details ?? data.type))
    return
  }
  if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
    networkRestarts++
    // 无参调用：hls.js 会回到当前播放位置；传 undefined 会被算成 NaN。
    if (networkRestarts <= MAX_NETWORK_RESTARTS && hls) hls.startLoad()
    else reportError(String(data.details ?? data.type))
    return
  }
  reportError(String(data.details ?? data.type))
}

// mpegts 没有内部重试，也没有 recoverMediaError：传输错误只能靠 unload()+load() 重连，
// 编解码/能力错误（MEDIA_ERROR）重连也不会好，直接换 mpv。
function onMpegtsError(errType: string, errDetail: string, info: unknown) {
  console.warn('[mpegts] error', errType, errDetail, info)
  if (errType === mpegts.ErrorTypes.MEDIA_ERROR) {
    reportError(errDetail)
    return
  }
  networkRestarts++
  if (networkRestarts <= MAX_NETWORK_RESTARTS && flv) {
    flv.unload(); flv.load()
  } else reportError(errDetail)
}

function onTimeUpdate() {
  const element = video.value
  if (!element) return
  currentTime.value = element.currentTime
  duration.value = Number.isFinite(element.duration) ? element.duration : 0
  emit('progress', element.currentTime, element.duration || 0)
}

function applySeek() {
  const pos = props.seekTo
  if (pos && pos > 0 && video.value) {
    video.value.currentTime = pos
  }
}

function onLoadedMetadata() {
  applySeek()
  applyRotation()
}

function onCanPlay() {
  emit('playback', 'ready')
  requestAutoplay()
}

function onSelectSubtitle(index: number) {
  trackState.value?.selectSubtitle(index)
}

function onDocumentClick(event: MouseEvent) {
  if (menuOpen.value && !(event.target as HTMLElement).closest('.track-menu, .track-toggle')) menuOpen.value = false
}

async function attach(plan: PlaybackPlan | null) {
  cleanup()
  const generation = attachGeneration
  fallbackSent = false
  errorReported = false
  networkRestarts = 0
  mediaRecoveries = 0
  if (!plan || plan.Backend !== 'web') return
  autoplayPending = true
  await nextTick()
  if (generation !== attachGeneration || plan !== props.plan) return
  const element = video.value
  if (!element) return
  // 用户选过的倍速在切集/换源后继续保持。
  element.playbackRate = playbackRate.value
  applyRotation()
  if (plan.Kind === 'hls' && Hls.isSupported()) {
    hls = new Hls({ enableWorker: false })
    hls.on(Hls.Events.ERROR, onHlsError)
    hls.loadSource(plan.URL); hls.attachMedia(element)
    trackState.value = useHlsTracks(hls)
    return
  }
  if ((plan.Kind === 'flv' || plan.Kind === 'ts') && mpegts.getFeatureList().mseLivePlayback) {
    flv = mpegts.createPlayer({ type: plan.Kind === 'flv' ? 'flv' : 'mpegts', url: plan.URL })
    flv.on(mpegts.Events.ERROR, onMpegtsError); flv.attachMediaElement(element); flv.load(); return
  }
  element.src = plan.URL
}

watch(() => props.plan, attach, { immediate: true })
watch(() => props.seekTo, applySeek)
// 轨道菜单打开期间固定显示控件，关闭后再按播放状态决定何时自动隐藏。
watch(menuOpen, (open) => {
  if (open) {
    controlsVisible.value = true
    clearHideTimer()
    return
  }
  scheduleHideControls()
})
onMounted(() => {
  detectPictureInPicture()
  document.addEventListener('click', onDocumentClick)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  window.addEventListener('resize', applyRotation)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  window.removeEventListener('resize', applyRotation)
  cleanup()
})
</script>

<template>
  <div class="playback-view" :class="{ 'controls-visible': controlsVisible }" tabindex="0"
    @mousemove="showControls" @mouseleave="scheduleHideControls" @keydown="onKeydown">
    <video v-if="plan?.Backend === 'web'" ref="video" playsinline preload="metadata"
      @timeupdate="onTimeUpdate" @loadedmetadata="onLoadedMetadata"
      @playing="onPlay"
      @pause="onPause"
      @waiting="emit('playback', 'buffering')"
      @stalled="emit('playback', 'buffering')"
      @canplay="onCanPlay"
      @ended="emit('playback', 'ended')"
      @volumechange="onVolumeChange"
      @ratechange="onRateUpdate"
      @enterpictureinpicture="onEnterPictureInPicture"
      @leavepictureinpicture="onLeavePictureInPicture"
      @dblclick="onVideoDblClick"
      @click="onVideoClick"
      @error="onVideoError" />
    <div v-if="plan?.Backend === 'web'" class="player-controls">
      <div class="player-tools">
        <button v-if="isHls" class="track-toggle" type="button" title="轨道设置" aria-label="轨道设置" @click.stop="menuOpen = !menuOpen">⚙</button>
        <button class="rotate-toggle" type="button" :title="`画面旋转（当前 ${rotation}°）`" aria-label="画面旋转" @click.stop="cycleRotation">⟳ {{ rotation }}°</button>
      </div>
      <div class="player-bar">
        <button class="ctrl-btn" type="button" @click.stop="togglePlay">{{ playing ? '暂停' : '播放' }}</button>
        <span class="ctrl-time">{{ formatTime(currentTime) }} / {{ formatTime(duration) }}</span>
        <input class="ctrl-seek" type="range" min="0" :max="duration || 0" step="0.1" :value="currentTime"
          aria-label="播放进度" @input="onSeekInput" @click.stop />
        <select class="ctrl-rate" :value="playbackRate" aria-label="播放速度" @change="onRateChange" @click.stop>
          <option v-for="rate in RATE_OPTIONS" :key="rate" :value="rate">{{ rate }}×</option>
        </select>
        <button class="ctrl-btn" type="button" @click.stop="toggleMute">{{ muted ? '取消静音' : '静音' }}</button>
        <input class="ctrl-volume" type="range" min="0" max="1" step="0.05" :value="muted ? 0 : volume"
          aria-label="音量" @input="onVolumeInput" @click.stop />
        <button v-if="pipSupported" class="ctrl-btn" type="button" @click.stop="togglePictureInPicture">{{ pipActive ? '退出画中画' : '画中画' }}</button>
        <button class="ctrl-btn" type="button" @click.stop="toggleFullscreen">{{ isFullscreen ? '退出全屏' : '全屏' }}</button>
      </div>
    </div>
    <TrackMenu v-if="isHls && menuOpen && trackState"
      :levels="trackState.levels" :current-level="trackState.currentLevel"
      :audio-tracks="trackState.audioTracks" :current-audio="trackState.currentAudio"
      :subtitle-tracks="trackState.subtitleTracks" :current-subtitle="trackState.currentSubtitle"
      :select-level="trackState.selectLevel" :select-audio="trackState.selectAudio"
      :select-subtitle="onSelectSubtitle" />
    <div v-if="plan?.Backend === 'mpv'" class="mpv-status">正在使用 mpv 播放</div>
    <div v-else-if="!plan" class="playback-empty">{{ emptyText || '选择频道或剧集开始播放' }}</div>
  </div>
</template>

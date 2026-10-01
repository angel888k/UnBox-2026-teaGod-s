// vodSourceDead —— 点播源失效弹窗的纯逻辑：弹窗模型构建与「返回」按钮行为决策。
import type { VodDetailOrigin } from './vodNavigation'

// VodSourceDeadInfo 是「无法访问点播源」弹窗的展示模型。
export interface VodSourceDeadInfo {
  siteName: string
  vodTitle: string
  message: string
}

export function buildVodSourceDeadInfo(siteName: string, vodTitle: string, error: unknown): VodSourceDeadInfo {
  return { siteName, vodTitle, message: String(error) }
}

// 失效弹窗「返回」按钮的去向：首页观看记录路径在详情加载前已切到详情页，
// 失败需回首页；收藏/搜索/列表路径在详情加载成功前不会离开原页面，留在原地即可。
export function vodSourceDeadBackTarget(origin: VodDetailOrigin): 'home' | 'stay' {
  return origin === 'home' ? 'home' : 'stay'
}

export function vodSourceDeadBackLabel(origin: VodDetailOrigin): string {
  return vodSourceDeadBackTarget(origin) === 'home' ? '返回首页' : '关闭'
}

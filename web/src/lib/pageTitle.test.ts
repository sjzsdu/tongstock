import { describe, it, expect } from 'vitest'
import {
  HOME_TITLE,
  clampTitle,
  indexTitle,
  stockTitle,
  titleForPath,
  withBrand,
} from './pageTitle'

describe('pageTitle', () => {
  describe('titleForPath', () => {
    it('首页用品牌句', () => {
      expect(titleForPath('/')).toBe('TongStock · AI 投资决策')
      expect(HOME_TITLE).toBe('TongStock · AI 投资决策')
    })

    it('常规页面用「页面 · TongStock」', () => {
      expect(titleForPath('/watchlist')).toBe('自选股 · TongStock')
      expect(titleForPath('/screen')).toBe('信号筛选 · TongStock')
      expect(titleForPath('/blocks')).toBe('股票池 · TongStock')
      expect(titleForPath('/agent')).toBe('AI 助手 · TongStock')
      expect(titleForPath('/strategy/overnight')).toBe('隔夜套利 · TongStock')
      expect(titleForPath('/news')).toBe('财经资讯 · TongStock')
      expect(titleForPath('/news/event/abc123')).toBe('热点事件 · TongStock')
      expect(titleForPath('/settings')).toBe('配置 · TongStock')
    })

    it('选股页优先于个股详情匹配', () => {
      expect(titleForPath('/stock/choose')).toBe('个股分析 · TongStock')
      expect(titleForPath('/stock/choose/signal')).toBe('个股分析 · TongStock')
    })

    it('个股详情先用代码占位，等行情返回后由页面覆盖', () => {
      expect(titleForPath('/stock/601688')).toBe('个股 601688 · TongStock')
      expect(titleForPath('/stock/601688/chart')).toBe('个股 601688 · TongStock')
    })

    it('指数详情带名称', () => {
      expect(titleForPath('/index/999999')).toBe('上证指数 · TongStock')
      expect(titleForPath('/index/399300/chart')).toBe('沪深300 · TongStock')
      expect(titleForPath('/index/888888')).toBe('指数 888888 · TongStock')
    })

    it('未知路由兜底', () => {
      expect(titleForPath('/nope')).toBe('页面不存在 · TongStock')
      expect(titleForPath('/stock')).toBe('个股分析 · TongStock')
    })
  })

  describe('详情页标题', () => {
    it('股票标题带名称与代码', () => {
      expect(stockTitle('华泰证券', '601688')).toBe('华泰证券(601688) · TongStock')
      expect(stockTitle('华泰证券', '')).toBe('华泰证券 · TongStock')
    })

    it('指数标题查不到名称时用代码', () => {
      expect(indexTitle('399006')).toBe('创业板指 · TongStock')
      expect(indexTitle('000001')).toBe('指数 000001 · TongStock')
    })
  })

  describe('clampTitle', () => {
    it('短标题原样返回并归一空白', () => {
      expect(clampTitle('  中美  关税 谈判  ')).toBe('中美 关税 谈判')
    })

    it('超长标题截断并加省略号', () => {
      const title = '美联储议息会议落地'.repeat(10)
      const clipped = clampTitle(title, 20)
      expect(clipped).toHaveLength(21)
      expect(clipped.endsWith('…')).toBe(true)
    })

    it('默认上限 30 字', () => {
      expect(clampTitle('一'.repeat(30))).toBe('一'.repeat(30))
      expect(clampTitle('一'.repeat(31))).toBe(`${'一'.repeat(30)}…`)
    })
  })

  it('withBrand 拼品牌后缀', () => {
    expect(withBrand('自选股')).toBe('自选股 · TongStock')
  })
})

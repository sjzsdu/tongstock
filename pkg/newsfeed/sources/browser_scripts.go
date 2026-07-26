package sources

// 财联社抓取脚本 - 获取最新电报列表
// 页面结构：每条电报在一个 class 包含 "p-t-20 p-b-20 b-b-w-1" 的 div 中
// 时间格式：HH:MM:SS，正文以"财联社X月X日电"开头，标题在【】中
const cailiansheFetchScript = `
const task = await useOrCreateTaskSpace('fetch cailianshe news')
await openOrReuseTab('https://www.cls.cn/telegraph', { wait: true, timeout: 20 })
await wait(2)
await scrollBy(500)
await wait(2)

const data = await js(String.raw` + "`" + `(() => {
  const results = []
  const items = document.querySelectorAll('div.p-t-20.p-b-20.b-b-w-1, [class*="p-t-20"][class*="p-b-20"][class*="b-b-w-1"]')
  
  for (const el of items) {
    const fullText = el.innerText.trim()
    if (!fullText || fullText.length < 20) continue
    
    // 解析时间（格式 HH:MM:SS）
    const timeMatch = fullText.match(/(\d{2}:\d{2}:\d{2})/)
    const timeStr = timeMatch ? timeMatch[1] : ''
    
    // 解析标题（在【】中的内容）
    const titleMatch = fullText.match(/【([^】]+)】/)
    let title = titleMatch ? titleMatch[1] : ''
    
    // 提取正文内容（去除时间和标签部分）
    let content = fullText.replace(/^\d{2}:\d{2}:\d{2}/, '').trim()
    content = content.replace(/【([^】]+)】/, '').trim()
    
    // 去除底部的标签和统计数据（如"环球市场情报\n期货市场\n阅16.77W\n评论(0)\n分享(16)"）
    const lines = content.split('\n')
    const contentLines = []
    for (const line of lines) {
      if (line.match(/^(阅\d|评论\(\d|分享\(\d|环球市场|期货市场|美股|A股|半导体|存储|新能源|原油|钢铁|医药|消费|科技|房地产|金融|汽车)/) && line.length < 20) {
        break
      }
      contentLines.push(line)
    }
    content = contentLines.join(' ').trim()
    
    // 如果没有标题，用内容前30字作为标题
    if (!title) {
      title = content.substring(0, 30)
      if (content.length > 30) title += '...'
    }
    
    // 提取标签（底部的小标签）
    const tags = []
    const tagLines = fullText.split('\n').slice(-5)
    for (const line of tagLines) {
      const trimmed = line.trim()
      if (trimmed.match(/^(环球市场|期货市场|美股|A股|半导体|存储芯片|新能源|原油|钢铁|医药|消费|科技|房地产|金融|汽车|人工智能|电池|光伏)/) && trimmed.length < 20) {
        tags.push(trimmed)
      }
    }
    
    // 阅读量作为热度
    const readMatch = fullText.match(/阅(\d+\.?\d*)/)
    let hotScore = 0
    if (readMatch) {
      const num = parseFloat(readMatch[1])
      hotScore = num > 100 ? Math.floor(num / 100) : Math.floor(num)
    }
    
    results.push({
      title: title,
      summary: content.substring(0, 200),
      content: content,
      publishTime: timeStr,
      hotScore: hotScore,
      tags: tags,
      url: '',
      originalId: 'cls_' + timeStr,
      newsType: 'flash'
    })
  }
  
  return JSON.stringify(results)
})()` + "`" + `)

console.log(data)
`

// 财联社按股票搜索脚本
const cailiansheFetchByStockScript = `
const task = await useOrCreateTaskSpace('fetch cailianshe stock news')
await openOrReuseTab('https://www.cls.cn/search?keyword=%s', { wait: true, timeout: 20 })
await wait(3)

const data = await js(String.raw` + "`" + `(() => {
  const results = []
  const items = document.querySelectorAll('[class*="search-result"], [class*="result-item"], article, .list-item')
  
  for (const el of items) {
    const fullText = el.innerText.trim()
    if (!fullText || fullText.length < 10) continue
    
    const linkEl = el.querySelector('a[href]')
    const url = linkEl ? linkEl.href : ''
    
    results.push({
      title: fullText.substring(0, 100),
      summary: fullText.substring(0, 200),
      content: fullText,
      publishTime: '',
      hotScore: 0,
      tags: [],
      url: url,
      originalId: url,
      newsType: 'flash'
    })
  }
  
  return JSON.stringify(results)
})()` + "`" + `)

console.log(data)
`

// 财联社按关键词搜索脚本
const cailiansheFetchByKeywordScript = cailiansheFetchByStockScript

// 雪球抓取脚本 - 获取最新讨论
const xueqiuFetchScript = `
const task = await useOrCreateTaskSpace('fetch xueqiu news')
await openOrReuseTab('https://xueqiu.com/', { wait: true, timeout: 20 })
await wait(3)
await scrollBy(500)
await wait(2)

const data = await js(String.raw` + "`" + `(() => {
  const results = []
  const items = document.querySelectorAll('[class*="timeline"], .status-item, article, [class*="feed"]')
  
  for (const el of items) {
    const fullText = el.innerText.trim()
    if (!fullText || fullText.length < 20) continue
    
    const linkEl = el.querySelector('a[href*="/status/"], a[href*="/p/"]')
    const url = linkEl ? linkEl.href : ''
    
    const lines = fullText.split('\n').filter(l => l.trim())
    const title = lines[0] ? lines[0].substring(0, 100) : ''
    const content = fullText.substring(0, 500)
    
    results.push({
      title: title,
      summary: content.substring(0, 200),
      content: content,
      publishTime: '',
      hotScore: 0,
      tags: [],
      url: url,
      originalId: url || 'xq_' + Date.now() + '_' + Math.random(),
      newsType: 'discussion'
    })
  }
  
  return JSON.stringify(results)
})()` + "`" + `)

console.log(data)
`

// 雪球按股票搜索脚本
const xueqiuFetchByStockScript = `
const task = await useOrCreateTaskSpace('fetch xueqiu stock news')
const code = '%s'
let prefix = 'SZ'
if (code.startsWith('6')) prefix = 'SH'
const fullCode = prefix + code

await openOrReuseTab('https://xueqiu.com/S/' + fullCode, { wait: true, timeout: 20 })
await wait(3)
await scrollBy(500)
await wait(2)

const data = await js(String.raw` + "`" + `(() => {
  const results = []
  const items = document.querySelectorAll('[class*="timeline"], .status-item, article, [class*="feed"]')
  
  for (const el of items) {
    const fullText = el.innerText.trim()
    if (!fullText || fullText.length < 20) continue
    
    const linkEl = el.querySelector('a[href*="/status/"], a[href*="/p/"]')
    const url = linkEl ? linkEl.href : ''
    
    const lines = fullText.split('\n').filter(l => l.trim())
    const title = lines[0] ? lines[0].substring(0, 100) : ''
    const content = fullText.substring(0, 500)
    
    results.push({
      title: title,
      summary: content.substring(0, 200),
      content: content,
      publishTime: '',
      hotScore: 0,
      tags: [],
      url: url,
      originalId: url || 'xq_stock_' + Date.now() + '_' + Math.random(),
      newsType: 'discussion'
    })
  }
  
  return JSON.stringify(results)
})()` + "`" + `)

console.log(data)
`

// 雪球按关键词搜索脚本
const xueqiuFetchByKeywordScript = `
const task = await useOrCreateTaskSpace('fetch xueqiu keyword news')
await openOrReuseTab('https://xueqiu.com/k?q=%s', { wait: true, timeout: 20 })
await wait(3)
await scrollBy(500)
await wait(2)

const data = await js(String.raw` + "`" + `(() => {
  const results = []
  const items = document.querySelectorAll('[class*="search-result"], [class*="result-item"], article, [class*="feed"]')
  
  for (const el of items) {
    const fullText = el.innerText.trim()
    if (!fullText || fullText.length < 20) continue
    
    const linkEl = el.querySelector('a[href]')
    const url = linkEl ? linkEl.href : ''
    
    const lines = fullText.split('\n').filter(l => l.trim())
    const title = lines[0] ? lines[0].substring(0, 100) : ''
    const content = fullText.substring(0, 500)
    
    results.push({
      title: title,
      summary: content.substring(0, 200),
      content: content,
      publishTime: '',
      hotScore: 0,
      tags: [],
      url: url,
      originalId: url || 'xq_kw_' + Date.now() + '_' + Math.random(),
      newsType: 'discussion'
    })
  }
  
  return JSON.stringify(results)
})()` + "`" + `)

console.log(data)
`

import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Alert, Button, Card, Col, Empty, Input, List, Row, Space, Spin, Statistic, Tag, Typography, message } from 'antd';
import { DashboardOutlined, RadarChartOutlined, RobotOutlined, SafetyCertificateOutlined, SearchOutlined, SettingOutlined, StockOutlined } from '@ant-design/icons';
import { api, type PositionDecisionRun, type SelectionRun } from '../api/client';

function QuickStartCard({ icon, title, desc, links }: { icon: React.ReactNode; title: string; desc: string; links: { to: string; label: string }[] }) {
  return (
    <Col xs={12} md={6}>
      <Space direction="vertical" size={6} style={{ display: 'flex' }}>
        <Space size={8}>
          <span style={{ color: '#1677ff', fontSize: 16 }}>{icon}</span>
          <Typography.Text strong>{title}</Typography.Text>
        </Space>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>{desc}</Typography.Text>
        <Space size={12} wrap>
          {links.map((l) => (
            <Link key={l.to} to={l.to}>{l.label}</Link>
          ))}
        </Space>
      </Space>
    </Col>
  );
}

export default function Dashboard(){
 const navigate=useNavigate();const [selection,setSelection]=useState<SelectionRun>();const [positions,setPositions]=useState<PositionDecisionRun>();const [loading,setLoading]=useState(true);const [question,setQuestion]=useState('');const [researching,setResearching]=useState(false);const [answer,setAnswer]=useState('');
 useEffect(()=>{void Promise.allSettled([api.selectionToday().then(setSelection),api.positionDecisionToday().then(setPositions)]).finally(()=>setLoading(false))},[]);
 const research=async()=>{const q=question.trim();if(!q)return;const stock=q.match(/\b\d{6}\b/)?.[0];if(stock&&q===stock){navigate(`/stock/${stock}`);return}setResearching(true);setAnswer('');try{const r=await api.agentChat(`请以投资方法研究流程分析：${q}。必须引用真实数据和证据；没有验证结果就明确拒绝结论。`);setAnswer(r.response||r.error||'AI 未返回结果')}catch(e){void message.error(e instanceof Error?e.message:'研究启动失败')}finally{setResearching(false)}};
 const urgent=(positions?.decisions||[]).filter(x=>x.action==='exit'||x.action==='reduce');
 return <Space direction="vertical" size={20} style={{display:'flex'}}>
  <Card bordered={false} style={{background:'linear-gradient(135deg,#102b50,#111827)'}}><Space direction="vertical" size={14} style={{display:'flex'}}><Tag color="blue" style={{width:'fit-content'}}>AI 原生投资决策</Tag><Typography.Title level={2} style={{margin:0}}>今天买什么，持仓什么时候卖</Typography.Title><Typography.Text type="secondary">AI 负责研究与解释；所有分数、行情和证据来自确定性引擎。数据不足时系统会明确拒绝推荐。</Typography.Text><Input.Search size="large" value={question} onChange={e=>setQuestion(e.target.value)} onSearch={()=>void research()} enterButton={<><SearchOutlined/> 开始研究</>} loading={researching} placeholder="输入股票代码、方法名称，或自然语言问题" />{answer&&<Alert type="info" showIcon icon={<RobotOutlined/>} message="AI 研究结果" description={answer}/>}</Space></Card>
  {/* 快速开始：按任务分组的功能导航 */}
  <Card title="快速开始" bordered={false}>
    <Row gutter={[16, 16]}>
      <QuickStartCard icon={<DashboardOutlined />} title="决策" desc="今天买什么，持仓什么时候卖" links={[{ to: '/', label: '今日决策' }, { to: '/portfolio', label: '持仓卖出' }]} />
      <QuickStartCard icon={<RadarChartOutlined />} title="方法研究" desc="可信方法、范式体系与 AI 研究" links={[{ to: '/methods', label: '可信方法' }, { to: '/paradigms', label: '范式库' }, { to: '/monitoring', label: '范式监控' }, { to: '/agent', label: 'AI 助手' }]} />
      <QuickStartCard icon={<StockOutlined />} title="行情工具" desc="自选股、个股与筛选工具" links={[{ to: '/watchlist', label: '自选股' }, { to: '/stock/choose', label: '个股分析' }, { to: '/blocks', label: '股票池' }, { to: '/screen', label: '信号筛选' }]} />
      <QuickStartCard icon={<SettingOutlined />} title="系统" desc="参数与本地配置" links={[{ to: '/settings', label: '配置' }]} />
    </Row>
  </Card>
  {loading?<Spin/>:<><Row gutter={[16,16]}><Col xs={24} md={8}><Card><Statistic title="数据日期" value={selection?.snapshot_date||positions?.snapshot_date||'暂无可用快照'}/></Card></Col><Col xs={12} md={8}><Card><Statistic title="今日买入候选" value={selection?.buy_count||0}/></Card></Col><Col xs={12} md={8}><Card><Statistic title="持仓风险动作" value={urgent.length}/></Card></Col></Row>
  {!selection?<Alert type="warning" showIcon message="尚无今日选股结果" description="需要先完成真实行情同步、冻结快照和可信方法验证。系统不会展示示例推荐。"/>:<Card title={`今日候选 · ${selection.snapshot_date}`} extra={<Typography.Text type="secondary">快照 {selection.snapshot_id}</Typography.Text>}><List dataSource={selection.candidates} locale={{emptyText:<Empty description="当前没有方法通过证据门槛，暂无推荐"/>}} renderItem={c=><List.Item actions={[<Button key="detail" onClick={()=>navigate(`/stock/${c.code}`)}>查看个股</Button>]}><List.Item.Meta title={<Space><Typography.Text code>{c.code}</Typography.Text><Tag color={c.action==='buy'?'green':'orange'}>{c.action}</Tag><Tag>评分 {c.score.toFixed(2)}</Tag></Space>} description={<Space direction="vertical" size={2}><span>{c.explanation}</span><span>买入窗口：{c.buy_window}；仓位上限 {(c.position_cap_pct*100).toFixed(0)}%</span><span>退出计划：{c.exit.complete?'完整':'不完整，仅观察'}</span></Space>}/></List.Item>}/></Card>}
  {!positions?<Alert type="info" showIcon message="尚无持仓判断" description="没有真实持仓或尚未运行持仓决策。"/>:<Card title="持仓卖出判断" extra={<Button onClick={()=>navigate('/portfolio')}>查看持仓</Button>}><List dataSource={positions.decisions} locale={{emptyText:<Empty description="当前没有真实持仓"/>}} renderItem={d=><List.Item><List.Item.Meta title={<Space><b>{d.code} {d.name}</b><Tag color={d.action==='exit'?'red':d.action==='reduce'?'orange':'blue'}>{d.action}</Tag>{d.inferred&&<Tag>推断依据</Tag>}</Space>} description={`${d.explanation}；最迟：${d.deadline}${d.constraint?`；约束：${d.constraint}`:''}`}/></List.Item>}/></Card>}
  <Card size="small"><SafetyCertificateOutlined/> 本页面只消费真实 API。没有快照、证据或持仓时显示空状态，不生成演示数据。</Card></>}</Space>
}

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Drawer,
  Empty,
  Grid,
  Input,
  Layout,
  Select,
  Space,
  Tag,
  Tooltip,
  Typography,
  theme,
} from 'antd';
import {
  BugOutlined,
  MenuOutlined,
  PlusOutlined,
  RobotOutlined,
  SearchOutlined,
  SendOutlined,
  SyncOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import AgentChatMessage from '../components/AgentChatMessage';
import { ProductStatusBanner, ProductStatusBlock } from '../components/ProductStatus';
import { useProductStatus } from '../hooks/useProductStatus';
import { api, fetchWithAccessToken } from '../api/client';
import { readSSE } from '../lib/sse';
import type { AgentInfo, AgentSessionInfo } from '../types/api';
import type { AgentDiagnosticResponse } from '../types/api';

type ChatMessage = { id: string; role: string; content: string; error?: boolean };

const { Sider, Content } = Layout;
const { Text } = Typography;

/** agent id → 中文名兜底；服务端 agents[].name 存在时优先用它 */
const AGENT_NAME_FALLBACK: Record<string, string> = {
  analyst: '智能分析师',
  researcher: '策略研究员',
  critic: '研究评审',
  screener: '选股助手',
  'stock-analyst': '个股分析师',
  'stock-fundamental-analyst': '基本面分析师',
  'stock-quant-technician': '量化技术分析师',
  'stock-paradigm-miner': '范式挖掘分析师',
  'stock-discussion-host': '投研讨论主持',
  'news-agent': '财经资讯助手',
  'news-summarize': '新闻简报员',
  'method-source-researcher': '投资方法来源研究员',
};

/** 将原始 agent id 映射为用户可读的产品名称 */
function agentDisplayName(id: string): string {
  return AGENT_NAME_FALLBACK[id] ?? id ?? '通用助手';
}

/** 角色展示名：服务端中文名 > 兜底映射 > 原始 id */
function agentName(agent: AgentInfo | undefined, fallbackId = ''): string {
  const name = agent?.name?.trim();
  if (name) return name;
  return agentDisplayName(agent?.id || fallbackId);
}

/** 历史列表时间：秒级没有信息量，只保留到分钟 */
function formatSessionTime(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

let messageSeq = 0;
const nextMessageId = () => `msg-${Date.now().toString(36)}-${++messageSeq}`;

export default function AgentWeb() {
  const { token } = theme.useToken();
  const screens = Grid.useBreakpoint();
  // 首帧 screens 里还没有断点数据，按桌面渲染，避免闪一下抽屉
  const isCompact = screens.lg === false;

  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [sessions, setSessions] = useState<AgentSessionInfo[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [session, setSession] = useState('web:default');
  const [model, setModel] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [inputFocused, setInputFocused] = useState(false);
  const [historyQuery, setHistoryQuery] = useState('');
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [diagnostic, setDiagnostic] = useState<AgentDiagnosticResponse | null>(null);
  const [diagOpen, setDiagOpen] = useState(false);

  // 产品级状态（统一反馈 loading / degraded / failed / unavailable）
  const { state: productStatus, markReady, markUnavailable } = useProductStatus();

  const messageListRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const lastSessionRef = useRef(session);
  const lastMessagesRef = useRef(messages);
  const forceScrollRef = useRef(false);
  const [atBottom, setAtBottom] = useState(true);

  // 消息区是独立滚动容器：只有贴近底部时才跟随新内容，
  // 切换会话 / 恢复历史则强制回到最新一条。
  const scrollToBottom = (force = false) =>
    requestAnimationFrame(() => {
      const el = messageListRef.current;
      if (!el) return;
      const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
      if (force || distance < 160) {
        el.scrollTop = el.scrollHeight;
        setAtBottom(true);
      }
    });

  const handleMessageScroll = () => {
    const el = messageListRef.current;
    if (!el) return;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    setAtBottom(distance < 48);
  };

  // 输入框随内容自动长高，最多 160px，超出后内部滚动
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(Math.max(el.scrollHeight, 56), 160)}px`;
  }, [input]);

  useEffect(() => {
    (async () => {
      try {
        const diag = await api.agentDiagnose();
        setDiagnostic(diag);
        if (!diag.ready) {
          markUnavailable(
            [...(diag.errors || []), ...(diag.hints || [])].join('；') ||
              'AI 助手服务尚未就绪',
          );
        } else {
          markReady();
        }
      } catch (err) {
        markUnavailable(
          err instanceof Error ? err.message : '无法连接到 AI 助手服务',
        );
      }
      try {
        const st = await api.agentState();
        setAgents(st.agents || []);
        setModel(st.defaults?.model || '');
        setSelectedAgent(st.defaults?.agents?.chat || '');
        setSession(st.defaults?.session || 'web:default');
      } catch {
        // 已通过 markUnavailable 处理
      }
      try {
        const sessRes = await api.agentSessions();
        setSessions(sessRes.sessions || []);
      } catch {}
    })();
  }, [markReady, markUnavailable]);

  useEffect(() => {
    const switched = lastSessionRef.current !== session;
    const messagesChanged = lastMessagesRef.current !== messages;
    lastSessionRef.current = session;
    lastMessagesRef.current = messages;
    if (!switched && !messagesChanged) return;
    // 恢复历史/新对话会先改会话再异步灌消息，两次渲染都要能落到最新一条
    const force = switched || forceScrollRef.current;
    if (force && messagesChanged) forceScrollRef.current = false;
    scrollToBottom(force);
  }, [messages, session]);

  const loadTranscript = async (agent?: string, sess?: string) => {
    forceScrollRef.current = true;
    try {
      const data = await api.agentTranscript(sess || session, agent || selectedAgent);
      if (data.missing) {
        setMessages([{ id: nextMessageId(), role: 'system', content: '没有历史对话，直接开始新对话吧。' }]);
        return;
      }
      const history = (data.messages || []).map((m: { role: string; content: string }) => ({
        id: nextMessageId(),
        role: m.role,
        content: m.content,
      }));
      setMessages([...history, { id: nextMessageId(), role: 'system', content: '已恢复上次对话。' }]);
    } catch (err) {
      setMessages([{ id: nextMessageId(), role: 'assistant', content: String(err), error: true }]);
    }
  };

  const submit = async () => {
    const text = input.trim();
    if (!text || busy) return;
    if (productStatus.kind === 'unavailable') return;
    setInput('');
    setBusy(true);

    const assistantId = nextMessageId();
    // 发消息属于用户主动动作，无条件滚到最新一条
    forceScrollRef.current = true;
    setMessages(prev => [
      ...prev,
      { id: nextMessageId(), role: 'user', content: text },
      { id: assistantId, role: 'assistant', content: '正在生成回复...' },
    ]);
    let acc = '';
    try {
      const res = await fetchWithAccessToken('/api/agent/chat/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
        body: JSON.stringify({
          message: text,
          agent: selectedAgent,
          session,
          model,
        }),
      });
      if (!res.ok || !res.body) throw new Error(res.statusText || '请求失败');
      await readSSE(res, event => {
        if (event.type === 'delta') {
          acc += event.delta || '';
          setMessages(prev =>
            prev.map(item => (item.id === assistantId ? { ...item, content: acc } : item)),
          );
        } else if (event.type === 'error') {
          setMessages(prev =>
            prev.map(item =>
              item.id === assistantId
                ? {
                    ...item,
                    content: event.message || event.error || '生成回复失败',
                    error: true,
                  }
                : item,
            ),
          );
        }
      });
      markReady();
      // 刷新对话列表
      const sessRes = await api.agentSessions();
      setSessions(sessRes.sessions || []);
    } catch (err) {
      markUnavailable(err instanceof Error ? err.message : String(err));
      setMessages(prev =>
        prev.map(item =>
          item.id === assistantId
            ? {
                ...item,
                content: `请求失败：${err instanceof Error ? err.message : String(err)}`,
                error: true,
              }
            : item,
        ),
      );
    } finally {
      setBusy(false);
    }
  };

  const startNewConversation = () => {
    const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
    const agent = selectedAgent || agents[0]?.id || 'default';
    const value = `web:${agent}:${stamp}`;
    forceScrollRef.current = true;
    setSession(value);
    setMessages([{ id: nextMessageId(), role: 'system', content: '已开启一段新对话。' }]);
    setSidebarOpen(false);
  };

  const agentNameById = useMemo(() => {
    const map = new Map<string, string>();
    agents.forEach(a => map.set(a.id, agentName(a)));
    return map;
  }, [agents]);

  const displayNameForAgent = (id?: string) =>
    (id && agentNameById.get(id)) || agentDisplayName(id || '') || '对话';

  const filteredSessions = useMemo(() => {
    const keyword = historyQuery.trim().toLowerCase();
    return sessions.filter(s => {
      if (selectedAgent && s.agent && s.agent !== selectedAgent) return false;
      if (!keyword) return true;
      const title = s.title || displayNameForAgent(s.agent);
      // 标题、会话 id、时间（09-28 17:05 或 2026-09-28）都能搜
      return (
        title.toLowerCase().includes(keyword) ||
        s.session.toLowerCase().includes(keyword) ||
        formatSessionTime(s.updated_at).includes(keyword) ||
        (s.updated_at || '').toLowerCase().includes(keyword)
      );
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessions, selectedAgent, historyQuery, agentNameById]);

  const handleSelectSession = (s: AgentSessionInfo) => {
    setSession(s.session);
    if (s.agent) setSelectedAgent(s.agent);
    setSidebarOpen(false);
    loadTranscript(s.agent, s.session);
  };

  const agentOptions = useMemo(
    () => agents.map(a => ({ value: a.id, label: agentName(a) })),
    [agents],
  );

  const selectedAgentInfo = agents.find(a => a.id === selectedAgent);
  const currentRoleLabel = agentName(selectedAgentInfo, selectedAgent);

  const handleAgentChange = (value: string) => {
    setSelectedAgent(value);
    // 切换角色时同步更新当前对话的 agent 部分
    const parts = session.split(':');
    const newSession = parts.length >= 2 ? `${parts[0]}:${value}:${parts.slice(2).join(':')}` : `web:${value}`;
    setSession(newSession);
  };

  const sidebar = (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        flex: 1,
        // Sider 的子容器是 block，得靠 100% 高度才能把历史列表撑满剩余空间
        height: '100%',
        minHeight: 0,
      }}
    >
      <Typography.Title level={5} style={{ color: token.colorTextLightSolid, margin: '0 0 16px' }}>
        <RobotOutlined /> AI 助手
      </Typography.Title>

      {/* 助手角色：下拉选择 + 角色说明（不暴露 agent 工程 id） */}
      <Text type="secondary" style={{ fontSize: 12 }}>
        助手角色
      </Text>
      <Select
        value={selectedAgent || undefined}
        placeholder="选择助手角色"
        onChange={handleAgentChange}
        options={agentOptions}
        showSearch
        optionFilterProp="label"
        style={{ width: '100%', marginTop: 6, marginBottom: 8 }}
        popupMatchSelectWidth
      />
      <Text
        type="secondary"
        style={{ fontSize: 12, lineHeight: 1.6, display: 'block', marginBottom: 16 }}
      >
        {selectedAgentInfo?.description || '选择一个角色，它会用对应的专业视角回答你。'}
      </Text>

      {/* 历史对话 */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          历史对话
        </Text>
        <Text type="secondary" style={{ fontSize: 11 }}>
          {filteredSessions.length}/{sessions.length}
        </Text>
      </div>
      <Input
        allowClear
        size="small"
        prefix={<SearchOutlined style={{ color: token.colorTextTertiary }} />}
        placeholder="按标题或时间搜索"
        value={historyQuery}
        onChange={e => setHistoryQuery(e.target.value)}
        style={{ margin: '6px 0 8px', background: token.colorBgContainer }}
      />
      <div
        style={{
          flex: '1 1 auto',
          minHeight: 0,
          overflowY: 'auto',
          background: token.colorBgContainer,
          borderRadius: token.borderRadiusSM,
          padding: 4,
        }}
      >
        {filteredSessions.length === 0 && (
          <Text type="secondary" style={{ fontSize: 12, display: 'block', padding: '4px 4px' }}>
            {sessions.length === 0 ? '暂无历史对话' : '没有匹配的对话'}
          </Text>
        )}
        {filteredSessions.map(s => {
          const roleLabel = displayNameForAgent(s.agent);
          const title = s.title || roleLabel;
          const time = formatSessionTime(s.updated_at);
          const isActive = s.session === session;
          return (
            <div
              key={s.session}
              className="chat-session-item"
              onClick={() => handleSelectSession(s)}
              title={s.session}
              style={{
                padding: '6px 8px',
                marginBottom: 4,
                borderRadius: token.borderRadiusSM,
                cursor: 'pointer',
                background: isActive ? token.colorPrimaryBg : undefined,
                border: `1px solid ${isActive ? token.colorPrimary : 'transparent'}`,
              }}
            >
              <Text
                style={{
                  color: isActive ? token.colorPrimary : token.colorText,
                  fontSize: 12,
                  display: 'block',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {title}
              </Text>
              <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginTop: 2 }}>
                {s.agent && title !== roleLabel && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    {roleLabel}
                  </Text>
                )}
                {time && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    {time}
                  </Text>
                )}
              </div>
            </div>
          );
        })}
      </div>

      <Space direction="vertical" style={{ width: '100%', marginTop: 12, flex: '0 0 auto' }}>
        <Button type="primary" block onClick={startNewConversation}>
          + 开启新对话
        </Button>
        <Tooltip title="把当前角色最近一次对话重新拉回聊天区">
          <Button block icon={<SyncOutlined />} onClick={() => loadTranscript()}>
            恢复上次对话
          </Button>
        </Tooltip>
      </Space>

      {/* 诊断入口（高级用户/问题排查） */}
      <Button
        type="link"
        size="small"
        icon={<BugOutlined />}
        onClick={() => setDiagOpen(true)}
        style={{ marginTop: 12, color: token.colorTextTertiary, paddingLeft: 0 }}
      >
        诊断信息
      </Button>
    </div>
  );

  return (
    <Layout>
      {!isCompact && (
        <Sider
          width={260}
          theme="dark"
          style={{
            borderRight: `1px solid ${token.colorBorder}`,
            padding: 16,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            flex: '0 0 260px',
          }}
        >
          {sidebar}
        </Sider>
      )}

      <Content
        style={{
          display: 'flex',
          flexDirection: 'column',
          background: token.colorBgLayout,
          minHeight: 0,
          overflow: 'hidden',
        }}
      >
        {/* 当前角色：切换后立刻可见，窄屏时这里也是侧边栏入口 */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '8px 16px',
            borderBottom: `1px solid ${token.colorBorder}`,
            flex: '0 0 auto',
          }}
        >
          {isCompact && (
            <Button
              type="text"
              icon={<MenuOutlined />}
              onClick={() => setSidebarOpen(true)}
              aria-label="打开助手菜单"
            />
          )}
          <Tag icon={<RobotOutlined />} color="blue" style={{ marginInlineEnd: 0 }}>
            {currentRoleLabel}
          </Tag>
          {selectedAgentInfo?.description && (
            <Text type="secondary" style={{ fontSize: 12, minWidth: 0, flex: 1 }} ellipsis>
              {selectedAgentInfo.description}
            </Text>
          )}
          <div style={{ flex: '0 0 auto', display: 'flex', gap: 8 }}>
            <Button size="small" icon={<PlusOutlined />} onClick={startNewConversation}>
              新对话
            </Button>
          </div>
        </div>

        {/* 顶部状态条（统一产品状态反馈） */}
        <ProductStatusBanner
          state={productStatus}
          contextLabel={productStatus.kind === 'ready' ? '对话服务运行中' : undefined}
          onRetry={() => {
            // 简单重试：重新加载一次状态
            (async () => {
              try {
                const diag = await api.agentDiagnose();
                setDiagnostic(diag);
                if (!diag.ready) {
                  markUnavailable([...(diag.errors || []), ...(diag.hints || [])].join('；'));
                } else {
                  markReady();
                }
              } catch (err) {
                markUnavailable(err instanceof Error ? err.message : String(err));
              }
            })();
          }}
        />

        {productStatus.kind === 'unavailable' ? (
          <div style={{ flex: '1 1 auto', minHeight: 0, overflow: 'auto' }}>
            <ProductStatusBlock state={productStatus} />
          </div>
        ) : (
          <div style={{ position: 'relative', flex: '1 1 auto', minHeight: 0, display: 'flex', flexDirection: 'column' }}>
            <div
              ref={messageListRef}
              onScroll={handleMessageScroll}
              className="chat-scroll"
              style={{ flex: '1 1 auto', minHeight: 0, overflowY: 'auto' }}
            >
              <div className="chat-stream">
                {messages.length === 0 && (
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    style={{ marginTop: 80 }}
                    description={
                      <Space direction="vertical" size={4} align="center">
                        <Text type="secondary">还没有对话，选一个角色开始提问吧</Text>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          例如：帮我看看 600519 现在值不值得买
                        </Text>
                      </Space>
                    }
                  />
                )}
                {messages.map(msg => (
                  <AgentChatMessage key={msg.id} role={msg.role} content={msg.content} error={msg.error} />
                ))}
              </div>
            </div>
            {!atBottom && (
              <button type="button" className="chat-jump" onClick={() => scrollToBottom(true)}>
                回到底部
              </button>
            )}
          </div>
        )}

        {/* 输入区固定在底部，消息再多也不会被顶走 */}
        <div
          style={{
            flex: '0 0 auto',
            padding: '10px 16px 12px',
            borderTop: `1px solid ${token.colorBorder}`,
            background: token.colorBgLayout,
            opacity: productStatus.kind === 'unavailable' ? 0.5 : 1,
            pointerEvents: productStatus.kind === 'unavailable' ? 'none' : 'auto',
          }}
        >
          <div style={{ display: 'flex', gap: 8, alignItems: 'stretch' }}>
            <textarea
              ref={textareaRef}
              className="chat-input-textarea"
              value={input}
              onChange={e => setInput(e.target.value)}
              onFocus={() => setInputFocused(true)}
              onBlur={() => setInputFocused(false)}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  submit();
                }
              }}
              placeholder="输入消息，Enter 发送，Shift+Enter 换行..."
              style={{
                borderRadius: token.borderRadiusLG,
                border: `1px solid ${inputFocused ? token.colorPrimary : token.colorBorder}`,
                background: token.colorBgContainer,
                color: token.colorText,
                boxShadow: inputFocused ? `0 0 0 2px ${token.colorPrimary}40` : 'none',
              }}
            />
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={submit}
              loading={busy}
              disabled={!input.trim()}
              style={{ width: 48, height: 'auto', alignSelf: 'stretch' }}
            />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 6 }}>
            <Text type="secondary" style={{ fontSize: 11 }}>
              Enter 发送 · Shift+Enter 换行
            </Text>
            {busy && (
              <Text type="secondary" style={{ fontSize: 11 }}>
                AI 正在思考...
              </Text>
            )}
          </div>
        </div>
      </Content>

      {/* 窄屏：侧边栏改为抽屉 */}
      <Drawer
        placement="left"
        open={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
        width={280}
        styles={{ body: { padding: 16, display: 'flex', flexDirection: 'column' } }}
      >
        {sidebar}
      </Drawer>

      {/* 诊断抽屉：仅在用户主动打开时展示工程细节 */}
      <Drawer
        title={
          <Space>
            <BugOutlined />
            <span>AI 助手诊断</span>
          </Space>
        }
        placement="right"
        open={diagOpen}
        onClose={() => setDiagOpen(false)}
        width={420}
      >
        {diagnostic && (
          <>
            <Alert
              type={diagnostic.ready ? 'success' : 'error'}
              showIcon
              icon={diagnostic.ready ? <SyncOutlined /> : <WarningOutlined />}
              message={diagnostic.ready ? '服务就绪' : '服务未就绪'}
              description={
                diagnostic.ready
                  ? 'AI 助手服务已完全就绪'
                  : [...(diagnostic.errors || []), ...(diagnostic.hints || [])].join('；')
              }
              style={{ marginBottom: 12 }}
            />
            <Text strong>底层配置</Text>
            <div
              style={{
                background: token.colorFillQuaternary,
                border: `1px solid ${token.colorBorderSecondary}`,
                padding: 8,
                borderRadius: token.borderRadiusSM,
                marginTop: 4,
              }}
            >
              <Text code style={{ whiteSpace: 'pre-wrap' }}>
                {`agent: ${selectedAgent || '(default)'}\nmodel: ${model || '(default)'}\nsession: ${session}`}
              </Text>
            </div>
          </>
        )}
        <Alert
          type="info"
          showIcon
          message="本区域仅供调试使用"
          description="这里展示 Agent / Model / Session 等工程细节。日常使用无需查看此面板。"
          style={{ marginTop: 16 }}
        />
      </Drawer>
    </Layout>
  );
}

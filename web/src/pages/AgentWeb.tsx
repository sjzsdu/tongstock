import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Drawer,
  Empty,
  Layout,
  Segmented,
  Space,
  Typography,
} from 'antd';
import { RobotOutlined, SendOutlined, SyncOutlined, WarningOutlined, BugOutlined } from '@ant-design/icons';
import AgentChatMessage from '../components/AgentChatMessage';
import { ProductStatusBanner, ProductStatusBlock } from '../components/ProductStatus';
import { useProductStatus } from '../hooks/useProductStatus';
import { api, fetchWithAccessToken } from '../api/client';
import { readSSE } from '../lib/sse';
import type { AgentInfo, AgentSessionInfo } from '../types/api';
import type { AgentDiagnosticResponse } from '../types/api';

type ChatMessage = { role: string; content: string; error?: boolean };

const { Sider, Content } = Layout;
const { Text } = Typography;

/** 将原始 agent id 映射为用户可读的产品名称 */
function agentDisplayName(id: string): string {
  const map: Record<string, string> = {
    analyst: '智能分析师',
    researcher: '策略研究员',
    critic: '研究评审',
    screener: '选股助手',
  };
  return map[id] ?? id ?? '通用助手';
}

/** 解析 session 名中的 agent 部分（例如 web:analyst:stamp） */
function extractAgentFromSession(session: string): string | null {
  const parts = session.split(':');
  if (parts.length >= 2) {
    // 形如 web:agent:stamp 或 web:agent
    if (parts[0] === 'web') return parts[1];
  }
  return null;
}

/** 友好的对话标题，避免暴露底层 session id */
function conversationTitle(session: AgentSessionInfo): string {
  const agent = session.agent ? agentDisplayName(session.agent) : '对话';
  const time = session.updated_at
    ? new Date(session.updated_at).toLocaleString()
    : '';
  return `${agent}${time ? ` · ${time}` : ''}`;
}

export default function AgentWeb() {
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [sessions, setSessions] = useState<AgentSessionInfo[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [session, setSession] = useState('web:default');
  const [model, setModel] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [diagnostic, setDiagnostic] = useState<AgentDiagnosticResponse | null>(null);
  const [diagOpen, setDiagOpen] = useState(false);

  // 产品级状态（统一反馈 loading / degraded / failed / unavailable）
  const { state: productStatus, markReady, markUnavailable } = useProductStatus();

  const messageEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = () =>
    requestAnimationFrame(() => {
      messageEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    });

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
        setSelectedAgent(st.defaults?.agent || '');
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
    scrollToBottom();
  }, [messages]);

  const loadTranscript = async (agent?: string, sess?: string) => {
    try {
      const data = await api.agentTranscript(sess || session, agent || selectedAgent);
      if (data.missing) {
        setMessages([{ role: 'system', content: '没有历史对话，直接开始新对话吧。' }]);
        return;
      }
      const history = (data.messages || []).map((m: { role: string; content: string }) => ({
        role: m.role,
        content: m.content,
      }));
      setMessages([...history, { role: 'system', content: '已加载上一次对话。' }]);
    } catch (err) {
      setMessages([{ role: 'assistant', content: String(err), error: true }]);
    }
  };

  const submit = async () => {
    const text = input.trim();
    if (!text || busy) return;
    if (productStatus.kind === 'unavailable') return;
    setInput('');
    setBusy(true);
    const pendingIndex = messages.length + 1;
    setMessages(prev => [
      ...prev,
      { role: 'user', content: text },
      { role: 'assistant', content: '正在生成回复...' },
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
            prev.map((item, idx) =>
              idx === pendingIndex ? { role: 'assistant', content: acc } : item,
            ),
          );
        } else if (event.type === 'error') {
          setMessages(prev =>
            prev.map((item, idx) =>
              idx === pendingIndex
                ? {
                    role: 'assistant',
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
        prev.map((item, idx) =>
          idx === pendingIndex
            ? {
                role: 'assistant',
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
    setSession(value);
    setMessages([{ role: 'system', content: '已开启一段新对话。' }]);
  };

  const filteredSessions = useMemo(
    () => sessions.filter(s => !selectedAgent || !s.agent || s.agent === selectedAgent),
    [sessions, selectedAgent],
  );

  const handleSelectSession = (s: AgentSessionInfo) => {
    setSession(s.session);
    if (s.agent) setSelectedAgent(s.agent);
    loadTranscript(s.agent, s.session);
  };

  // 用户可见的助手选项（隐藏底层 agent id）
  const agentOptions = useMemo(
    () =>
      agents.map(a => ({
        value: a.id,
        label: a.description ? `${agentDisplayName(a.id)} · ${a.description}` : agentDisplayName(a.id),
      })),
    [agents],
  );

  return (
    <Layout style={{ minHeight: 'calc(100vh - 64px)' }}>
      <Sider width={260} theme="dark" style={{ borderRight: '1px solid #1f2937', padding: 16 }}>
        <Typography.Title level={5} style={{ color: '#fff', marginBottom: 16 }}>
          <RobotOutlined /> AI 助手
        </Typography.Title>

        {/* 助手角色选择（不再暴露 "Agent" 工程术语） */}
        <Text type="secondary" style={{ fontSize: 12 }}>
          助手角色
        </Text>
        <Segmented
          value={selectedAgent}
          onChange={v => {
            const val = String(v);
            setSelectedAgent(val);
            // 切换角色时同步更新当前对话的 agent 部分
            const parts = session.split(':');
            const newSession = parts.length >= 2 ? `${parts[0]}:${val}:${parts.slice(2).join(':')}` : `web:${val}`;
            setSession(newSession);
          }}
          options={agentOptions.length > 0 ? agentOptions.map(o => ({ value: o.value, label: o.label.split(' · ')[0] })) : []}
          block
          style={{ marginBottom: 16, background: '#1a1a1a' }}
        />

        {/* 历史对话 */}
        <Text type="secondary" style={{ fontSize: 12 }}>
          历史对话
        </Text>
        <div
          style={{
            maxHeight: 'calc(100vh - 340px)',
            overflowY: 'auto',
            background: '#1a1a1a',
            borderRadius: 4,
            padding: 4,
          }}
        >
          {filteredSessions.length === 0 && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              暂无历史对话
            </Text>
          )}
          {filteredSessions.map(s => {
            const agent = s.agent ? agentDisplayName(s.agent) : '对话';
            const time = s.updated_at ? new Date(s.updated_at).toLocaleString() : '';
            const isActive = s.session === session;
            return (
              <div
                key={s.session}
                onClick={() => handleSelectSession(s)}
                style={{
                  padding: '6px 8px',
                  marginBottom: 4,
                  borderRadius: 4,
                  cursor: 'pointer',
                  background: isActive ? '#1677ff33' : 'transparent',
                  border: isActive ? '1px solid #1677ff' : '1px solid transparent',
                }}
              >
                <Text style={{ color: '#fff', fontSize: 12 }}>{agent}</Text>
                {time && (
                  <Text type="secondary" style={{ fontSize: 11, display: 'block' }}>
                    {time}
                  </Text>
                )}
              </div>
            );
          })}
        </div>

        <Space direction="vertical" style={{ width: '100%', marginTop: 12 }}>
          <Button type="primary" block onClick={startNewConversation}>
            + 开启新对话
          </Button>
          <Button block icon={<SyncOutlined />} onClick={() => loadTranscript()}>
            加载当前对话
          </Button>
        </Space>

        {/* 诊断入口（高级用户/问题排查） */}
        <Button
          type="link"
          size="small"
          icon={<BugOutlined />}
          onClick={() => setDiagOpen(true)}
          style={{ marginTop: 12, color: '#8b95a7' }}
        >
          诊断信息
        </Button>
      </Sider>

      <Content style={{ display: 'flex', flexDirection: 'column', background: '#0b0f19' }}>
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
          <ProductStatusBlock state={productStatus} />
        ) : (
          <div style={{ flex: 1, overflow: 'auto', padding: '16px 0' }}>
            {messages.length === 0 && (
              <Empty
                description="选择角色后开始一段新对话"
                style={{ marginTop: 80 }}
              />
            )}
            {messages.map((msg, idx) => (
              <AgentChatMessage key={idx} role={msg.role} content={msg.content} error={msg.error} />
            ))}
            <div ref={messageEndRef} />
          </div>
        )}

        <div
          style={{
            padding: '12px 16px',
            borderTop: '1px solid #1f2937',
            opacity: productStatus.kind === 'unavailable' ? 0.5 : 1,
            pointerEvents: productStatus.kind === 'unavailable' ? 'none' : 'auto',
          }}
        >
          <div style={{ display: 'flex', gap: 8 }}>
            <textarea
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  submit();
                }
              }}
              placeholder="输入消息，Enter 发送，Shift+Enter 换行..."
              style={{
                flex: 1,
                resize: 'none',
                height: 60,
                padding: '8px 12px',
                borderRadius: 8,
                border: '1px solid #404040',
                background: '#1a1a1a',
                color: '#e0e0e0',
                fontSize: 13,
                outline: 'none',
              }}
            />
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={submit}
              disabled={busy || !input.trim()}
              style={{ height: 60, width: 48 }}
            />
          </div>
          {busy && (
            <div style={{ textAlign: 'center', padding: 4 }}>
              <Text type="secondary">AI 正在思考...</Text>
            </div>
          )}
        </div>
      </Content>

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
            <div style={{ background: '#f5f5f5', padding: 8, borderRadius: 4, marginTop: 4 }}>
              <Text code style={{ whiteSpace: 'pre-wrap' }}>
                {`agent: ${selectedAgent || '(default)'}
model: ${model || '(default)'}
session: ${session}`}
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

// 保留给外部可能需要的辅助函数
export { extractAgentFromSession, conversationTitle };

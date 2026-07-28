import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Alert,
  Empty,
  Layout,
  Select,
  Space,
  Spin,
  Typography,
} from 'antd';
import { RobotOutlined, SendOutlined, SyncOutlined } from '@ant-design/icons';
import AgentChatMessage from '../components/AgentChatMessage';
import { api, fetchWithAccessToken } from '../api/client';
import { readSSE } from '../lib/sse';
import type { AgentInfo, AgentSessionInfo } from '../types/api';
import type { AgentDiagnosticResponse } from '../types/api';

type ChatMessage = { role: string; content: string; error?: boolean };

const { Sider, Content } = Layout;

function sessionLabel(s: AgentSessionInfo) {
  const agent = s.agent ? `[${s.agent}] ` : '';
  const size = s.size ? ` · ${Math.round(s.size / 1024)} KiB` : '';
  const at = s.updated_at ? ` · ${new Date(s.updated_at).toLocaleString()}` : '';
  return `${agent}${s.session}${size}${at}`;
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
  const messageEndRef = useRef<HTMLDivElement>(null);

  const filteredSessions = useMemo(
    () => sessions.filter(s => !selectedAgent || !s.agent || s.agent === selectedAgent),
    [sessions, selectedAgent],
  );

  const scrollToBottom = () =>
    requestAnimationFrame(() => {
      messageEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    });

  useEffect(() => {
    (async () => {
      try {
        const diag = await api.agentDiagnose();
        setDiagnostic(diag);
      } catch {}
      try {
        const state = await api.agentState();
        setAgents(state.agents || []);
        setModel(state.defaults?.model || '');
        setSelectedAgent(state.defaults?.agent || '');
        setSession(state.defaults?.session || 'web:default');
      } catch {
        setMessages([{ role: 'system', content: '无法连接到 Agent 服务。请确保后端已启用 agent 功能。' }]);
      }
      try {
        const sessRes = await api.agentSessions();
        setSessions(sessRes.sessions || []);
      } catch {}
    })();
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  const loadTranscript = async (agent?: string, sess?: string) => {
    try {
      const data = await api.agentTranscript(sess || session, agent || selectedAgent);
      if (data.missing) {
        setMessages([{ role: 'system', content: '没有找到历史记录，可以直接开始新对话。' }]);
        return;
      }
      const history = (data.messages || []).map((m: { role: string; content: string }) => ({ role: m.role, content: m.content }));
      setMessages([...history, { role: 'system', content: `已加载历史：${data.path || ''}` }]);
    } catch (err) {
      setMessages([{ role: 'assistant', content: String(err), error: true }]);
    }
  };

  const submit = async () => {
    const text = input.trim();
    if (!text || busy) return;
    setInput('');
    setBusy(true);
    const pendingIndex = messages.length + 1;
    setMessages(prev => [...prev, { role: 'user', content: text }, { role: 'assistant', content: '正在等待回复...' }]);
    let acc = '';
    try {
      const res = await fetchWithAccessToken('/api/agent/chat/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
        body: JSON.stringify({ message: text, agent: selectedAgent, session, model }),
      });
      if (!res.ok || !res.body) throw new Error(res.statusText || 'stream request failed');
      await readSSE(res, event => {
        if (event.type === 'delta') {
          acc += event.delta || '';
          setMessages(prev => prev.map((item, idx) =>
            idx === pendingIndex ? { role: 'assistant', content: acc } : item
          ));
        } else if (event.type === 'error') {
          setMessages(prev => prev.map((item, idx) =>
            idx === pendingIndex ? { role: 'assistant', content: event.message || event.error || 'agent failed', error: true } : item
          ));
        }
      });
      // Refresh sessions
      const sessRes = await api.agentSessions();
      setSessions(sessRes.sessions || []);
    } catch (err) {
      setMessages(prev => prev.map((item, idx) =>
        idx === pendingIndex ? { role: 'assistant', content: String(err), error: true } : item
      ));
    } finally {
      setBusy(false);
    }
  };

  const createSession = () => {
    const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
    const value = `web:${selectedAgent || 'agent'}:${stamp}`;
    setSession(value);
    setMessages([{ role: 'system', content: `已创建新 session：${value}` }]);
  };

  return (
    <Layout style={{ minHeight: 'calc(100vh - 64px)' }}>
      <Sider width={260} theme="dark" style={{ borderRight: '1px solid #1f2937', padding: 16 }}>
        <Typography.Title level={5} style={{ color: '#fff', marginBottom: 16 }}>
          <RobotOutlined /> AI 助手
        </Typography.Title>

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>Agent</Typography.Text>
        <Select
          value={selectedAgent}
          onChange={v => { setSelectedAgent(v); loadTranscript(v, session); }}
          options={agents.map(a => ({ value: a.id, label: a.description ? `${a.id} - ${a.description}` : a.id }))}
          style={{ width: '100%', marginBottom: 12 }}
          size="small"
        />

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>Session</Typography.Text>
        <input
          value={session}
          onChange={e => setSession(e.target.value)}
          onBlur={() => loadTranscript()}
          style={{
            width: '100%', padding: '4px 8px', marginBottom: 8, borderRadius: 4,
            border: '1px solid #404040', background: '#1a1a1a', color: '#e0e0e0', fontSize: 12,
          }}
        />

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>历史</Typography.Text>
        <Select
          value=""
          onChange={v => {
            const s = sessions.find(item => item.session === v);
            if (!s) return;
            setSession(s.session);
            if (s.agent) setSelectedAgent(s.agent);
            loadTranscript(s.agent, s.session);
          }}
          options={filteredSessions.map(s => ({ value: s.session, label: sessionLabel(s) }))}
          style={{ width: '100%', marginBottom: 8 }}
          size="small"
          placeholder="选择历史 session..."
        />

        <Space direction="vertical" style={{ width: '100%' }}>
          <Button size="small" block onClick={createSession}>新建 Session</Button>
          <Button size="small" block icon={<SyncOutlined />} onClick={() => loadTranscript()}>加载历史</Button>
        </Space>

        <Typography.Text type="secondary" style={{ fontSize: 11, marginTop: 12, display: 'block' }}>
          Model: {model || '(default)'}
        </Typography.Text>
      </Sider>

      <Content style={{ display: 'flex', flexDirection: 'column', background: '#0b0f19' }}>
        {diagnostic && !diagnostic.ready && (
          <Alert
            type="warning"
            showIcon
            message="Agent 服务未完全就绪"
            description={[...(diagnostic.errors || []), ...(diagnostic.hints || [])].join('；')}
            style={{ margin: 12 }}
          />
        )}
        <div style={{ flex: 1, overflow: 'auto', padding: '16px 0' }}>
          {messages.length === 0 && (
            <Empty description="开始一段新对话" style={{ marginTop: 80 }} />
          )}
          {messages.map((msg, idx) => (
            <AgentChatMessage key={idx} role={msg.role} content={msg.content} error={msg.error} />
          ))}
          <div ref={messageEndRef} />
        </div>

        <div style={{ padding: '12px 16px', borderTop: '1px solid #1f2937' }}>
          <div style={{ display: 'flex', gap: 8 }}>
            <textarea
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
              placeholder="输入消息，Enter 发送，Shift+Enter 换行..."
              style={{
                flex: 1, resize: 'none', height: 60, padding: '8px 12px',
                borderRadius: 8, border: '1px solid #404040', background: '#1a1a1a',
                color: '#e0e0e0', fontSize: 13, outline: 'none',
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
          {busy && <div style={{ textAlign: 'center', padding: 4 }}><Spin size="small" /></div>}
        </div>
      </Content>
    </Layout>
  );
}

import { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Select, Space, Spin, Typography } from 'antd';
import ResizableDrawer from './ResizableDrawer';
import { RobotOutlined, SendOutlined } from '@ant-design/icons';
import AgentChatMessage from './AgentChatMessage';
import { api } from '../api/client';
import { readSSE } from '../lib/sse';

type ChatMessage = { role: string; content: string; error?: boolean };

interface AgentChatPanelProps {
  stockCode: string;
  stockName?: string;
  open: boolean;
  onClose: () => void;
}

export default function AgentChatPanel({ stockCode, stockName, open, onClose }: AgentChatPanelProps) {
  const [agents, setAgents] = useState<{ id: string; name: string }[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const messageEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      messageEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    });
  }, []);

  useEffect(() => {
    if (open) {
      api.agentState().then(state => {
        setAgents(state.agents || []);
        // Prefer stock_agent for stock detail panel, fallback to default agent
        const defaultAgent = state.defaults?.stock_agent || state.defaults?.agent || '';
        setSelectedAgent(defaultAgent);
        if (!state.agents?.length) {
          setMessages([{ role: 'system', content: 'Agent 未配置。请在 ~/.tongstock/config.yaml 中设置 agent.enabled: true 并配置 picoclaw。' }]);
        }
      }).catch(() => {
        setMessages([{ role: 'system', content: '无法连接到 Agent 服务。请确保后端已启动且 agent 功能已启用。' }]);
      });
    }
  }, [open]);

  useEffect(() => {
    if (open && messages.length === 0) {
      setMessages([{ role: 'system', content: `已连接到 AI 分析助手。${stockName || ''}（${stockCode}）` }]);
    }
  }, [open, messages.length, stockCode, stockName]);

  useEffect(scrollToBottom, [messages, scrollToBottom]);

  const submit = async () => {
    const text = input.trim();
    if (!text || busy) return;
    setInput('');
    setBusy(true);

    const pendingIndex = messages.length + 1;
    setMessages(prev => [...prev, { role: 'user', content: text }, { role: 'assistant', content: '正在分析...' }]);

    let acc = '';
    try {
      const res = await fetch('/api/agent/chat/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
        body: JSON.stringify({
          message: `[${stockName || stockCode}(${stockCode})] ${text}`,
          agent: selectedAgent,
          session: `web:${stockCode}`,
        }),
      });
      if (!res.ok || !res.body) {
        const errText = await res.text().catch(() => '');
        throw new Error(errText || res.statusText || 'Agent 服务不可用，请检查配置');
      }
      await readSSE(res, event => {
        if (event.type === 'delta') {
          acc += event.delta || '';
          setMessages(prev => prev.map((item, idx) =>
            idx === pendingIndex ? { role: 'assistant', content: acc } : item
          ));
        } else if (event.type === 'error') {
          setMessages(prev => prev.map((item, idx) =>
            idx === pendingIndex ? { role: 'assistant', content: event.error || 'agent failed', error: true } : item
          ));
        }
      });
    } catch (err: any) {
      const msg = err?.message?.includes('Failed to fetch')
        ? '无法连接到后端服务，请确保服务器已启动'
        : String(err);
      setMessages(prev => prev.map((item, idx) =>
        idx === pendingIndex ? { role: 'assistant', content: msg, error: true } : item
      ));
    } finally {
      setBusy(false);
    }
  };

  return (
    <ResizableDrawer
      title={
        <Space>
          <RobotOutlined />
          <span>AI 分析助手</span>
          {stockName && <Typography.Text type="secondary">{stockName}（{stockCode}）</Typography.Text>}
        </Space>
      }
      placement="right"
      defaultWidth={480}
      open={open}
      onClose={onClose}
      styles={{ body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' } }}
      footer={
        <div style={{ display: 'flex', gap: 8 }}>
          <Select
            value={selectedAgent}
            onChange={setSelectedAgent}
            options={agents.map(a => ({ value: a.id, label: a.name || a.id }))}
            style={{ width: 180 }}
            placeholder="选择分析师"
            size="small"
          />
          <div style={{ flex: 1 }} />
          <Button size="small" onClick={() => setMessages([])}>清空对话</Button>
        </div>
      }
    >
      <div style={{ flex: 1, overflow: 'auto', paddingBottom: 8 }}>
        {messages.map((msg, idx) => (
          <AgentChatMessage key={idx} role={msg.role} content={msg.content} error={msg.error} />
        ))}
        <div ref={messageEndRef} />
      </div>
      <div style={{ padding: '8px 12px', borderTop: '1px solid #303030' }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <textarea
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
            placeholder={`向 AI 分析师提问关于 ${stockName || stockCode} 的问题...`}
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
        {busy && <div style={{ textAlign: 'center', padding: 4 }}><Spin size="small" /></div>}
      </div>
    </ResizableDrawer>
  );
}

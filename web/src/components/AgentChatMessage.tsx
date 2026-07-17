import { memo } from 'react';
import { Typography } from 'antd';
import Markdown from 'react-markdown';
import rehypeSanitize from 'rehype-sanitize';

type MessageRole = 'user' | 'assistant' | 'system' | 'tool';

interface AgentChatMessageProps {
  role: string;
  content: string;
  error?: boolean;
}

function normalizeRole(role: string): MessageRole {
  return (['user', 'assistant', 'tool', 'system'] as const).includes(role as MessageRole)
    ? (role as MessageRole)
    : 'assistant';
}

const roleStyles: Record<MessageRole, { bg: string; align: React.CSSProperties['justifyContent']; border: string }> = {
  user: { bg: '#1677ff', align: 'flex-end', border: '1px solid #4096ff' },
  assistant: { bg: '#1f1f1f', align: 'flex-start', border: '1px solid #303030' },
  system: { bg: '#1a1a2e', align: 'center', border: '1px solid #2a2a4a' },
  tool: { bg: '#1a1a1a', align: 'flex-start', border: '1px solid #404040' },
};

function MarkdownContent({ content }: { content: string }) {
  return (
    <div className="agent-chat-markdown" style={{ fontSize: 13, lineHeight: 1.6 }}>
      <Markdown rehypePlugins={[rehypeSanitize]}>{content}</Markdown>
    </div>
  );
}

const MemoizedMarkdown = memo(MarkdownContent);

function AgentChatMessageInner({ role, content, error }: AgentChatMessageProps) {
  const normalizedRole = normalizeRole(role);
  const style = roleStyles[normalizedRole];

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: style.align,
        padding: '8px 16px',
      }}
    >
      <div
        style={{
          maxWidth: '80%',
          padding: '10px 14px',
          borderRadius: 12,
          background: style.bg,
          border: error ? '1px solid #ff4d4f' : style.border,
          color: normalizedRole === 'user' ? '#fff' : '#e0e0e0',
          wordBreak: 'break-word',
        }}
      >
        {normalizedRole === 'tool' && (
          <div style={{ fontSize: 11, color: '#888', marginBottom: 4 }}>tool output</div>
        )}
        {normalizedRole === 'system' ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>{content}</Typography.Text>
        ) : normalizedRole === 'user' ? (
          <div style={{ fontSize: 13, lineHeight: 1.6 }}>{content}</div>
        ) : (
          <MemoizedMarkdown content={content} />
        )}
      </div>
    </div>
  );
}

const AgentChatMessage = memo(AgentChatMessageInner);
export default AgentChatMessage;

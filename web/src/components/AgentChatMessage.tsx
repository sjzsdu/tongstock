import { memo, useState } from 'react';
import { Typography, theme } from 'antd';
import MarkdownRenderer from './MarkdownRenderer';

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

function AgentChatMessageInner({ role, content, error }: AgentChatMessageProps) {
  const { token } = theme.useToken();
  const [copied, setCopied] = useState(false);
  const normalizedRole = normalizeRole(role);

  // system（"已开启一段新对话" 这类提示）做成轻分隔条，不跟正经气泡抢视觉
  if (normalizedRole === 'system') {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          justifyContent: 'center',
          padding: '8px 16px',
        }}
      >
        <div style={{ flex: 1, height: 1, background: token.colorSplit }} />
        <Typography.Text type="secondary" style={{ fontSize: 12, whiteSpace: 'nowrap' }}>
          {content}
        </Typography.Text>
        <div style={{ flex: 1, height: 1, background: token.colorSplit }} />
      </div>
    );
  }

  const styles: Record<
    Exclude<MessageRole, 'system'>,
    { bg: string; align: React.CSSProperties['justifyContent']; border: string; color: string }
  > = {
    user: {
      bg: token.colorPrimary,
      align: 'flex-end',
      border: `1px solid ${token.colorPrimaryBorderHover}`,
      color: '#fff',
    },
    assistant: {
      bg: token.colorBgContainer,
      align: 'flex-start',
      border: `1px solid ${token.colorBorderSecondary}`,
      color: token.colorText,
    },
    tool: {
      bg: token.colorFillQuaternary,
      align: 'flex-start',
      border: `1px solid ${token.colorBorderSecondary}`,
      color: token.colorText,
    },
  };
  const style = styles[normalizedRole as Exclude<MessageRole, 'system'>];

  // 回复气泡右上角常驻复制入口（右侧留白避免压住正文）
  const showCopy = !error && normalizedRole !== 'user';
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // 剪贴板不可用时静默忽略
    }
  };

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: style.align,
        padding: '6px 16px',
      }}
    >
      <div
        className="chat-bubble"
        style={{
          maxWidth: '100%',
          padding: showCopy ? '10px 62px 10px 14px' : '10px 14px',
          borderRadius: 12,
          background: style.bg,
          border: error ? `1px solid ${token.colorError}` : style.border,
          color: style.color,
          wordBreak: 'break-word',
        }}
      >
        {normalizedRole === 'tool' && (
          <div style={{ fontSize: 11, color: token.colorTextTertiary, marginBottom: 4 }}>
            tool output
          </div>
        )}
        {error ? (
          <div style={{ fontSize: 13, lineHeight: 1.7, whiteSpace: 'pre-wrap' }}>{content}</div>
        ) : (
          <MarkdownRenderer
            content={content}
            className={normalizedRole === 'user' ? 'markdown-user' : undefined}
          />
        )}
        {showCopy && (
          <button type="button" className="chat-copy" onClick={handleCopy} aria-label="复制内容">
            {copied ? '已复制' : '复制'}
          </button>
        )}
      </div>
    </div>
  );
}

const AgentChatMessage = memo(AgentChatMessageInner);
export default AgentChatMessage;

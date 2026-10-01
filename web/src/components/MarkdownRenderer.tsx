import { useMemo } from 'react';
import { Marked } from 'marked';
import { markedHighlight } from 'marked-highlight';
import hljs from 'highlight.js';
import DOMPurify from 'dompurify';

const marked = new Marked(
  {
    // 聊天语境：表格/删除线等 GFM 语法要能渲染；
    // 单个换行也算换行，否则流式回答里手写的换行会被合并成一段
    gfm: true,
    breaks: true,
  },
  markedHighlight({
    langPrefix: 'hljs language-',
    highlight(code: string, lang: string) {
      if (lang && hljs.getLanguage(lang)) {
        try {
          return hljs.highlight(code, { language: lang }).value;
        } catch {}
      }
      return code;
    },
  }),
);

interface MarkdownRendererProps {
  content: string;
  className?: string;
}

export default function MarkdownRenderer({ content, className }: MarkdownRendererProps) {
  const html = useMemo(() => {
    if (!content) return '';
    // AI 文本属不可信上游：marked 渲染后必须 sanitize，
    // 清除脚本/事件属性，链接仅保留 http(s)/mailto 等安全协议
    return DOMPurify.sanitize(marked.parse(content) as string, {
      ALLOWED_ATTR: ['href', 'title', 'src', 'alt', 'class', 'target', 'rel'],
      ALLOWED_TAGS: [
        'a', 'abbr', 'b', 'blockquote', 'br', 'code', 'dd', 'del', 'details', 'div', 'dl', 'dt',
        'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr', 'i', 'img', 'ins', 'kbd', 'li', 'mark',
        'ol', 'p', 'pre', 'q', 's', 'span', 'strong', 'sub', 'summary', 'sup', 'table', 'tbody',
        'td', 'th', 'thead', 'tr', 'u', 'ul',
      ],
      FORBID_ATTR: ['style', 'onerror', 'onload', 'onclick'],
    });
  }, [content]);

  // 颜色、字号、行高等统一走 index.css 的 .markdown-body 规则，
  // 这里只叠加调用方的差异（如用户气泡白字）
  return (
    <div
      className={['markdown-body', className].filter(Boolean).join(' ')}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

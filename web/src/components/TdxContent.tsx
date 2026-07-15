import { memo } from 'react';
import type { Section } from '../lib/tdx-parser';

/**
 * Safe URL protocol whitelist for link rendering.
 * Only http and https are allowed; everything else (javascript:, data:, etc.)
 * is rendered as plain text to prevent XSS.
 */
const SAFE_PROTOCOLS = /^(https?):$/i;

function isSafeUrl(url: string): boolean {
  try {
    const parsed = new URL(url);
    return SAFE_PROTOCOLS.test(parsed.protocol);
  } catch {
    return false;
  }
}

/**
 * Renders parsed TDX F10 content as React elements.
 *
 * This replaces the previous approach of building an HTML string and injecting
 * it via dangerouslySetInnerHTML. By rendering structured Section[] data as
 * React elements, all text content is automatically escaped by React,
 * eliminating the XSS attack surface. Link protocols are validated against a
 * whitelist, and all external links get rel="noopener noreferrer".
 */
function TdxContentBase({ sections }: { sections: Section[] }) {
  return (
    <div className="tdx-content text-sm text-slate-300">
      {sections.map((section, idx) => {
        switch (section.type) {
          case 'header':
            return <h3 key={idx} className="tdx-header">{section.text}</h3>;

          case 'table': {
            const maxCols = Math.max(...section.content.map((r) => r.length));
            return (
              <div key={idx} className="tdx-table-wrap">
                <table className="tdx-table">
                  <tbody>
                    {section.content.map((row, rowIdx) => {
                      const isHeaderRow = rowIdx === 0 || (row.length > 0 && /^●/.test(row[0]));
                      return (
                        <tr key={rowIdx}>
                          {Array.from({ length: maxCols }, (_, colIdx) => {
                            const cell = row[colIdx] || '';
                            const tag = isHeaderRow ? 'th' : 'td';
                            const className = isHeaderRow ? 'tdx-th' : 'tdx-td';
                            if (tag === 'th') {
                              return <th key={colIdx} className={className}>{cell}</th>;
                            }
                            return <td key={colIdx} className={className}>{cell}</td>;
                          })}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            );
          }

          case 'paragraph':
            return <div key={idx} className="tdx-para">{section.text}</div>;

          case 'link':
            // Only render safe links as clickable anchors; unsafe URLs
            // (javascript:, data:, etc.) fall back to plain text.
            if (isSafeUrl(section.text)) {
              return (
                <a
                  key={idx}
                  href={section.text}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="tdx-link"
                >
                  {section.text}
                </a>
              );
            }
            return <span key={idx} className="tdx-link">{section.text}</span>;

          case 'divider':
            return <hr key={idx} className="tdx-divider" />;

          default:
            return null;
        }
      })}
    </div>
  );
}

const TdxContent = memo(TdxContentBase);
export default TdxContent;

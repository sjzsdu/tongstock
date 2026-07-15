export interface Section {
  type: 'header' | 'table' | 'paragraph' | 'link' | 'divider';
  content: string[][];
  text: string;
}

function parseTableLine(line: string): string[] {
  const cells: string[] = [];
  const parts = line.split('│');
  for (let i = 0; i < parts.length; i++) {
    const cell = parts[i].trim();
    if (i > 0 && i < parts.length - 1) {
      cells.push(cell);
    }
  }
  return cells;
}

function isTableBorder(line: string): boolean {
  return /^\s*[┌├└─┬┼┴┘┐]/.test(line) && line.includes('─');
}

export function parseTdxText(text: string): Section[] {
  const lines = text.replace(/\r/g, '').split('\n');
  const sections: Section[] = [];
  let currentTable: string[][] = [];
  let currentParagraph: string[] = [];

  const flushTable = () => {
    if (currentTable.length > 0) {
      sections.push({ type: 'table', content: currentTable, text: '' });
      currentTable = [];
    }
  };

  const flushParagraph = () => {
    if (currentParagraph.length > 0) {
      sections.push({ type: 'paragraph', content: [], text: currentParagraph.join('\n') });
      currentParagraph = [];
    }
  };

  for (const line of lines) {
    const trimmed = line.trim();

    if (trimmed === '') {
      flushTable();
      flushParagraph();
      continue;
    }

    if (isTableBorder(trimmed)) {
      flushParagraph();
      continue;
    }

    if (trimmed.startsWith('│')) {
      flushParagraph();
      const cells = parseTableLine(trimmed);
      if (cells.length > 0) {
        currentTable.push(cells);
      }
      continue;
    }

    flushTable();

    if (/^【.*】$/.test(trimmed)) {
      flushParagraph();
      sections.push({ type: 'header', content: [], text: trimmed });
      continue;
    }

    if (/^https?:\/\/\S+$/.test(trimmed)) {
      flushParagraph();
      sections.push({ type: 'link', content: [], text: trimmed });
      continue;
    }

    if (/^─+$/.test(trimmed)) {
      flushParagraph();
      sections.push({ type: 'divider', content: [], text: '' });
      continue;
    }

    if (/^[★☆◇●]/.test(trimmed) || trimmed.startsWith('    ') || trimmed.startsWith('│')) {
      currentParagraph.push(trimmed);
      continue;
    }

    currentParagraph.push(trimmed);
  }

  flushTable();
  flushParagraph();

  return sections;
}

// renderTdxHtml and escapeHtml have been removed in favor of the TdxContent React
// component, which renders the parsed Section[] as React elements. This eliminates
// the dangerouslySetInnerHTML XSS surface entirely — React auto-escapes all text
// content, and link protocols are validated at render time.
